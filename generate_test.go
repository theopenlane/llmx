package llmx_test

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
	"github.com/theopenlane/llmx/mock"
)

var errStream = errors.New("stream failed")

func TestGenerateCollectsChunks(t *testing.T) {
	provider := &mock.Provider{Chunks: []llmx.Chunk{
		{Text: "hello "},
		{Text: "world", FinishReason: llmx.FinishMaxTokens},
		{Usage: &llmx.Usage{InputTokens: 5, OutputTokens: 2}},
	}}

	response, err := llmx.Generate(context.Background(), provider, llmx.GenerateRequest{})

	assert.NilError(t, err)
	assert.Check(t, response.Text == "hello world")
	assert.Check(t, response.FinishReason == llmx.FinishMaxTokens)
	assert.Check(t, response.Usage != nil && response.Usage.InputTokens == 5 && response.Usage.OutputTokens == 2)
	assert.Check(t, len(provider.Generated) == 1)
}

func TestGenerateReturnsStreamError(t *testing.T) {
	provider := &mock.Provider{Chunks: mock.TextChunks("partial"), StreamErr: errStream}

	response, err := llmx.Generate(context.Background(), provider, llmx.GenerateRequest{})

	assert.ErrorIs(t, err, errStream)
	assert.Check(t, response.Text == "")
}
