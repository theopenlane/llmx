package llmx

import (
	"context"
	"strings"
)

// Response is a whole generation collected from its stream
type Response struct {
	// Text is the full response text
	Text string
	// FinishReason is why the model stopped generating
	FinishReason FinishReason
	// Usage is the token count the provider reported, nil when it reported none
	Usage *Usage
}

// Generate runs one generation and collects the stream into a single response
func Generate(ctx context.Context, generator Generator, req GenerateRequest) (Response, error) {
	var (
		text     strings.Builder
		response Response
	)

	for chunk, err := range generator.Stream(ctx, req) {
		if err != nil {
			return Response{}, err
		}

		text.WriteString(chunk.Text)

		if chunk.FinishReason != FinishUnknown {
			response.FinishReason = chunk.FinishReason
		}

		if chunk.Usage != nil {
			response.Usage = chunk.Usage
		}
	}

	response.Text = text.String()

	return response, nil
}
