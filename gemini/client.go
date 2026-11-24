package gemini

import (
	"context"

	"google.golang.org/genai"
)

const (
	// defaultModel is the Gemini model used when none is configured
	defaultModel = "gemini-3.1-pro-preview"
	// defaultTemperature keeps extraction close to the document rather than inventive
	defaultTemperature = 0.1
	// defaultMaxOutputTokens caps one response
	defaultMaxOutputTokens = int32(65536)
)

// Backend names the API the Gemini client talks to
type Backend string

const (
	// BackendGemini is the Gemini Developer API, authenticated with an api key
	BackendGemini Backend = "gemini"
	// BackendVertex is Vertex AI, authenticated with Google Cloud credentials, with the ability to use model armour
	BackendVertex Backend = "vertex"
)

// GenAI maps the backend name to the genai client backend, defaulting to the Gemini Developer API
func (b Backend) GenAI() genai.Backend {
	if b == BackendVertex {
		return genai.BackendVertexAI
	}

	return genai.BackendGeminiAPI
}

// Config configures the Gemini provider
type Config struct {
	// Backend selects the API the client talks to: gemini for the Gemini Developer API or vertex for Vertex AI
	Backend Backend `json:"backend" koanf:"backend" jsonschema:"description=Gemini backend to use: gemini for the Gemini Developer API or vertex for Vertex AI,enum=gemini,enum=vertex,default=gemini"`
	// APIKey authenticates with the Gemini Developer API, on Vertex AI it enables express mode
	APIKey string `json:"apikey" koanf:"apikey" jsonschema:"description=Gemini API key" sensitive:"true"`
	// Project is the Google Cloud project, required for the Vertex AI backend
	Project string `json:"project" koanf:"project" jsonschema:"description=Google Cloud project for the Vertex AI backend"`
	// Location is the Google Cloud region, required for the Vertex AI backend
	Location string `json:"location" koanf:"location" jsonschema:"description=Google Cloud region for the Vertex AI backend, for example us-central1"`
}

// Provisioned reports whether the config has the minimum required fields to make Gemini calls
func (c Config) Provisioned() bool {
	if c.Backend == BackendVertex {
		return c.Project != "" && c.Location != ""
	}

	return c.APIKey != ""
}

// Provider runs llmx generations against Gemini
type Provider struct {
	client            *genai.Client
	backend           genai.Backend
	model             string
	temperature       float32
	maxOutputTokens   int32
	systemInstruction *genai.Content
}

// Option configures a Provider
type Option func(*Provider)

// WithModel sets the Gemini model used for content generation, an empty model keeps the default
func WithModel(model string) Option {
	return func(p *Provider) {
		if model != "" {
			p.model = model
		}
	}
}

// WithSystemInstruction sets the system prompt applied to every generation and cache, none when empty
func WithSystemInstruction(instruction string) Option {
	return func(p *Provider) {
		if instruction != "" {
			p.systemInstruction = genai.NewContentFromText(instruction, "")
		}
	}
}

// WithTemperature sets the sampling temperature, defaulting to 0.1
func WithTemperature(temperature float32) Option {
	return func(p *Provider) {
		p.temperature = temperature
	}
}

// WithMaxOutputTokens sets the token cap for one response, defaulting to 65536
func WithMaxOutputTokens(maxOutputTokens int32) Option {
	return func(p *Provider) {
		p.maxOutputTokens = maxOutputTokens
	}
}

// New creates a Gemini provider from the supplied configuration
func New(ctx context.Context, config Config, opts ...Option) (*Provider, error) {
	if !config.Provisioned() {
		return nil, ErrConfigInvalid
	}

	provider := &Provider{
		backend:         config.Backend.GenAI(),
		model:           defaultModel,
		temperature:     defaultTemperature,
		maxOutputTokens: defaultMaxOutputTokens,
	}

	for _, opt := range opts {
		opt(provider)
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:   config.APIKey,
		Backend:  provider.backend,
		Project:  config.Project,
		Location: config.Location,
	})
	if err != nil {
		return nil, err
	}

	provider.client = client

	return provider, nil
}

// Model is the model generations run against
func (p *Provider) Model() string { return p.model }
