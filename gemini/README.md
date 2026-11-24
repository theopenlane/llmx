# gemini

`llmx.Provider` backed by Gemini, on the Gemini Developer API or Vertex AI. It implements every llmx seam: generation, uploads, and cached content.

## Setup

Gemini Developer API, authenticated with an api key:

```go
provider, err := gemini.New(ctx, gemini.Config{APIKey: os.Getenv("GEMINI_API_KEY")})
```

Vertex AI, authenticated with application default credentials:

```go
provider, err := gemini.New(ctx, gemini.Config{
	Backend:  gemini.BackendVertex,
	Project:  "my-project",
	Location: "us-central1",
},
	gemini.WithModel("gemini-3.1-pro-preview"),
	gemini.WithSystemInstruction("You extract structured data from audit reports"),
	gemini.WithTemperature(0.2),
	gemini.WithMaxOutputTokens(8192),
)
```

`Config` holds connection settings and carries koanf tags, so it can be loaded straight from config. `New` returns `ErrConfigInvalid` without an api key, or without a project and location on Vertex AI. Check `Config.Provisioned()` at startup to disable a feature instead:

```go
if !cfg.Provisioned() {
	return nil
}
```

## Generate text

```go
response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("Summarize the SOC 2 trust services criteria in one line"))},
})
if err != nil {
	return err
}

fmt.Println(response.Text, response.FinishReason)
```

## Stream

```go
for chunk, err := range provider.Stream(ctx, req) {
	if err != nil {
		return err
	}

	fmt.Print(chunk.Text)

	if chunk.FinishReason == llmx.FinishMaxTokens {
		// the response was cut off, ask the model to continue
	}
}
```

## Structured output

`ResponseSchema` takes a json schema document as `json.RawMessage`, written by hand or generated from a Go type with any schema library:

```go
type Summary struct {
	Title  string   `json:"title"`
	Points []string `json:"points"`
}

schema, err := json.Marshal(jsonschema.Reflect(&Summary{}))
if err != nil {
	return err
}

response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages:       []llmx.Message{llmx.UserMessage(llmx.TextPart("Summarize this release"))},
	ResponseSchema: schema,
})

var summary Summary
err = json.Unmarshal([]byte(response.Text), &summary)
```

Set `JSON: true` instead to ask for json without a schema.

## Documents

On the Gemini Developer API the content goes through the files api, on Vertex AI it travels inline. Release the document when done either way:

```go
pdf, err := os.Open("report.pdf")
if err != nil {
	return err
}
defer pdf.Close()

doc, err := provider.Upload(ctx, pdf, "application/pdf")
if err != nil {
	return err
}
defer doc.Release(ctx)

response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.DocumentPart(doc), llmx.TextPart("List the controls in this report"))},
})
```

## Cached content

Cache a document once and point many generations at it instead of resending it:

```go
cache, err := provider.CreateCache(ctx, llmx.CacheRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.DocumentPart(doc))},
	TTL:      time.Hour,
})
if err != nil {
	return err
}
defer provider.DeleteCache(ctx, cache)

response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Cache:    cache,
	Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("List the findings"))},
})
```

- A request against a cache does not resend the system instruction, the cache already holds it
- A cache is bound to one model. Set `CacheRequest.Model` when the generations will override `Model`, and keep them the same
- `DeleteCache` on a cache that already expired returns nil

## Per-request overrides

```go
temperature := float32(0.7)

response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages:        []llmx.Message{llmx.UserMessage(llmx.TextPart("Suggest three names"))},
	Model:           fastModel,
	Temperature:     &temperature,
	MaxOutputTokens: 256,
})
```

## Retries

`Classify` says what to do with a failed generation:

```go
response, err := llmx.Generate(ctx, provider, req)
if err != nil {
	disposition := provider.Classify(err)

	switch {
	case disposition.CacheMissing:
		// the cache expired, rebuild it and retry
	case disposition.Throttled:
		// over quota, back off for disposition.After before retrying
	case disposition.Retry:
		// transient, retry after disposition.After or a short delay
	default:
		return err
	}
}
```

- The delay comes from the `RetryInfo` detail Google attaches to quota errors, zero when there is none
- `CacheMissing` is set on any not found error, so only act on it when the request named a cache
- A cancelled context, `ErrPromptBlocked` (Gemini refused the prompt outright), and `ErrForeignDocument` (a document uploaded through another provider) are never retried
