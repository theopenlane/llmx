package cloudflare

import (
	"encoding/json"
	"strings"

	"github.com/theopenlane/llmx"
)

const (
	// roleSystem is the chat role of the system instruction
	roleSystem = "system"
	// roleUser is the chat role of a caller message
	roleUser = "user"
	// roleAssistant is the chat role of an earlier model response
	roleAssistant = "assistant"
	// responseSchemaName names the json schema sent with a structured request
	responseSchemaName = "response"
)

// chatRequest is the body of an OpenAI compatible chat completion request
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Stream         bool            `json:"stream"`
	StreamOptions  *streamOptions  `json:"stream_options,omitempty"`
	Temperature    float32         `json:"temperature"`
	MaxTokens      int32           `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// chatMessage is one turn of a chat completion request
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// streamOptions asks for token usage on the last streamed event
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// responseFormat asks for json output, constrained to a schema when one is set
type responseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *jsonSchemaFormat `json:"json_schema,omitempty"`
}

// jsonSchemaFormat is the schema a structured response must match
type jsonSchemaFormat struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

// chatChunk is one streamed chat completion event, or a whole completion when the request did not stream
type chatChunk struct {
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage,omitempty"`
}

// chatChoice is one candidate, carrying a delta when streamed and a message when not
type chatChoice struct {
	Delta        chatMessage `json:"delta"`
	Message      chatMessage `json:"message"`
	FinishReason *string     `json:"finish_reason"`
}

// chatUsage is the token count reported with the last streamed event
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// buildRequest converts a generation request into a chat completion request, streamed unless it asks for json
func (p *Provider) buildRequest(req llmx.GenerateRequest) (chatRequest, error) {
	if req.Cache != "" {
		return chatRequest{}, ErrCacheUnsupported
	}

	body := chatRequest{
		Model:         p.model,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
		Temperature:   p.temperature,
		MaxTokens:     p.maxOutputTokens,
	}

	if req.Model != "" {
		body.Model = req.Model
	}

	if req.Temperature != nil {
		body.Temperature = *req.Temperature
	}

	if req.MaxOutputTokens > 0 {
		body.MaxTokens = req.MaxOutputTokens
	}

	switch {
	case len(req.ResponseSchema) > 0:
		body.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: &jsonSchemaFormat{Name: responseSchemaName, Schema: req.ResponseSchema, Strict: true},
		}
	case req.JSON:
		body.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	// Workers AI json mode does not support streaming, so a json request returns one whole completion
	if body.ResponseFormat != nil {
		body.Stream = false
		body.StreamOptions = nil
	}

	system := p.systemInstruction
	if req.SystemInstruction != "" {
		system = req.SystemInstruction
	}

	if system != "" {
		body.Messages = append(body.Messages, chatMessage{Role: roleSystem, Content: system})
	}

	for _, message := range req.Messages {
		converted, ok, err := chatMessageFrom(message)
		if err != nil {
			return chatRequest{}, err
		}

		if ok {
			body.Messages = append(body.Messages, converted)
		}
	}

	return body, nil
}

// chatMessageFrom joins a message's text parts, reporting false when there is no text to send
func chatMessageFrom(message llmx.Message) (chatMessage, bool, error) {
	texts := make([]string, 0, len(message.Parts))

	for _, part := range message.Parts {
		switch part.Kind {
		case llmx.PartText:
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		case llmx.PartDocument:
			if part.Document != nil {
				return chatMessage{}, false, ErrDocumentsUnsupported
			}
		case llmx.PartBytes:
			return chatMessage{}, false, ErrDocumentsUnsupported
		default:
			return chatMessage{}, false, llmx.ErrUnsupportedPart
		}
	}

	if len(texts) == 0 {
		return chatMessage{}, false, nil
	}

	role := roleUser
	if message.Role == llmx.RoleModel {
		role = roleAssistant
	}

	return chatMessage{Role: role, Content: strings.Join(texts, "\n")}, true, nil
}

// responseChunk converts a streamed event into a chunk
func responseChunk(event chatChunk) llmx.Chunk {
	var chunk llmx.Chunk

	if event.Usage != nil {
		chunk.Usage = &llmx.Usage{InputTokens: event.Usage.PromptTokens, OutputTokens: event.Usage.CompletionTokens}
	}

	if len(event.Choices) == 0 {
		return chunk
	}

	choice := event.Choices[0]
	chunk.Text = choice.Delta.Content + choice.Message.Content

	if choice.FinishReason != nil {
		chunk.FinishReason = finishReason(*choice.FinishReason)
	}

	return chunk
}

// finishReason maps an OpenAI compatible finish reason onto the provider-neutral one
func finishReason(reason string) llmx.FinishReason {
	switch reason {
	case "":
		return llmx.FinishUnknown
	case "stop":
		return llmx.FinishStop
	case "length":
		return llmx.FinishMaxTokens
	case "content_filter":
		return llmx.FinishFiltered
	default:
		return llmx.FinishOther
	}
}
