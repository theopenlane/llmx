package cloudflare

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		config        Config
		opts          []Option
		expectedModel string
		expectedErr   error
	}{
		{name: "defaults the model", config: Config{AccountID: "acct", APIKey: "key"}, expectedModel: defaultModel},
		{name: "keeps the configured model", config: Config{AccountID: "acct", APIKey: "key"}, opts: []Option{WithModel("@cf/custom")}, expectedModel: "@cf/custom"},
		{name: "empty model keeps the default", config: Config{AccountID: "acct", APIKey: "key"}, opts: []Option{WithModel("")}, expectedModel: defaultModel},
		{name: "missing account id", config: Config{APIKey: "key"}, expectedErr: ErrConfigInvalid},
		{name: "missing api key", config: Config{AccountID: "acct"}, expectedErr: ErrConfigInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := New(tt.config, tt.opts...)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)

				return
			}

			assert.NilError(t, err)
			assert.Check(t, provider.Model() == tt.expectedModel)
		})
	}
}

func TestNewAppliesSystemInstruction(t *testing.T) {
	provider, err := New(Config{AccountID: "acct", APIKey: "key"}, WithSystemInstruction("be brief"))

	assert.NilError(t, err)
	assert.Check(t, provider.systemInstruction == "be brief")
	assert.Check(t, provider.completionsPath() == "accounts/acct/ai/v1/chat/completions")
}
