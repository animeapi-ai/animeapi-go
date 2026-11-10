// Package animeapi is the official Go client for AnimeAPI (https://animeapi.ai):
// anime image generation from $0.003 per image, plus anime video.
//
//	client := animeapi.New(os.Getenv("ANIMEAPI_API_KEY"))
//	img, err := client.Generate(ctx, animeapi.ImageParams{Prompt: "a silver-haired girl reading under a cherry tree"})
//	fmt.Println(img.ImageURL)
//
// It has no dependencies outside the standard library.
package animeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the version of this module.
const Version = "0.1.2"

// DefaultBaseURL is where requests go unless Client.BaseURL is changed.
const DefaultBaseURL = "https://animeapi.ai"

// Image models. Anime is $0.003 per image, Ultra $0.01, UltraMax $0.02.
const (
	ModelAnime    = "anime"
	ModelUltra    = "ultra"
	ModelUltraMax = "ultra-max"
)

// Orientations. Every orientation costs the same.
const (
	Portrait  = "portrait"
	Square    = "square"
	Landscape = "landscape"
)

// Client calls AnimeAPI. Create one with New; the fields may be changed before first use.
type Client struct {
	// BaseURL defaults to https://animeapi.ai.
	BaseURL string
	// HTTPClient defaults to a client with a 120 second timeout.
	HTTPClient *http.Client
	// MaxRetries is how many times a retryable failure is tried again. Default 2.
	MaxRetries int

	apiKey string
}

// New returns a client for the given API key (it starts with "ank_").
// Create a key at https://animeapi.ai/dashboard/api-keys.
func New(apiKey string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		MaxRetries: 2,
		apiKey:     apiKey,
	}
}

// Error is returned for every failed API call.
type Error struct {
	// Status is the HTTP status, or 0 when no response was received.
	Status int
	// Code is machine-readable, for example INVALID_KEY, INSUFFICIENT_BALANCE,
	// MODERATION_BLOCKED, PROVIDER_REJECTED, RATE_LIMITED, GENERATION_FAILED,
	// PROVIDER_UNAVAILABLE, NOT_FOUND, CONNECTION_ERROR or VIDEO_FAILED.
	Code string
	// Message is the human-readable reason.
	Message string
	// Retryable is true when sending the same request again can succeed.
	Retryable bool
	// RetryAfter is how long the server asked you to wait, if it said.
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("animeapi: %s (status %d, code %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("animeapi: %s (status %d)", e.Message, e.Status)
}

// ImageParams are the options for Generate. Only Prompt is required.
type ImageParams struct {
	// Prompt is what to draw, in plain English. Up to 1000 characters.
	Prompt string `json:"prompt"`
	// Model is ModelAnime (default), ModelUltra or ModelUltraMax.
	Model string `json:"model,omitempty"`
	// Orientation is Portrait (default), Square or Landscape.
	Orientation string `json:"orientation,omitempty"`
	// Enhance (anime model only) rewrites the prompt into tags first. API default true.
	Enhance *bool `json:"enhance,omitempty"`
	// Deprecated: AllowNSFW is ignored by the API; adult output is not available.
	// The field is kept so existing code compiles and is not sent.
	AllowNSFW *bool `json:"-"`
}

// Image is the result of Generate.
type Image struct {
	// ImageURL is a temporary URL of the form https://animeapi.ai/i/<id>.<ext>. Download the image if you need to keep it.
	ImageURL string `json:"image_url"`
	// EnhancedPrompt is what the model actually received.
	EnhancedPrompt string `json:"enhanced_prompt"`
	// BalanceUSD is the balance after this charge.
	BalanceUSD       float64 `json:"balance_usd"`
	GenerationTimeMS float64 `json:"generation_time_ms"`
}

// VideoParams are the options for CreateVideo. Only Prompt is required.
type VideoParams struct {
	Prompt string `json:"prompt"`
	// Resolution is "768p" (default, $0.04 per second) or "480p" ($0.016 per second).
	Resolution string `json:"resolution,omitempty"`
	// Duration in whole seconds, 5 to 15. Default 5.
	Duration    int    `json:"duration,omitempty"`
	Orientation string `json:"orientation,omitempty"`
	Enhance     *bool  `json:"enhance,omitempty"`
	// FirstFrame and LastFrame are an http(s) URL or a base64 data URI, up to 10 MB.
	FirstFrame string `json:"first_frame,omitempty"`
	LastFrame  string `json:"last_frame,omitempty"`
}

// VideoJob is returned when a video is started.
type VideoJob struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	PollURL        string  `json:"poll_url"`
	EnhancedPrompt string  `json:"enhanced_prompt"`
	Resolution     string  `json:"resolution"`
	Duration       int     `json:"duration"`
	Orientation    string  `json:"orientation"`
	CostUSD        float64 `json:"cost_usd"`
	BalanceUSD     float64 `json:"balance_usd"`
}

