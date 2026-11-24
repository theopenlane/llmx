package mock

import "github.com/theopenlane/llmx"

// yieldChunks walks the configured chunks and ends on the configured error
func (p *Provider) yieldChunks(yield func(llmx.Chunk, error) bool) {
	for _, chunk := range p.Chunks {
		if !yield(chunk, nil) {
			return
		}
	}

	if p.StreamErr != nil {
		yield(llmx.Chunk{}, p.StreamErr)
	}
}
