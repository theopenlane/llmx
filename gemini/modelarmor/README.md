# modelarmor

`llmx.Screener` backed by Google Cloud Model Armor. It checks an untrusted pdf before any model sees it.

## Setup

The template is a full resource name, and the client calls the regional endpoint from it using application default credentials:

```go
screener, err := modelarmor.NewScreener(ctx, os.Getenv("MODEL_ARMOR_TEMPLATE"))
if err != nil {
	return err
}
```

An empty template returns a nil screener, so screening can be left off by config without a separate check. Use `modelarmor.New` instead when a template is required, it returns `ErrTemplateInvalid` for anything that is not `projects/<project>/locations/<location>/templates/<template>`. Call `Close` on the client from `New` when done.

## Screen a document

```go
data, err := io.ReadAll(pdf)
if err != nil {
	return err
}

verdict, err := screener.Screen(ctx, data)
if err != nil {
	return err
}

if verdict.Blocked {
	return fmt.Errorf("document rejected by %v", verdict.Filters)
}

if len(verdict.Skipped) > 0 {
	// a blocking filter did not run, decide whether an unscreened document is acceptable
}

if len(verdict.Reported) > 0 {
	// sensitive data was found, handle it under your own data policy
}
```

- Prompt injection, jailbreak, responsible AI, malicious url, CSAM, and virus filters block the document
- A blocking filter that did not run is listed in `Skipped` rather than treated as clean
- Sensitive data matches are only reported, since reports legitimately contain it
- When Model Armor runs only some filters or none, `Screen` returns `ErrScreenIncomplete` instead of a verdict