// Video is the state of a video job. Status is "processing", "succeeded" or "failed".
type Video struct {
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	VideoURL         string   `json:"video_url"`
	Prompt           string   `json:"prompt"`
	EnhancedPrompt   string   `json:"enhanced_prompt"`
	Resolution       string   `json:"resolution"`
	Duration         int      `json:"duration"`
	Orientation      string   `json:"orientation"`
	FirstFrame       bool     `json:"first_frame"`
	LastFrame        bool     `json:"last_frame"`
	CostUSD          float64  `json:"cost_usd"`
	Refunded         bool     `json:"refunded"`
	GenerationTimeMS *float64 `json:"generation_time_ms"`
	Error            string   `json:"error"`
	CreatedAt        string   `json:"created_at"`
}

// Model describes one image model.
type Model struct {
	ID          string `json:"id"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Description string `json:"description"`
}

// Bool returns a pointer to b, for the optional boolean fields.
func Bool(b bool) *bool { return &b }

// Generate makes one anime image. It returns in about 4 seconds on the anime
// model and about 11 seconds on ultra.
func (c *Client) Generate(ctx context.Context, params ImageParams) (*Image, error) {
	var out Image
	if err := c.do(ctx, http.MethodPost, "/api/generate", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateVideo starts a video job and returns at once. The price is charged up
// front and refunded if the job fails.
func (c *Client) CreateVideo(ctx context.Context, params VideoParams) (*VideoJob, error) {
	var out VideoJob
	if err := c.do(ctx, http.MethodPost, "/api/videos", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVideo returns the current state of a video job.
func (c *Client) GetVideo(ctx context.Context, id string) (*Video, error) {
	var out Video
	if err := c.do(ctx, http.MethodGet, "/api/videos/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitVideo polls every interval (5 seconds when zero) until the video has
// finished or ctx is done. A failed job returns an *Error with code VIDEO_FAILED.
func (c *Client) WaitVideo(ctx context.Context, id string, interval time.Duration) (*Video, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		video, err := c.GetVideo(ctx, id)
		if err != nil {
			return nil, err
		}
		switch video.Status {
		case "succeeded":
			return video, nil
		case "failed":
			msg := video.Error
			if msg == "" {
				msg = "the video couldn't be generated"
			}
			return video, &Error{Status: http.StatusOK, Code: "VIDEO_FAILED", Message: msg}
		}
		if err := sleep(ctx, interval); err != nil {
			return nil, err
		}
	}
}

// Models lists the available image models with their prices.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Data []Model `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/models", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Download fetches an image or video URL returned by the API and returns its
// bytes and content type. URLs are not permanent storage, so save what you keep.
func (c *Client) Download(ctx context.Context, fileURL string) ([]byte, string, error) {
	resp, err := c.send(ctx, http.MethodGet, fileURL, nil, false)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", &Error{Code: "CONNECTION_ERROR", Message: err.Error(), Retryable: true}
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	resp, err := c.send(ctx, method, base+path, payload, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &Error{Status: resp.StatusCode, Code: "GENERATION_FAILED", Message: "response was not valid JSON"}
	}
	return nil
}

func (c *Client) send(ctx context.Context, method, target string, payload []byte, authenticated bool) (*http.Response, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	for attempt := 0; ; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "animeapi-go/"+Version)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
			req.Header.Set("Accept", "application/json")
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		var apiErr *Error
		resp, err := httpClient.Do(req)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			apiErr = &Error{Code: "CONNECTION_ERROR", Message: err.Error(), Retryable: !isTimeout(err)}
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return resp, nil
		default:
			apiErr = errorFromResponse(resp)
			resp.Body.Close()
		}

		if !apiErr.Retryable || attempt >= c.MaxRetries {
			return nil, apiErr
		}
		wait := apiErr.RetryAfter
		if wait <= 0 {
			base := 500 * time.Millisecond << attempt
			if base > 8*time.Second {
				base = 8 * time.Second
			}
			wait = base/2 + time.Duration(rand.Int63n(int64(base/2)))
		} else if wait > time.Minute {
			wait = time.Minute
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func errorFromResponse(resp *http.Response) *Error {
	out := &Error{Status: resp.StatusCode, Retryable: resp.StatusCode == 500 || resp.StatusCode == 503}
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
			out.RetryAfter = time.Duration(secs * float64(time.Second))
		} else if when, err := http.ParseTime(v); err == nil {
			if d := time.Until(when); d > 0 {
				out.RetryAfter = d
			}
		}
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body struct {
		Error     json.RawMessage `json:"error"`
		Code      string          `json:"code"`
		Retryable *bool           `json:"retryable"`
	}
	if json.Unmarshal(data, &body) == nil {
		out.Code = body.Code
		if body.Retryable != nil {
			out.Retryable = *body.Retryable
		}
		var msg string
		var nested struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(body.Error, &msg) == nil {
			out.Message = msg
		} else if json.Unmarshal(body.Error, &nested) == nil {
			out.Message = nested.Message
			if out.Code == "" {
				out.Code = nested.Code
			}
		}
	}
	if out.Message == "" {
		out.Message = "request failed: " + resp.Status
	}
	return out
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
