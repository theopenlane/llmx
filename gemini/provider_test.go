package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"google.golang.org/genai"
	"google.golang.org/genproto/googleapis/rpc/code"
	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

var errTransport = errors.New("connection reset")

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		err                  error
		expectedRetry        bool
		expectedThrottled    bool
		expectedAfter        time.Duration
		expectedCacheMissing bool
	}{
		{
			name:              "quota rejection is throttled",
			err:               genai.APIError{Status: code.Code_RESOURCE_EXHAUSTED.String()},
			expectedRetry:     true,
			expectedThrottled: true,
		},
		{
			name:          "transient status is not throttled",
			err:           genai.APIError{Status: code.Code_UNAVAILABLE.String()},
			expectedRetry: true,
		},
		{
			name: "unretryable status",
			err:  genai.APIError{Status: "INVALID_ARGUMENT"},
		},
		{
			name:          "reason phrase body still retries on 503",
			err:           genai.APIError{Code: http.StatusServiceUnavailable, Status: "503 Service Unavailable"},
			expectedRetry: true,
		},
		{
			name:          "reason phrase body still retries on 500",
			err:           genai.APIError{Code: http.StatusInternalServerError, Status: "500 Internal Server Error"},
			expectedRetry: true,
		},
		{
			name:          "bad gateway retries",
			err:           genai.APIError{Code: http.StatusBadGateway, Status: "502 Bad Gateway"},
			expectedRetry: true,
		},
		{
			name:          "gateway timeout retries",
			err:           genai.APIError{Code: http.StatusGatewayTimeout, Status: "504 Gateway Timeout"},
			expectedRetry: true,
		},
		{
			name:              "reason phrase body still throttles on 429",
			err:               genai.APIError{Code: http.StatusTooManyRequests, Status: "429 Too Many Requests"},
			expectedRetry:     true,
			expectedThrottled: true,
		},
		{
			name: "unretryable status and code",
			err:  genai.APIError{Code: http.StatusBadRequest, Status: "INVALID_ARGUMENT"},
		},
		{
			name:                 "not found means the cache is gone",
			err:                  genai.APIError{Status: "NOT_FOUND", Code: http.StatusNotFound},
			expectedCacheMissing: true,
		},
		{
			name:          "transport failure with no api error is retried",
			err:           errTransport,
			expectedRetry: true,
		},
		{
			name: "cancelled context is not retried",
			err:  fmt.Errorf("stream: %w", context.Canceled),
		},
		{
			name: "expired context is not retried",
			err:  context.DeadlineExceeded,
		},
		{
			name: "blocked prompt is not retried",
			err:  fmt.Errorf("%w: SAFETY", ErrPromptBlocked),
		},
		{
			name: "foreign document is not retried",
			err:  ErrForeignDocument,
		},
		{
			name: "unsupported part is not retried",
			err:  llmx.ErrUnsupportedPart,
		},
		{
			name: "system override on a cached request is not retried",
			err:  llmx.ErrSystemWithCache,
		},
		{
			name: "retry info supplies the delay",
			err: genai.APIError{
				Status: code.Code_RESOURCE_EXHAUSTED.String(),
				Details: []map[string]any{
					{"@type": "type.googleapis.com/google.rpc.Help"},
					{"@type": retryInfoType, "retryDelay": "42s"},
				},
			},
			expectedRetry:     true,
			expectedThrottled: true,
			expectedAfter:     42 * time.Second,
		},
		{
			name: "malformed retry info is ignored",
			err: genai.APIError{
				Status:  code.Code_RESOURCE_EXHAUSTED.String(),
				Details: []map[string]any{{"@type": retryInfoType, "retryDelay": "soon"}},
			},
			expectedRetry:     true,
			expectedThrottled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			disposition := (&Provider{}).Classify(tt.err)

			assert.Check(t, disposition.Retry == tt.expectedRetry)
			assert.Check(t, disposition.Throttled == tt.expectedThrottled)
			assert.Check(t, disposition.After == tt.expectedAfter)
			assert.Check(t, disposition.CacheMissing == tt.expectedCacheMissing)
		})
	}
}

func TestPromptBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resp     *genai.GenerateContentResponse
		expected error
	}{
		{name: "no feedback", resp: &genai.GenerateContentResponse{}},
		{name: "feedback without a block", resp: &genai.GenerateContentResponse{PromptFeedback: &genai.GenerateContentResponsePromptFeedback{}}},
		{
			name:     "blocked",
			resp:     &genai.GenerateContentResponse{PromptFeedback: &genai.GenerateContentResponsePromptFeedback{BlockReason: genai.BlockedReasonSafety}},
			expected: ErrPromptBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := promptBlocked(tt.resp)
			if tt.expected == nil {
				assert.NilError(t, err)

				return
			}

			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	assert.Check(t, isNotFound(fmt.Errorf("delete: %w", genai.APIError{Code: http.StatusNotFound})))
	assert.Check(t, !isNotFound(genai.APIError{Code: http.StatusForbidden}))
	assert.Check(t, !isNotFound(errTransport))
}

func TestStreamRejectsSystemOverrideOnCache(t *testing.T) {
	var err error

	for _, streamErr := range (&Provider{}).Stream(context.Background(), llmx.GenerateRequest{Cache: "cachedContents/1", SystemInstruction: "summarize"}) {
		err = streamErr
	}

	assert.ErrorIs(t, err, llmx.ErrSystemWithCache)
}

func TestModelFor(t *testing.T) {
	provider := &Provider{model: defaultModel}

	assert.Check(t, provider.modelFor("") == defaultModel)
	assert.Check(t, provider.modelFor("gemini-custom") == "gemini-custom")
}
