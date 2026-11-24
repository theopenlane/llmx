package llmx

import (
	"context"
	"errors"
	"net/http"
)

// retryableStatusCodes are the http statuses every provider treats as transient
var retryableStatusCodes = map[int]struct{}{
	http.StatusRequestTimeout:      {},
	http.StatusTooManyRequests:     {},
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusServiceUnavailable:  {},
	http.StatusGatewayTimeout:      {},
}

// RetryableStatus reports whether an http status is transient and worth re-requesting
func RetryableStatus(statusCode int) bool {
	_, ok := retryableStatusCodes[statusCode]

	return ok
}

// ContextDone reports whether the error is a cancelled or expired context, which is never worth retrying
func ContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
