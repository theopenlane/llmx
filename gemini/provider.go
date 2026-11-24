package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"

	"google.golang.org/genai"

	"github.com/theopenlane/logx"

	"github.com/theopenlane/llmx"
)

// ensure the provider satisfies the llmx seam
var _ llmx.Provider = (*Provider)(nil)

// document is the content as the model receives it, a file reference on the Gemini API or inline bytes on Vertex AI
type document struct {
	client *genai.Client
	part   *genai.Part
	file   *genai.File
}

// Release removes the uploaded file once the caller is finished, a no-op for inline documents
func (d *document) Release(ctx context.Context) {
	if d.file == nil {
		return
	}

	if _, err := d.client.Files.Delete(ctx, d.file.Name, nil); err != nil {
		logx.FromContext(ctx).Warn().Err(err).Str("file", d.file.Name).Msg("llmx: failed to delete uploaded file")
	}
}

// Upload prepares the content for the configured backend, inline on Vertex AI since it has no files api
func (p *Provider) Upload(ctx context.Context, r io.Reader, mimeType string) (llmx.Document, error) {
	if p.backend == genai.BackendVertexAI {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}

		return &document{client: p.client, part: genai.NewPartFromBytes(data, mimeType)}, nil
	}

	uploaded, err := p.client.Files.Upload(ctx, r, &genai.UploadFileConfig{MIMEType: mimeType})
	if err != nil {
		return nil, err
	}

	return &document{
		client: p.client,
		part:   genai.NewPartFromURI(uploaded.URI, uploaded.MIMEType),
		file:   uploaded,
	}, nil
}

// CreateCache stores the messages against the request's model, or the provider's when it names none
func (p *Provider) CreateCache(ctx context.Context, req llmx.CacheRequest) (string, error) {
	contents, err := requestContents(req.Messages)
	if err != nil {
		return "", err
	}

	cache, err := p.client.Caches.Create(ctx, p.modelFor(req.Model), &genai.CreateCachedContentConfig{
		TTL:               req.TTL,
		Contents:          contents,
		SystemInstruction: p.systemContent(req.SystemInstruction),
	})
	if err != nil {
		return "", err
	}

	return cache.Name, nil
}

// DeleteCache removes cached content, a cache that already expired is not an error
func (p *Provider) DeleteCache(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}

	if _, err := p.client.Caches.Delete(ctx, name, nil); err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to delete document cache: %w", err)
	}

	return nil
}

// Stream runs one generation, yielding each response chunk as it arrives
func (p *Provider) Stream(ctx context.Context, req llmx.GenerateRequest) iter.Seq2[llmx.Chunk, error] {
	return func(yield func(llmx.Chunk, error) bool) {
		if req.Cache != "" && req.SystemInstruction != "" {
			yield(llmx.Chunk{}, llmx.ErrSystemWithCache)

			return
		}

		contents, err := requestContents(req.Messages)
		if err != nil {
			yield(llmx.Chunk{}, err)

			return
		}

		for resp, err := range p.client.Models.GenerateContentStream(ctx, p.modelFor(req.Model), contents, p.generateConfig(req)) {
			if err == nil {
				err = promptBlocked(resp)
			}

			if err != nil {
				yield(llmx.Chunk{}, err)

				return
			}

			if !yield(responseChunk(resp), nil) {
				return
			}
		}
	}
}

// Classify reports whether an error warrants another attempt, retrying transport failures with no api error
func (p *Provider) Classify(err error) llmx.Disposition {
	if llmx.ContextDone(err) || errors.Is(err, ErrPromptBlocked) || errors.Is(err, ErrForeignDocument) ||
		errors.Is(err, llmx.ErrUnsupportedPart) || errors.Is(err, llmx.ErrSystemWithCache) {
		return llmx.Disposition{}
	}

	apiErr, ok := errors.AsType[genai.APIError](err)
	if !ok {
		return llmx.Disposition{Retry: true}
	}

	if isRetryableStatus(apiErr.Status) || llmx.RetryableStatus(apiErr.Code) {
		return llmx.Disposition{
			Retry:     true,
			Throttled: isThrottled(apiErr.Status, apiErr.Code),
			After:     retryInfoDelay(apiErr.Details),
		}
	}

	return llmx.Disposition{CacheMissing: apiErr.Code == http.StatusNotFound}
}

// modelFor returns the requested model, or the provider's when the request names none
func (p *Provider) modelFor(model string) string {
	if model != "" {
		return model
	}

	return p.model
}

// promptBlocked returns ErrPromptBlocked with the reason when Gemini refused the prompt and sent no candidates
func promptBlocked(resp *genai.GenerateContentResponse) error {
	if resp.PromptFeedback == nil || resp.PromptFeedback.BlockReason == "" {
		return nil
	}

	return fmt.Errorf("%w: %s %s", ErrPromptBlocked, resp.PromptFeedback.BlockReason, resp.PromptFeedback.BlockReasonMessage)
}

// isNotFound reports whether the error is a Gemini api not found response
func isNotFound(err error) bool {
	apiErr, ok := errors.AsType[genai.APIError](err)

	return ok && apiErr.Code == http.StatusNotFound
}
