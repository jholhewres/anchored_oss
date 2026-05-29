package chat

import (
	"fmt"
	"os"
	"strings"
)

// Config selects and parameterizes the chat provider. Disabled by default —
// the chat feature is opt-in. APIKeyEnv names the env var holding the key so
// secrets never land in config files.
type Config struct {
	Enabled   bool   `yaml:"enabled"`
	Provider  string `yaml:"provider"` // "openai" | "anthropic"
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	MaxTokens int    `yaml:"max_tokens"`
}

// DefaultConfig returns the disabled default.
func DefaultConfig() Config {
	return Config{Enabled: false, MaxTokens: 1024}
}

// New builds a Provider from config. Returns (nil, nil) when chat is disabled
// so callers can treat it as optional.
func New(cfg Config) (Provider, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("chat: a model is required when enabled")
	}
	apiKey := ""
	if cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("chat: env var %q is empty or unset (required when api_key_env is set)", cfg.APIKeyEnv)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "openai":
		return NewOpenAIProvider(cfg.BaseURL, cfg.Model, apiKey, cfg.MaxTokens), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.BaseURL, cfg.Model, apiKey, cfg.MaxTokens), nil
	default:
		return nil, fmt.Errorf("chat: unknown provider %q", cfg.Provider)
	}
}
