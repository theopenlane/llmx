package cloudflare

import (
	"context"
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/theopenlane/llmx"
)

type fakeDocument struct{}

func (fakeDocument) Release(context.Context) {}

func TestBuildRequest(t *testing.T) {
	provider := &Provider{model: defaultModel, systemInstruction: "be brief", temperature: 0.1, maxOutputTokens: 100}

	body, err := provider.buildRequest(llmx.GenerateRequest{
		Messages: []llmx.Message{
			llmx.UserMessage(llmx.TextPart("first"), llmx.TextPart("second")),
			{Role: llmx.RoleModel, Parts: []llmx.Part{llmx.TextPart("answer")}},
			llmx.UserMessage(llmx.TextPart("")),
		},
	})

	assert.NilError(t, err)
	assert.Check(t, body.Stream)
	assert.Check(t, body.Model == defaultModel)
	assert.Check(t, body.ResponseFormat == nil)
	if !assert.Check(t, len(body.Messages) == 3) {
		return
	}

	assert.Check(t, body.Messages[0] == chatMessage{Role: roleSystem, Content: "be brief"})
	assert.Check(t, body.Messages[1] == chatMessage{Role: roleUser, Content: "first\nsecond"})
	assert.Check(t, body.Messages[2] == chatMessage{Role: roleAssistant, Content: "answer"})
}

func TestBuildRequestOverrides(t *testing.T) {
	temperature := float32(0.9)
	provider := &Provider{model: defaultModel, temperature: 0.1, maxOutputTokens: 100}

	body, err := provider.buildRequest(llmx.GenerateRequest{Model: "@cf/other", Temperature: &temperature, MaxOutputTokens: 10})

	assert.NilError(t, err)
	assert.Check(t, body.Model == "@cf/other")
	assert.Check(t, body.Temperature == temperature)
	assert.Check(t, body.MaxTokens == 10)
	assert.Check(t, len(body.Messages) == 0)
}

func TestBuildRequestSystemOverride(t *testing.T) {
	provider := &Provider{model: defaultModel, systemInstruction: "be brief"}

	body, err := provider.buildRequest(llmx.GenerateRequest{
		SystemInstruction: "summarize",
		Messages:          []llmx.Message{llmx.UserMessage(llmx.TextPart("text"))},
	})

	assert.NilError(t, err)

	if !assert.Check(t, len(body.Messages) == 2) {
		return
	}

	assert.Check(t, body.Messages[0] == chatMessage{Role: roleSystem, Content: "summarize"})
}

func TestBuildRequestResponseFormat(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{"type":"object"}`)

	tests := []struct {
		name         string
		req          llmx.GenerateRequest
		expectedType string
	}{
		{name: "schema", req: llmx.GenerateRequest{ResponseSchema: schema}, expectedType: "json_schema"},
		{name: "schema wins over json mode", req: llmx.GenerateRequest{ResponseSchema: schema, JSON: true}, expectedType: "json_schema"},
		{name: "json mode", req: llmx.GenerateRequest{JSON: true}, expectedType: "json_object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, err := (&Provider{}).buildRequest(tt.req)

			assert.NilError(t, err)
			assert.Check(t, body.ResponseFormat != nil && body.ResponseFormat.Type == tt.expectedType)
			assert.Check(t, !body.Stream && body.StreamOptions == nil)
		})
	}
}

func TestResponseChunkReadsWholeCompletion(t *testing.T) {
	reason := "length"

	chunk := responseChunk(chatChunk{Choices: []chatChoice{{Message: chatMessage{Content: "partial"}, FinishReason: &reason}}})

	assert.Check(t, chunk.Text == "partial")
	assert.Check(t, chunk.FinishReason == llmx.FinishMaxTokens)
}

func TestBuildRequestRejectsUnsupportedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         llmx.GenerateRequest
		expectedErr error
	}{
		{name: "document part", req: llmx.GenerateRequest{Messages: []llmx.Message{llmx.UserMessage(llmx.DocumentPart(fakeDocument{}))}}, expectedErr: ErrDocumentsUnsupported},
		{name: "bytes part", req: llmx.GenerateRequest{Messages: []llmx.Message{llmx.UserMessage(llmx.BytesPart([]byte("%PDF"), "application/pdf"))}}, expectedErr: ErrDocumentsUnsupported},
		{name: "unknown part kind", req: llmx.GenerateRequest{Messages: []llmx.Message{llmx.UserMessage(llmx.Part{Kind: llmx.PartKind(99)})}}, expectedErr: llmx.ErrUnsupportedPart},
		{name: "cache", req: llmx.GenerateRequest{Cache: "cached"}, expectedErr: ErrCacheUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := (&Provider{}).buildRequest(tt.req)

			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestFinishReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason   string
		expected llmx.FinishReason
	}{
		{reason: "", expected: llmx.FinishUnknown},
		{reason: "stop", expected: llmx.FinishStop},
		{reason: "length", expected: llmx.FinishMaxTokens},
		{reason: "content_filter", expected: llmx.FinishFiltered},
		{reason: "tool_calls", expected: llmx.FinishOther},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, finishReason(tt.reason) == tt.expected)
		})
	}
}
