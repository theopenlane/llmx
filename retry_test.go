package llmx_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

func TestRetryableStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode int
		expected   bool
	}{
		{statusCode: http.StatusRequestTimeout, expected: true},
		{statusCode: http.StatusTooManyRequests, expected: true},
		{statusCode: http.StatusInternalServerError, expected: true},
		{statusCode: http.StatusBadGateway, expected: true},
		{statusCode: http.StatusServiceUnavailable, expected: true},
		{statusCode: http.StatusGatewayTimeout, expected: true},
		{statusCode: http.StatusBadRequest},
		{statusCode: http.StatusNotFound},
		{statusCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			t.Parallel()

			assert.Check(t, llmx.RetryableStatus(tt.statusCode) == tt.expected)
		})
	}
}

func TestContextDone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "canceled", err: context.Canceled, expected: true},
		{name: "deadline", err: context.DeadlineExceeded, expected: true},
		{name: "wrapped", err: fmt.Errorf("request failed: %w", context.Canceled), expected: true},
		{name: "other", err: errors.New("connection reset")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, llmx.ContextDone(tt.err) == tt.expected)
		})
	}
}
