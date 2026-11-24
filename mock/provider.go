package mock

import (
	"context"
	"io"
	"iter"

	"github.com/theopenlane/llmx"
)

var _ llmx.Provider = (*Provider)(nil)

// Document is an uploaded document that counts how often it was released
type Document struct {
	// Released is how many times Release was called
	Released int
}

// Release records the release
func (d *Document) Release(context.Context) { d.Released++ }

// Provider is a llmx.Provider that returns canned responses and records every call
type Provider struct {
	// Doc is the document Upload hands back, created on first use when nil
	Doc *Document
	// UploadErr fails Upload when set
	UploadErr error
	// CacheName is the cache name CreateCache returns
	CacheName string
	// CacheErr fails CreateCache when set
	CacheErr error
	// DeleteErr fails DeleteCache when set
	DeleteErr error
	// Chunks are what Stream yields in order, including any finish reason or usage
	Chunks []llmx.Chunk
	// StreamErr is yielded after the chunks when set
	StreamErr error
	// Disposition is what Classify reports for every error
	Disposition llmx.Disposition

	// Uploaded are the mime types Upload was called with, in order
	Uploaded []string
	// CacheReqs are the cache requests received, in order
	CacheReqs []llmx.CacheRequest
	// Deleted are the cache names DeleteCache was called with, in order
	Deleted []string
	// Generated are the generation requests received, in order
	Generated []llmx.GenerateRequest
}

// Upload records the call and returns the configured document
func (p *Provider) Upload(_ context.Context, _ io.Reader, mimeType string) (llmx.Document, error) {
	p.Uploaded = append(p.Uploaded, mimeType)

	if p.UploadErr != nil {
		return nil, p.UploadErr
	}

	if p.Doc == nil {
		p.Doc = &Document{}
	}

	return p.Doc, nil
}

// CreateCache records the request and returns the configured cache name
func (p *Provider) CreateCache(_ context.Context, req llmx.CacheRequest) (string, error) {
	p.CacheReqs = append(p.CacheReqs, req)

	if p.CacheErr != nil {
		return "", p.CacheErr
	}

	return p.CacheName, nil
}

// DeleteCache records the name it was asked to delete
func (p *Provider) DeleteCache(_ context.Context, name string) error {
	p.Deleted = append(p.Deleted, name)

	return p.DeleteErr
}

// Stream records the request and yields the configured chunks, then the configured error
func (p *Provider) Stream(_ context.Context, req llmx.GenerateRequest) iter.Seq2[llmx.Chunk, error] {
	p.Generated = append(p.Generated, req)

	return p.yieldChunks
}

// Classify reports the configured disposition for every error
func (p *Provider) Classify(error) llmx.Disposition { return p.Disposition }
