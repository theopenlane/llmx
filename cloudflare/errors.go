package cloudflare

import "errors"

var (
	// ErrConfigInvalid is returned when the config is missing the account id or api key
	ErrConfigInvalid = errors.New("cloudflare: account id and api key are required")
	// ErrDocumentsUnsupported is returned when a request carries a document part, which Workers AI chat models cannot read
	ErrDocumentsUnsupported = errors.New("cloudflare: document parts are not supported")
	// ErrCacheUnsupported is returned when a request names cached content, which Workers AI has no api for
	ErrCacheUnsupported = errors.New("cloudflare: cached content is not supported")
	// ErrStreamDecode is returned when a response or streamed event is not valid json or is too large to read
	ErrStreamDecode = errors.New("cloudflare: stream event decode failed")
	// ErrStreamTruncated is returned when the stream ends before a finish reason or the done event, and is retryable
	ErrStreamTruncated = errors.New("cloudflare: stream ended before the response finished")
)
