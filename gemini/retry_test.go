package gemini

import (
	"testing"

	"google.golang.org/genproto/googleapis/rpc/code"
	"gotest.tools/v3/assert"
)

func TestIsRetryableStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   bool
	}{
		{status: code.Code_CANCELLED.String(), want: true},
		{status: code.Code_UNAVAILABLE.String(), want: true},
		{status: code.Code_INTERNAL.String(), want: true},
		{status: code.Code_RESOURCE_EXHAUSTED.String(), want: true},
		{status: "INVALID_ARGUMENT"},
		{status: "FAILED"},
		{status: ""},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, isRetryableStatus(tt.status) == tt.want)
		})
	}
}
