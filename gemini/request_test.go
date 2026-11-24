package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/genai"
	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

func TestRequestParts(t *testing.T) {
	t.Parallel()

	doc := &document{part: genai.NewPartFromText("doc")}

	tests := []struct {
		name        string
		parts       []llmx.Part
		expected    int
		expectedErr error
	}{
		{name: "cached document sends only the prompt", parts: []llmx.Part{llmx.TextPart("prompt")}, expected: 1},
		{name: "document and prompt", parts: []llmx.Part{llmx.DocumentPart(doc), llmx.TextPart("prompt")}, expected: 2},
		{name: "document without a prompt", parts: []llmx.Part{llmx.DocumentPart(doc)}, expected: 1},
		{name: "neither document nor prompt", parts: []llmx.Part{llmx.DocumentPart(nil), llmx.TextPart("")}},
		{name: "inline bytes", parts: []llmx.Part{llmx.BytesPart([]byte("%PDF"), "application/pdf"), llmx.TextPart("prompt")}, expected: 2},
		{name: "empty bytes are skipped", parts: []llmx.Part{llmx.BytesPart(nil, "application/pdf")}},
		{name: "document from another provider is rejected", parts: []llmx.Part{llmx.DocumentPart(&foreignDocument{}), llmx.TextPart("prompt")}, expectedErr: ErrForeignDocument},
		{name: "unknown kind is rejected", parts: []llmx.Part{{Kind: llmx.PartKind(99)}}, expectedErr: llmx.ErrUnsupportedPart},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parts, err := requestParts(tt.parts)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)

				return
			}

			assert.NilError(t, err)
			assert.Check(t, len(parts) == tt.expected)
		})
	}
}

type foreignDocument struct{}

func (foreignDocument) Release(context.Context) {}

func TestRequestContents(t *testing.T) {
	contents, err := requestContents([]llmx.Message{
		llmx.UserMessage(llmx.TextPart("question")),
		{Role: llmx.RoleModel, Parts: []llmx.Part{llmx.TextPart("answer")}},
		llmx.UserMessage(),
	})

	assert.NilError(t, err)
	if !assert.Check(t, len(contents) == 2) {
		return
	}

	assert.Check(t, contents[0].Role == genai.RoleUser)
	assert.Check(t, contents[1].Role == genai.RoleModel)
}

func TestRequestContentsRejectsForeignDocument(t *testing.T) {
	_, err := requestContents([]llmx.Message{llmx.UserMessage(llmx.DocumentPart(&foreignDocument{}))})

	assert.ErrorIs(t, err, ErrForeignDocument)
}

func TestResponseChunk(t *testing.T) {
	chunk := responseChunk(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []*genai.Part{{Text: "a"}, nil, {Text: "b"}}},
			FinishReason: genai.FinishReasonMaxTokens,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        10,
			CachedContentTokenCount: 6,
			CandidatesTokenCount:    4,
			ThoughtsTokenCount:      3,
		},
	})

	assert.Check(t, chunk.Text == "ab")
	assert.Check(t, chunk.FinishReason == llmx.FinishMaxTokens)
	if !assert.Check(t, chunk.Usage != nil) {
		return
	}

	assert.Check(t, chunk.Usage.InputTokens == 10)
	assert.Check(t, chunk.Usage.CachedInputTokens == 6)
	assert.Check(t, chunk.Usage.OutputTokens == 7)
}

func TestFinishReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason   genai.FinishReason
		expected llmx.FinishReason
	}{
		{reason: "", expected: llmx.FinishUnknown},
		{reason: genai.FinishReasonStop, expected: llmx.FinishStop},
		{reason: genai.FinishReasonMaxTokens, expected: llmx.FinishMaxTokens},
		{reason: genai.FinishReasonSafety, expected: llmx.FinishFiltered},
		{reason: genai.FinishReasonMalformedFunctionCall, expected: llmx.FinishOther},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			t.Parallel()

			assert.Check(t, finishReason(tt.reason) == tt.expected)
		})
	}
}

func TestGenerateConfigSendsJSONSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)

	cfg := (&Provider{}).generateConfig(llmx.GenerateRequest{ResponseSchema: schema})

	sent, ok := cfg.ResponseJsonSchema.(json.RawMessage)
	assert.Check(t, ok && string(sent) == string(schema))
	assert.Check(t, cfg.ResponseSchema == nil)
	assert.Check(t, cfg.ResponseMIMEType == "application/json")
}

func TestGenerateConfigSystemInstruction(t *testing.T) {
	t.Parallel()

	provider := &Provider{systemInstruction: genai.NewContentFromText("provider default", "")}

	tests := []struct {
		name     string
		req      llmx.GenerateRequest
		expected string
	}{
		{name: "provider default", req: llmx.GenerateRequest{}, expected: "provider default"},
		{name: "request override", req: llmx.GenerateRequest{SystemInstruction: "summarize"}, expected: "summarize"},
		{name: "cache holds the instruction", req: llmx.GenerateRequest{Cache: "cachedContents/1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := provider.generateConfig(tt.req)
			if tt.expected == "" {
				assert.Check(t, cfg.SystemInstruction == nil)

				return
			}

			if !assert.Check(t, cfg.SystemInstruction != nil && len(cfg.SystemInstruction.Parts) == 1) {
				return
			}

			assert.Check(t, cfg.SystemInstruction.Parts[0].Text == tt.expected)
		})
	}
}

func TestGenerateConfigOmitsMissingSchema(t *testing.T) {
	cfg := (&Provider{}).generateConfig(llmx.GenerateRequest{})

	assert.Check(t, cfg.ResponseJsonSchema == nil)
	assert.Check(t, cfg.ResponseMIMEType == "")
}

func TestGenerateConfigAppliesOverrides(t *testing.T) {
	temperature := float32(0.7)

	cfg := (&Provider{temperature: 0.1, maxOutputTokens: 100}).generateConfig(llmx.GenerateRequest{
		JSON:            true,
		Temperature:     &temperature,
		MaxOutputTokens: 50,
	})

	assert.Check(t, *cfg.Temperature == temperature)
	assert.Check(t, cfg.MaxOutputTokens == 50)
	assert.Check(t, cfg.ResponseMIMEType == "application/json")
}
