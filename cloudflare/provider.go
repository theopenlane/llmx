package cloudflare

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/option"

	"github.com/theopenlane/llmx"
)

const (
	// eventDataPrefix starts every data line of a server-sent event stream
	eventDataPrefix = "data:"
	// eventDone is the data that ends an OpenAI compatible stream
	eventDone = "[DONE]"
	// maxEventSize caps one streamed event line
	maxEventSize = 1 << 20
)

// ensure the provider satisfies the llmx generator seam
var _ llmx.Generator = (*Provider)(nil)

// Stream runs one generation, yielding each streamed chunk as it arrives
func (p *Provider) Stream(ctx context.Context, req llmx.GenerateRequest) iter.Seq2[llmx.Chunk, error] {
	return func(yield func(llmx.Chunk, error) bool) {
		body, err := p.buildRequest(req)
		if err != nil {
			yield(llmx.Chunk{}, err)

			return
		}

		encoded, err := json.Marshal(body)
		if err != nil {
			yield(llmx.Chunk{}, err)

			return
		}

		accept := "application/json"
		if body.Stream {
			accept = "text/event-stream"
		}

		// an *http.Response target makes the sdk hand back the body unread
		var resp *http.Response
		if err := p.client.Post(ctx, p.completionsPath(), encoded, &resp, option.WithHeader("Accept", accept)); err != nil {
			yield(llmx.Chunk{}, err)

			return
		}

		defer resp.Body.Close()

		if !body.Stream {
			yield(readCompletion(resp.Body))

			return
		}

		for chunk, err := range readEvents(resp.Body) {
			if !yield(chunk, err) || err != nil {
				return
			}
		}
	}
}

// readCompletion decodes a whole chat completion into one chunk, used for json requests that cannot stream
func readCompletion(r io.Reader) (llmx.Chunk, error) {
	var completion chatChunk
	if err := json.NewDecoder(r).Decode(&completion); err != nil {
		return llmx.Chunk{}, fmt.Errorf("%w: %w", ErrStreamDecode, err)
	}

	return responseChunk(completion), nil
}

// Classify reports whether an error warrants another attempt, retrying transport failures with no api error
func (p *Provider) Classify(err error) llmx.Disposition {
	if llmx.ContextDone(err) || errors.Is(err, ErrDocumentsUnsupported) || errors.Is(err, ErrCacheUnsupported) ||
		errors.Is(err, ErrStreamDecode) || errors.Is(err, llmx.ErrUnsupportedPart) {
		return llmx.Disposition{}
	}

	apiErr, ok := errors.AsType[*cf.Error](err)
	if !ok {
		return llmx.Disposition{Retry: true}
	}

	if !llmx.RetryableStatus(apiErr.StatusCode) {
		return llmx.Disposition{}
	}

	return llmx.Disposition{
		Retry:     true,
		Throttled: apiErr.StatusCode == http.StatusTooManyRequests,
		After:     retryAfter(apiErr.Response),
	}
}

// readEvents walks a server-sent event stream, yielding a chunk per data event and failing a stream that ends early
func readEvents(r io.Reader) iter.Seq2[llmx.Chunk, error] {
	return func(yield func(llmx.Chunk, error) bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventSize)

		finished := false

		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), eventDataPrefix)
			if !ok {
				continue
			}

			data = strings.TrimSpace(data)
			if data == eventDone {
				return
			}

			var event chatChunk
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				yield(llmx.Chunk{}, fmt.Errorf("%w: %w", ErrStreamDecode, err))

				return
			}

			chunk := responseChunk(event)
			finished = finished || chunk.FinishReason != llmx.FinishUnknown

			if !yield(chunk, nil) {
				return
			}
		}

		switch err := scanner.Err(); {
		case errors.Is(err, bufio.ErrTooLong):
			yield(llmx.Chunk{}, fmt.Errorf("%w: %w", ErrStreamDecode, err))
		case err != nil:
			yield(llmx.Chunk{}, err)
		case !finished:
			yield(llmx.Chunk{}, ErrStreamTruncated)
		}
	}
}

// retryAfter parses the Retry-After header as seconds or an http date, zero when absent or unreadable
func retryAfter(resp *http.Response) time.Duration {
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}

	return 0
}
