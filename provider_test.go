package llmx_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

func TestPartConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		part     llmx.Part
		expected llmx.PartKind
	}{
		{name: "text", part: llmx.TextPart("hi"), expected: llmx.PartText},
		{name: "bare text literal", part: llmx.Part{Text: "hi"}, expected: llmx.PartText},
		{name: "document", part: llmx.DocumentPart(nil), expected: llmx.PartDocument},
		{name: "bytes", part: llmx.BytesPart([]byte("%PDF"), "application/pdf"), expected: llmx.PartBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, tt.part.Kind == tt.expected)
		})
	}
}
