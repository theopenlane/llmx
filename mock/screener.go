package mock

import (
	"context"

	"github.com/theopenlane/llmx"
)

// Screener is a llmx.Screener that returns a fixed verdict and records what it screened
type Screener struct {
	// Verdict is returned for every document
	Verdict llmx.Verdict
	// Err fails Screen when set
	Err error
	// Screened are the documents Screen received, in order
	Screened [][]byte
}

var _ llmx.Screener = (*Screener)(nil)

// Screen records the document and returns the configured verdict
func (s *Screener) Screen(_ context.Context, pdf []byte) (llmx.Verdict, error) {
	s.Screened = append(s.Screened, pdf)

	return s.Verdict, s.Err
}
