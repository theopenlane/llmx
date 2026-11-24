[![Build status](https://badge.buildkite.com/b4e79f9d76e5c585fc971ae299106d45e85fd8e7a16241386a.svg)](https://buildkite.com/theopenlane/llmx?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/theopenlane/llmx.svg)](https://pkg.go.dev/github.com/theopenlane/llmx)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache2.0-brightgreen.svg)](https://opensource.org/licenses/Apache-2.0)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=theopenlane_llmx&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=theopenlane_llmx)

# llmx

Provider-neutral seams for talking to model backends, plus the clients that implement them. Callers depend on the interfaces here and pick a backend at setup.

## Packages

| Package | What it does |
| --- | --- |
| `llmx` | `Generator`, `Uploader`, `Cacher`, `Provider`, and `Screener` interfaces, their request and result types, and `Generate` |
| [`llmx/gemini`](gemini/README.md) | `llmx.Provider` backed by Gemini, on the Gemini Developer API or Vertex AI |
| [`llmx/gemini/modelarmor`](gemini/modelarmor/README.md) | `llmx.Screener` backed by Google Cloud Model Armor |
| [`llmx/cloudflare`](cloudflare/README.md) | `llmx.Generator` backed by Cloudflare Workers AI through its OpenAI compatible endpoint |
| [`llmx/mock`](mock/README.md) | In-memory provider and screener that record every call, for tests |

Each package README has worked examples: setup, generating, streaming, structured output, and handling retries.

## Interfaces

Each backend implements only what it supports:

- `Generator` runs generations. `Stream` yields `Chunk`s with text, a `FinishReason` on the last one, and `Usage` when the provider reports it. `Classify` turns an error into a `Disposition`: retry, throttled, the delay the api asked for, or a missing cache
- `Uploader` makes a document available to the model as a mime type and returns a `Document` the caller releases when done
- `Cacher` holds messages on the provider side so repeated generations do not resend them
- `Provider` is all three, for callers like `docextract` that need uploads and caching

`Screener` checks an untrusted document before any provider receives it and returns a `Verdict` of blocking, skipped, and reported filters.

`RetryableStatus` and `ContextDone` are the shared rules every provider's `Classify` follows: 408, 429, and 5xx gateway statuses are transient, and a cancelled or expired context is never retried.

A `GenerateRequest` is a list of `Message`s, each a role and `Part`s. Build parts with `TextPart`, `DocumentPart` for an uploaded `Document`, or `BytesPart` for inline content with a mime type. A provider returns `ErrUnsupportedPart` for a kind it cannot send.

A request can also:

- set a `ResponseSchema` as a json schema document (`json.RawMessage`), or ask for `JSON` without one
- override the provider's `SystemInstruction`, `Model`, `Temperature`, and `MaxOutputTokens`
- name a `Cache`, in which case the system instruction comes from the cache and overriding it returns `ErrSystemWithCache`

`llmx.Generate` runs one generation and collects the stream into a `Response`:

```go
response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("Summarize this in one line"))},
})
```

## Gemini

The system instruction is set once on the provider and sent with every cache and uncached generation.

```go
provider, err := gemini.New(ctx, gemini.Config{
	Backend:  gemini.BackendVertex,
	Project:  "my-project",
	Location: "us-central1",
},
	gemini.WithModel("gemini-3.1-pro-preview"),
	gemini.WithSystemInstruction("You extract structured data from audit reports"),
)
```

- `Config` holds connection settings only: `Backend` is `gemini` (api key) or `vertex` (Google Cloud credentials), defaulting to `gemini`
- `Config.Provisioned()` reports whether the config has enough to make calls, useful for disabling a feature at startup
- `WithModel`, `WithSystemInstruction`, `WithTemperature`, and `WithMaxOutputTokens` set the provider defaults, an empty model or instruction keeps the default

## Model Armor

```go
screener, err := modelarmor.NewScreener(ctx, "projects/<project>/locations/<location>/templates/<template>")
```

An empty template returns a nil screener, so screening stays off without a separate check. Sensitive data matches are reported, every other filter match blocks the document.

## Cloudflare

```go
provider, err := cloudflare.New(cloudflare.Config{AccountID: accountID, APIKey: token},
	cloudflare.WithModel("@cf/google/gemma-4-26b-a4b-it"),
	cloudflare.WithSystemInstruction("Be brief"),
)
```

- The default model is `@cf/google/gemma-4-26b-a4b-it`
- Requests go through the `cloudflare-go` sdk client, `WithClientOptions` passes sdk options such as an http client
- Text only: a document part returns `ErrDocumentsUnsupported` and a cache returns `ErrCacheUnsupported`
- `ResponseSchema` is sent as an OpenAI `json_schema` response format, `JSON` as `json_object`, and either one turns streaming off since Workers AI json mode cannot stream
- The sdk's own retries are off, `Classify` marks 429 and 5xx as retryable and reads `Retry-After`
