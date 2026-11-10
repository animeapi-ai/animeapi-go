# AnimeAPI client for Go

Official Go client for [AnimeAPI](https://animeapi.ai): anime image generation from **$0.003 per image**, one of the cheapest anime image APIs, plus anime video. New accounts get **3 free images, no card**.

Standard library only. Go 1.21+.

```bash
go get github.com/animeapi-ai/animeapi-go
```

## Quick start

```go
client := animeapi.New(os.Getenv("ANIMEAPI_API_KEY"))
img, err := client.Generate(context.Background(), animeapi.ImageParams{Prompt: "a silver-haired girl reading under a cherry tree"})
if err != nil { log.Fatal(err) }
fmt.Println(img.ImageURL)
```

Create a key (it starts with `ank_`) at <https://animeapi.ai/dashboard/api-keys>.

## Prices

| Model | Price | Good for |
| --- | --- | --- |
| `anime` (default) | $0.003 per image | Characters and illustrations; plain-English prompts are rewritten into tags |
| `ultra` | $0.01 per image | Long, detailed sentences and legible text |
| `ultra-max` | $0.02 per image | Finest detail and real typography |
| Video, 480p | $0.016 per second | 5 to 15 second clips, 24 fps, with sound |
| Video, 768p | $0.04 per second | 5 to 15 second clips, 24 fps, with sound |

Any orientation costs the same. You pay from a prepaid balance (from $5) that never expires; there is no subscription. New accounts get 3 free images, no card.

## Images

```go
img, err := client.Generate(ctx, animeapi.ImageParams{
	Prompt:      "a knight in ornate armor on a cliff at sunrise",
	Model:       animeapi.ModelAnime,   // ModelAnime | ModelUltra | ModelUltraMax
	Orientation: animeapi.Landscape,    // Portrait (default) | Square | Landscape
})

// Image URLs are not permanent storage. Download what you want to keep:
data, contentType, err := client.Download(ctx, img.ImageURL)
```

AnimeAPI does not generate adult content. Explicit prompts are refused and not charged.

## Video

The price is charged when the job starts and refunded if it fails. A 5 second 480p clip costs $0.08.

```go
job, err := client.CreateVideo(ctx, animeapi.VideoParams{Prompt: "a girl waits at a rainy bus stop", Resolution: "480p", Duration: 5})
video, err := client.WaitVideo(ctx, job.ID, 5*time.Second) // polls until succeeded or failed
fmt.Println(video.VideoURL)
```

## Error handling

Every failed call returns an `*animeapi.Error` with `Status`, `Code` and `Retryable`.

```go
var apiErr *animeapi.Error
if errors.As(err, &apiErr) {
	switch apiErr.Code {
	case "INSUFFICIENT_BALANCE": // top up at https://animeapi.ai/dashboard/billing
	case "MODERATION_BLOCKED":   // the prompt was blocked
	default:
		log.Println(apiErr.Status, apiErr.Code, apiErr.Retryable, apiErr.Message)
	}
}
```

Codes: `INVALID_KEY` (401), `INSUFFICIENT_BALANCE` (402), `MODERATION_BLOCKED` and `PROVIDER_REJECTED` (400), `NOT_FOUND` (404), `RATE_LIMITED` (429), `GENERATION_FAILED` (500), `PROVIDER_UNAVAILABLE` (503), plus `CONNECTION_ERROR` and `VIDEO_FAILED` from the client itself.

Retryable failures are tried again up to `client.MaxRetries` times (default 2) with exponential back-off, waiting for `Retry-After` when the API sends it. Set `client.HTTPClient` and `client.BaseURL` to customise transport.

## Links

- Docs: <https://animeapi.ai/docs>
- JavaScript/TypeScript SDK: <https://github.com/animeapi-ai/animeapi-js>
- Python SDK: <https://github.com/animeapi-ai/animeapi-python>

MIT licence.
