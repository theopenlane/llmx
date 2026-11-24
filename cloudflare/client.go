package cloudflare

import (
	cf "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/option"
)

const (
	// defaultModel is the Workers AI model used when none is configured
	defaultModel = "@cf/google/gemma-4-26b-a4b-it"
	// defaultTemperature keeps output close to the input rather than inventive
	defaultTemperature = 0.1
	// defaultMaxOutputTokens caps one response
	defaultMaxOutputTokens = int32(4096)
)

// Config configures the Cloudflare provider
type Config struct {
	// AccountID is the Cloudflare account Workers AI runs under
	AccountID string `json:"accountid" koanf:"accountid" jsonschema:"description=Cloudflare account ID"`
	// APIKey is a Cloudflare api token with Workers AI access
	APIKey string `json:"apikey" koanf:"apikey" jsonschema:"description=Cloudflare API token with Workers AI access" sensitive:"true"`
	// BaseURL overrides the Cloudflare api endpoint, the public api when empty
	BaseURL string `json:"baseurl" koanf:"baseurl" jsonschema:"description=Cloudflare API endpoint override"`
}

// Provisioned reports whether the config has the minimum required fields to make Workers AI calls
func (c Config) Provisioned() bool {
	return c.AccountID != "" && c.APIKey != ""
}

// Provider runs llmx generations against Workers AI
type Provider struct {
	client            *cf.Client
	accountID         string
	model             string
	systemInstruction string
	temperature       float32
	maxOutputTokens   int32
	clientOptions     []option.RequestOption
}

// Option configures a Provider
type Option func(*Provider)

// WithModel sets the Workers AI model used for content generation, an empty model keeps the default
func WithModel(model string) Option {
	return func(p *Provider) {
		if model != "" {
			p.model = model
		}
	}
}

// WithSystemInstruction sets the system prompt applied to every generation, none when empty
func WithSystemInstruction(instruction string) Option {
	return func(p *Provider) {
		p.systemInstruction = instruction
	}
}

// WithTemperature sets the sampling temperature, defaulting to 0.1
func WithTemperature(temperature float32) Option {
	return func(p *Provider) {
		p.temperature = temperature
	}
}

// WithMaxOutputTokens sets the token cap for one response, defaulting to 4096
func WithMaxOutputTokens(maxOutputTokens int32) Option {
	return func(p *Provider) {
		p.maxOutputTokens = maxOutputTokens
	}
}

// WithClientOptions passes request options to the underlying Cloudflare sdk client, such as an http client
func WithClientOptions(opts ...option.RequestOption) Option {
	return func(p *Provider) {
		p.clientOptions = append(p.clientOptions, opts...)
	}
}

// New creates a Workers AI provider from the supplied configuration
func New(config Config, opts ...Option) (*Provider, error) {
	if !config.Provisioned() {
		return nil, ErrConfigInvalid
	}

	provider := &Provider{
		accountID:       config.AccountID,
		model:           defaultModel,
		temperature:     defaultTemperature,
		maxOutputTokens: defaultMaxOutputTokens,
	}

	for _, opt := range opts {
		opt(provider)
	}

	// retries are left to the caller through Classify, matching the other providers
	clientOpts := []option.RequestOption{option.WithAPIToken(config.APIKey), option.WithMaxRetries(0)}

	if config.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(config.BaseURL))
	}

	provider.client = cf.NewClient(append(clientOpts, provider.clientOptions...)...)

	return provider, nil
}

// Model is the model generations run against
func (p *Provider) Model() string { return p.model }

// completionsPath is the OpenAI compatible chat completions path for the account
func (p *Provider) completionsPath() string {
	return "accounts/" + p.accountID + "/ai/v1/chat/completions"
}
