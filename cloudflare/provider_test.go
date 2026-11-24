package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

var errTransport = errors.New("connection reset")

const streamBody = `data: {"choices":[{"delta":{"content":"{\"a\":"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"1}"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3}}

data: [DONE]
`

func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider, err := New(Config{AccountID: "acct", APIKey: "key", BaseURL: server.URL}, WithClientOptions(option.WithHTTPClient(server.Client())))
	assert.NilError(t, err)

	return provider
}

func TestStream(t *testing.T) {
	provider := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Check(t, strings.HasSuffix(r.URL.Path, "/accounts/acct/ai/v1/chat/completions"))
		assert.Check(t, r.Header.Get("Authorization") == "Bearer key")
		assert.Check(t, r.Header.Get("Accept") == "text/event-stream")

		_, _ = io.WriteString(w, streamBody)
	})

	response, err := llmx.Generate(context.Background(), provider, llmx.GenerateRequest{
		Messages: []llmx.Message{llmx.UserMessage(llmx.TextPart("hi"))},
	})

	assert.NilError(t, err)
	assert.Check(t, response.Text == `{"a":1}`)
	assert.Check(t, response.FinishReason == llmx.FinishStop)
	assert.Check(t, response.Usage != nil && response.Usage.InputTokens == 12 && response.Usage.OutputTokens == 3)
}

func TestStreamJSONRequestReturnsWholeCompletion(t *testing.T) {
	provider := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest

		assert.Check(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Check(t, !body.Stream && body.StreamOptions == nil)
		assert.Check(t, body.ResponseFormat != nil && body.ResponseFormat.Type == "json_schema")
		assert.Check(t, r.Header.Get("Accept") == "application/json")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"a\":1}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
	})

	var chunks []llmx.Chunk

	for chunk, err := range provider.Stream(context.Background(), llmx.GenerateRequest{
		Messages:       []llmx.Message{llmx.UserMessage(llmx.TextPart("hi"))},
		ResponseSchema: json.RawMessage(`{"type":"object"}`),
	}) {
		assert.NilError(t, err)

		chunks = append(chunks, chunk)
	}

	if !assert.Check(t, len(chunks) == 1) {
		return
	}

	assert.Check(t, chunks[0].Text == `{"a":1}`)
	assert.Check(t, chunks[0].FinishReason == llmx.FinishStop)
	assert.Check(t, chunks[0].Usage != nil && chunks[0].Usage.InputTokens == 12)
}

func TestReadCompletionRejectsMalformedBody(t *testing.T) {
	_, err := readCompletion(strings.NewReader("{not json"))

	assert.ErrorIs(t, err, ErrStreamDecode)
}

func TestStreamAPIError(t *testing.T) {
	provider := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"errors":[{"code":10000,"message":"rate limited"}]}`)
	})

	_, err := llmx.Generate(context.Background(), provider, llmx.GenerateRequest{})

	apiErr, ok := errors.AsType[*cf.Error](err)
	if !assert.Check(t, ok) {
		return
	}

	assert.Check(t, apiErr.StatusCode == http.StatusTooManyRequests)

	disposition := provider.Classify(err)
	assert.Check(t, disposition.Retry && disposition.Throttled)
	assert.Check(t, disposition.After == 7*time.Second)
}

func TestReadEventsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		expectedErr error
	}{
		{name: "malformed event", body: "data: {not json\n", expectedErr: ErrStreamDecode},
		{name: "event over the size cap", body: "data: " + strings.Repeat("a", maxEventSize+1) + "\n", expectedErr: ErrStreamDecode},
		{name: "stream ends before finishing", body: `data: {"choices":[{"delta":{"content":"{\"a\":"}}]}` + "\n", expectedErr: ErrStreamTruncated},
		{name: "empty body", body: "", expectedErr: ErrStreamTruncated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var err error

			for _, streamErr := range readEvents(strings.NewReader(tt.body)) {
				err = streamErr
			}

			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestReadEventsFinishWithoutDone(t *testing.T) {
	var err error

	for _, streamErr := range readEvents(strings.NewReader(`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n")) {
		err = streamErr
	}

	assert.NilError(t, err)
}

func TestReadEventsStopsWhenCallerBreaks(t *testing.T) {
	seen := 0

	for range readEvents(strings.NewReader(streamBody)) {
		seen++

		break
	}

	assert.Check(t, seen == 1)
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		err               error
		expectedRetry     bool
		expectedThrottled bool
	}{
		{name: "rate limited", err: apiError(http.StatusTooManyRequests), expectedRetry: true, expectedThrottled: true},
		{name: "unavailable", err: apiError(http.StatusServiceUnavailable), expectedRetry: true},
		{name: "bad gateway", err: apiError(http.StatusBadGateway), expectedRetry: true},
		{name: "request timeout", err: apiError(http.StatusRequestTimeout), expectedRetry: true},
		{name: "bad request", err: apiError(http.StatusBadRequest)},
		{name: "unsupported document", err: ErrDocumentsUnsupported},
		{name: "malformed stream", err: ErrStreamDecode},
		{name: "truncated stream", err: ErrStreamTruncated, expectedRetry: true},
		{name: "cancelled context", err: context.Canceled},
		{name: "expired context", err: fmt.Errorf("post: %w", context.DeadlineExceeded)},
		{name: "transport failure", err: errTransport, expectedRetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			disposition := (&Provider{}).Classify(tt.err)

			assert.Check(t, disposition.Retry == tt.expectedRetry)
			assert.Check(t, disposition.Throttled == tt.expectedThrottled)
		})
	}
}

func apiError(status int) *cf.Error {
	return &cf.Error{StatusCode: status, Response: &http.Response{StatusCode: status, Header: http.Header{}}}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		expected time.Duration
	}{
		{value: "", expected: 0},
		{value: "30", expected: 30 * time.Second},
		{value: "soon", expected: 0},
		{value: "Mon, 01 Jan 2001 00:00:00 GMT", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, retryAfter(&http.Response{Header: http.Header{"Retry-After": []string{tt.value}}}) == tt.expected)
		})
	}
}

func TestRetryAfterFutureDate(t *testing.T) {
	at := time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)

	after := retryAfter(&http.Response{Header: http.Header{"Retry-After": []string{at}}})

	assert.Check(t, after > 0 && after <= time.Minute)
}
