package mock

import "github.com/theopenlane/llmx"

// TextChunks returns one text chunk per string, for a stream that needs no finish reason or usage
func TextChunks(texts ...string) []llmx.Chunk {
	chunks := make([]llmx.Chunk, 0, len(texts))

	for _, text := range texts {
		chunks = append(chunks, llmx.Chunk{Text: text})
	}

	return chunks
}
