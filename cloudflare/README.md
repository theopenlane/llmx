# cloudflare

`llmx.Generator` backed by Cloudflare Workers AI. Requests go to the OpenAI compatible chat completions endpoint through the `cloudflare-go` sdk client, and the streamed response is parsed here. It is text only: no uploads and no cached content.

## Setup

```go
provider, err := cloudflare.New(cloudflare.Config{
	AccountID: os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
	APIKey:    os.Getenv("CLOUDFLARE_API_TOKEN"),
},
	cloudflare.WithModel("@cf/google/gemma-4-26b-a4b-it"),
	cloudflare.WithSystemInstruction("Be brief"),
	cloudflare.WithTemperature(0.2),
	cloudflare.WithMaxOutputTokens(1024),
)
```

The default model is `@cf/google/gemma-4-26b-a4b-it`, the efficient open model Cloudflare's changelog recommends. The api token needs Workers AI access. `New` returns `ErrConfigInvalid` without an account id and token, and `Config.Provisioned()` reports the same check up front.

Sdk options pass through `WithClientOptions`, for example a custom http client:

```go
provider, err := cloudflare.New(cfg, cloudflare.WithClientOptions(option.WithHTTPClient(httpClient)))
```

## Generate text

```go
response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("Summarize this ticket in one line: " + ticket))},
})
if err != nil {
	return err
}

fmt.Println(response.Text)
```

## Stream

```go
for chunk, err := range provider.Stream(ctx, req) {
	if err != nil {
		return err
	}

	fmt.Print(chunk.Text)
}
```

The last chunks carry the finish reason and, when Workers AI reports it, token usage.

## Multi-turn

Earlier model responses go in as `llmx.RoleModel` messages:

```go
response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{
		llmx.UserMessage(llmx.TextPart("Name a SOC 2 trust services criterion")),
		{Role: llmx.RoleModel, Parts: []llmx.Part{llmx.TextPart("Security")}},
		llmx.UserMessage(llmx.TextPart("Name another")),
	},
})
```

## Structured output

```go
response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages:       []llmx.Message{llmx.UserMessage(llmx.TextPart("Extract the vendor name and domain from: " + text))},
	ResponseSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"domain":{"type":"string"}},"required":["name"]}`),
})
```

- `ResponseSchema` is sent as an OpenAI `json_schema` response format and `JSON: true` as `json_object`
- Workers AI json mode cannot stream, so a json request is sent without streaming and `Stream` yields the whole completion as one chunk
- Not every Workers AI model enforces the schema, so validate the response before trusting it

## Unsupported input

A document part returns `ErrDocumentsUnsupported` and a request naming a cache returns `ErrCacheUnsupported`. Both come back from `Stream` before any request is sent, and `Classify` marks them as not worth retrying.

## Retries

The sdk's own retries are turned off so the caller decides. `Classify` uses the shared `llmx.RetryableStatus` set, marking 429 as throttled and 408, 500, 502, 503, and 504 as retryable, with the delay from `Retry-After`:

```go
response, err := llmx.Generate(ctx, provider, req)
if err != nil {
	disposition := provider.Classify(err)
	if !disposition.Retry {
		return err
	}

	// wait disposition.After, or a short delay when it is zero, then retry
}
```

- Other api errors come back as the sdk's `*cloudflare.Error`
- A stream that ends before a finish reason or the done event returns `ErrStreamTruncated`, which is retryable
- An event that is not valid json or is over 1 MiB returns `ErrStreamDecode`, which is not
- A cancelled or expired context is never retried
