package gemini

import (
	"context"
	"testing"

	"google.golang.org/genai"
	"gotest.tools/v3/assert"
)

func TestNewAppliesDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          Config
		opts            []Option
		expectedModel   string
		expectedBackend genai.Backend
		expectedSystem  bool
	}{
		{
			name:            "empty backend and model take the defaults",
			config:          Config{APIKey: "key"},
			expectedModel:   defaultModel,
			expectedBackend: genai.BackendGeminiAPI,
		},
		{
			name:            "configured model is kept",
			config:          Config{APIKey: "key"},
			opts:            []Option{WithModel("gemini-custom")},
			expectedModel:   "gemini-custom",
			expectedBackend: genai.BackendGeminiAPI,
		},
		{
			name:            "empty model and instruction keep the defaults",
			config:          Config{APIKey: "key"},
			opts:            []Option{WithModel(""), WithSystemInstruction("")},
			expectedModel:   defaultModel,
			expectedBackend: genai.BackendGeminiAPI,
		},
		{
			name:            "system instruction is set",
			config:          Config{APIKey: "key"},
			opts:            []Option{WithSystemInstruction("extract")},
			expectedModel:   defaultModel,
			expectedBackend: genai.BackendGeminiAPI,
			expectedSystem:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := New(context.Background(), tt.config, tt.opts...)

			assert.NilError(t, err)
			assert.Check(t, provider.Model() == tt.expectedModel)
			assert.Check(t, provider.backend == tt.expectedBackend)
			assert.Check(t, (provider.systemInstruction != nil) == tt.expectedSystem)
		})
	}
}

func TestNewRejectsUnprovisionedConfig(t *testing.T) {
	_, err := New(context.Background(), Config{Backend: BackendVertex, Project: "project"})

	assert.ErrorIs(t, err, ErrConfigInvalid)
}

func TestBackendGenAI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		backend  Backend
		expected genai.Backend
	}{
		{backend: BackendGemini, expected: genai.BackendGeminiAPI},
		{backend: BackendVertex, expected: genai.BackendVertexAI},
		{backend: "", expected: genai.BackendGeminiAPI},
	}

	for _, tt := range tests {
		t.Run(string(tt.backend), func(t *testing.T) {
			t.Parallel()

			assert.Check(t, tt.backend.GenAI() == tt.expected)
		})
	}
}

func TestConfigProvisioned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{name: "gemini with api key", config: Config{APIKey: "key"}, expected: true},
		{name: "gemini without api key", config: Config{Project: "project", Location: "us-central1"}},
		{name: "vertex with project and location", config: Config{Backend: BackendVertex, Project: "project", Location: "us-central1"}, expected: true},
		{name: "vertex without location", config: Config{Backend: BackendVertex, Project: "project"}},
		{name: "vertex with only an api key", config: Config{Backend: BackendVertex, APIKey: "key"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Check(t, tt.config.Provisioned() == tt.expected)
		})
	}
}
