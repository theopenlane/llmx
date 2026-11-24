package gemini

import (
	"google.golang.org/genai"

	"github.com/theopenlane/llmx"
)

// jsonMIMEType is the response mime type that asks the model for json
const jsonMIMEType = "application/json"

// responseChunk is the text, finish reason, and usage of a response's first candidate
func responseChunk(resp *genai.GenerateContentResponse) llmx.Chunk {
	var chunk llmx.Chunk

	if usage := resp.UsageMetadata; usage != nil {
		chunk.Usage = &llmx.Usage{
			InputTokens:       int(usage.PromptTokenCount),
			CachedInputTokens: int(usage.CachedContentTokenCount),
			OutputTokens:      int(usage.CandidatesTokenCount + usage.ThoughtsTokenCount),
		}
	}

	if len(resp.Candidates) == 0 {
		return chunk
	}

	candidate := resp.Candidates[0]
	chunk.FinishReason = finishReason(candidate.FinishReason)

	if candidate.Content == nil {
		return chunk
	}

	for _, part := range candidate.Content.Parts {
		if part != nil {
			chunk.Text += part.Text
		}
	}

	return chunk
}

// finishReason maps a genai finish reason onto the provider-neutral one
func finishReason(reason genai.FinishReason) llmx.FinishReason {
	switch reason {
	case "", genai.FinishReasonUnspecified:
		return llmx.FinishUnknown
	case genai.FinishReasonStop:
		return llmx.FinishStop
	case genai.FinishReasonMaxTokens:
		return llmx.FinishMaxTokens
	case genai.FinishReasonSafety, genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonRecitation:
		return llmx.FinishFiltered
	default:
		return llmx.FinishOther
	}
}

// requestContents builds one genai content per message, skipping messages with nothing to send
func requestContents(messages []llmx.Message) ([]*genai.Content, error) {
	contents := make([]*genai.Content, 0, len(messages))

	for _, message := range messages {
		parts, err := requestParts(message.Parts)
		if err != nil {
			return nil, err
		}

		if len(parts) == 0 {
			continue
		}

		role := genai.Role(genai.RoleUser)
		if message.Role == llmx.RoleModel {
			role = genai.RoleModel
		}

		contents = append(contents, genai.NewContentFromParts(parts, role))
	}

	return contents, nil
}

// requestParts converts message parts, skipping empty ones and rejecting documents from another provider
func requestParts(parts []llmx.Part) ([]*genai.Part, error) {
	var converted []*genai.Part

	for _, part := range parts {
		switch part.Kind {
		case llmx.PartText:
			if part.Text != "" {
				converted = append(converted, genai.NewPartFromText(part.Text))
			}
		case llmx.PartDocument:
			if part.Document == nil {
				continue
			}

			geminiDoc, ok := part.Document.(*document)
			if !ok {
				return nil, ErrForeignDocument
			}

			converted = append(converted, geminiDoc.part)
		case llmx.PartBytes:
			if len(part.Data) > 0 {
				converted = append(converted, genai.NewPartFromBytes(part.Data, part.MIMEType))
			}
		default:
			return nil, llmx.ErrUnsupportedPart
		}
	}

	return converted, nil
}

// systemContent returns the request's system instruction, or the provider's when the request sets none
func (p *Provider) systemContent(instruction string) *genai.Content {
	if instruction != "" {
		return genai.NewContentFromText(instruction, "")
	}

	return p.systemInstruction
}

// generateConfig returns the generation configuration, using the named cache when there is one
func (p *Provider) generateConfig(req llmx.GenerateRequest) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{
		Temperature:     new(p.temperature),
		MaxOutputTokens: p.maxOutputTokens,
	}

	if req.Temperature != nil {
		cfg.Temperature = req.Temperature
	}

	if req.MaxOutputTokens > 0 {
		cfg.MaxOutputTokens = req.MaxOutputTokens
	}

	if req.JSON || len(req.ResponseSchema) > 0 {
		cfg.ResponseMIMEType = jsonMIMEType
	}

	if len(req.ResponseSchema) > 0 {
		cfg.ResponseJsonSchema = req.ResponseSchema
	}

	if req.Cache != "" {
		cfg.CachedContent = req.Cache
	} else {
		cfg.SystemInstruction = p.systemContent(req.SystemInstruction)
	}

	return cfg
}
