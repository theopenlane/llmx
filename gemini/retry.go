package gemini

import (
	"net/http"
	"time"

	"google.golang.org/genproto/googleapis/rpc/code"
)

// retryableStatuses are the canonical google rpc codes worth re-requesting
var retryableStatuses = map[string]struct{}{
	code.Code_CANCELLED.String():          {},
	code.Code_UNAVAILABLE.String():        {},
	code.Code_INTERNAL.String():           {},
	code.Code_RESOURCE_EXHAUSTED.String(): {},
}

// retryInfoType is the error detail carrying the delay the api asks callers to wait
const retryInfoType = "type.googleapis.com/google.rpc.RetryInfo"

// isRetryableStatus reports whether a generation error status warrants re-requesting
func isRetryableStatus(status string) bool {
	_, ok := retryableStatuses[status]

	return ok
}

// isThrottled reports whether the request was rejected for exceeding a quota
func isThrottled(status string, responseCode int) bool {
	return status == code.Code_RESOURCE_EXHAUSTED.String() || responseCode == http.StatusTooManyRequests
}

// retryInfoDelay reads the delay google returns alongside a quota rejection
func retryInfoDelay(details []map[string]any) time.Duration {
	for _, detail := range details {
		if kind, _ := detail["@type"].(string); kind != retryInfoType {
			continue
		}

		delay, ok := detail["retryDelay"].(string)
		if !ok {
			continue
		}

		parsed, err := time.ParseDuration(delay)
		if err != nil {
			continue
		}

		return parsed
	}

	return 0
}
