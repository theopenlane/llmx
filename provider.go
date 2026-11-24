package llmx

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"time"
)

// Document is content uploaded to a provider, released once the work that uploaded it finishes
type Document interface {
	// Release removes the provider side resource, give it a context that survives cancellation such as context.WithoutCancel
	Release(ctx context.Context)
}

// Role is who authored a message
type Role string

const (
	// RoleUser marks a message from the caller
	RoleUser Role = "user"
	// RoleModel marks an earlier model response, for multi-turn requests
	RoleModel Role = "model"
)

// PartKind says which field of a Part carries its content
type PartKind int

const (
	// PartText carries Text, and is the zero value so a bare text literal stays valid
	PartText PartKind = iota
	// PartDocument carries a Document uploaded through the same provider
	PartDocument
	// PartBytes carries inline Data of MIMEType, sent without an upload
	PartBytes
)

// Part is one piece of a message, build it with TextPart, DocumentPart, or BytesPart
type Part struct {
	// Kind says which field carries the content
	Kind PartKind
	// Text is the content of a text part
	Text string
	// Document is the content of a document part, nil when a cache already holds it
	Document Document
	// Data is the content of a bytes part
	Data []byte
	// MIMEType is the type of a bytes part, such as application/pdf or image/png
	MIMEType string
}

// TextPart returns a part holding text
func TextPart(text string) Part {
	return Part{Kind: PartText, Text: text}
}

// DocumentPart returns a part holding an uploaded document
func DocumentPart(doc Document) Part {
	return Part{Kind: PartDocument, Document: doc}
}

// BytesPart returns a part holding inline content of the given mime type
func BytesPart(data []byte, mimeType string) Part {
	return Part{Kind: PartBytes, Data: data, MIMEType: mimeType}
}

// Message is one turn of a request
type Message struct {
	// Role is who authored the message
	Role Role
	// Parts are the message contents in order
	Parts []Part
}

// UserMessage returns a caller message made of the given parts
func UserMessage(parts ...Part) Message {
	return Message{Role: RoleUser, Parts: parts}
}

// CacheRequest asks a provider to hold messages for later generations
type CacheRequest struct {
	// Messages are cached so a repeated generation does not resend them
	Messages []Message
	// Model is the model the cache is bound to, the provider's model when empty
	Model string
	// SystemInstruction is stored with the cache, the provider's instruction when empty
	SystemInstruction string
	// TTL is how long the cache lives, zero leaves the provider default
	TTL time.Duration
}

// GenerateRequest is one generation: what to send the model and what shape to return
type GenerateRequest struct {
	// Messages are sent in order, after whatever Cache already holds
	Messages []Message
	// SystemInstruction overrides the provider's system prompt, and cannot be set alongside a Cache that holds one
	SystemInstruction string
	// ResponseSchema is a json schema document the model output must match, empty for free text
	ResponseSchema json.RawMessage
	// JSON asks for a json response even when no schema is set
	JSON bool
	// Cache names cached content to generate against, empty when everything travels with the request
	Cache string
	// Model overrides the provider's model for this request, and must match the cache's model when Cache is set
	Model string
	// Temperature overrides the provider's sampling temperature for this request
	Temperature *float32
	// MaxOutputTokens overrides the provider's response token cap for this request
	MaxOutputTokens int32
}

// FinishReason is why the model stopped generating
type FinishReason string

const (
	// FinishUnknown is used when the provider gave no reason, usually on a chunk before the last
	FinishUnknown FinishReason = ""
	// FinishStop means the model finished its response
	FinishStop FinishReason = "stop"
	// FinishMaxTokens means the response hit the output token cap and was cut off
	FinishMaxTokens FinishReason = "max_tokens"
	// FinishFiltered means a safety or content filter stopped the response
	FinishFiltered FinishReason = "filtered"
	// FinishOther covers every other reason a provider reports
	FinishOther FinishReason = "other"
)

// Usage is the token count a provider reports for a generation
type Usage struct {
	// InputTokens is how many tokens the request used, including cached content
	InputTokens int
	// CachedInputTokens is how many of the input tokens were served from cached content
	CachedInputTokens int
	// OutputTokens is how many tokens the response used, including any thinking
	OutputTokens int
}

// Chunk is one piece of a streamed response
type Chunk struct {
	// Text is the response text in this chunk
	Text string
	// FinishReason is set on the chunk that ends the response
	FinishReason FinishReason
	// Usage is set when the provider reports token counts, usually on the last chunk
	Usage *Usage
}

// Disposition classifies a generation failure so the caller knows how to proceed
type Disposition struct {
	// Retry is true when the generation should be re-requested, never for a cancelled or expired context
	Retry bool
	// Throttled is true when a quota rejected the request, so concurrent callers should spread their retries
	Throttled bool
	// After is the delay the provider asked for, zero when it supplied none
	After time.Duration
	// CacheMissing is true on a not found error, only meaningful when the request named a cache
	CacheMissing bool
}

// Generator runs generations against a model and classifies their failures
type Generator interface {
	// Stream runs one generation per range over the result, ending with one error when the request or stream fails
	Stream(ctx context.Context, req GenerateRequest) iter.Seq2[Chunk, error]
	// Classify says whether a generation or cache error warrants another attempt
	Classify(err error) Disposition
}

// Uploader makes documents available to a model
type Uploader interface {
	// Upload sends the content to the provider as the given mime type
	Upload(ctx context.Context, r io.Reader, mimeType string) (Document, error)
}

// Cacher holds request content on the provider side between generations
type Cacher interface {
	// CreateCache stores the messages, returning the cache name
	CreateCache(ctx context.Context, req CacheRequest) (string, error)
	// DeleteCache removes cached content, a cache that already expired is not an error
	DeleteCache(ctx context.Context, name string) error
}

// Provider is a backend that can upload documents, cache them, and generate against them
type Provider interface {
	Generator
	Uploader
	Cacher
}
