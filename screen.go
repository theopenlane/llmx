package llmx

import "context"

// Screener checks an untrusted document before any provider receives it
type Screener interface {
	// Screen reports which filters matched the pdf
	Screen(ctx context.Context, pdf []byte) (Verdict, error)
}

// Verdict is the outcome of screening one document
type Verdict struct {
	// Blocked is true when a filter that must stop the upload matched
	Blocked bool
	// Filters names the matched filters that stop the upload
	Filters []string
	// Reported names matched filters that do not stop the upload, such as sensitive data the caller handles by policy
	Reported []string
	// Skipped names blocking filters that did not run, so the caller decides whether an unscreened document is acceptable
	Skipped []string
}
