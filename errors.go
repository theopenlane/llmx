package llmx

import "errors"

var (
	// ErrUnsupportedPart is returned when a provider cannot send a part of the given kind
	ErrUnsupportedPart = errors.New("llmx: part kind not supported by this provider")
	// ErrSystemWithCache is returned when a request overrides the system instruction while naming a cache that holds one
	ErrSystemWithCache = errors.New("llmx: system instruction cannot be overridden on a cached request")
)
