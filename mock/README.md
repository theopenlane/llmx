# mock

In-memory `llmx.Provider` and `llmx.Screener` for tests. Both return what they are configured with and record every call.

## Provider

```go
provider := &mock.Provider{
	Chunks:    mock.TextChunks(`{"controls":`, `[]}`),
	CacheName: "cachedContents/1",
}

response, err := llmx.Generate(ctx, provider, llmx.GenerateRequest{
	Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("list the controls"))},
})

assert.NilError(t, err)
assert.Check(t, response.Text == `{"controls":[]}`)
assert.Check(t, len(provider.Generated) == 1)
```

Use `llmx.Chunk` values directly to drive truncation or usage handling:

```go
provider := &mock.Provider{Chunks: []llmx.Chunk{
	{Text: `{"controls": [`},
	{FinishReason: llmx.FinishMaxTokens, Usage: &llmx.Usage{InputTokens: 900, OutputTokens: 8192}},
}}
```

Fail a call with `UploadErr`, `CacheErr`, `DeleteErr`, or `StreamErr`, which is yielded after the chunks. `Disposition` is what `Classify` reports for every error, so a test can drive retry paths:

```go
provider := &mock.Provider{
	StreamErr:   errors.New("unavailable"),
	Disposition: llmx.Disposition{Retry: true, After: time.Second},
}
```

Recorded calls:

| Field | Holds |
| --- | --- |
| `Uploaded` | the mime type of each `Upload` |
| `Doc` | the document `Upload` returns, with a `Released` count |
| `CacheReqs` | each `CreateCache` request |
| `Deleted` | each cache name passed to `DeleteCache` |
| `Generated` | each `Stream` request |

## Screener

```go
screener := &mock.Screener{Verdict: llmx.Verdict{Blocked: true, Filters: []string{"pi_and_jailbreak"}}}

verdict, err := screener.Screen(ctx, []byte("%PDF"))

assert.NilError(t, err)
assert.Check(t, verdict.Blocked)
assert.Check(t, len(screener.Screened) == 1)
```
