package embeddings

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Config selects and parameterizes the embedder. Provider is "local" (default,
// dependency-free lexical hashing), "onnx" (the real local semantic model,
// paraphrase-multilingual-MiniLM-L12-v2), "openai" (any OpenAI-compatible
// endpoint), or "none" to disable embedding entirely. APIKeyEnv names the
// environment variable holding the provider key so the secret never lands in
// config files.
type Config struct {
	Provider   string `yaml:"provider"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
	BaseURL    string `yaml:"base_url"`
	APIKeyEnv  string `yaml:"api_key_env"`
	// ModelDir is where the "onnx" provider stores its model files (under a
	// per-model subdir) and the runtime .so (in a sibling "lib" directory).
	// Both are downloaded on first run if absent. Ignored by other providers.
	// Defaults to "models" (relative to the working directory).
	ModelDir string `yaml:"model_dir"`
}

// DefaultConfig returns the zero-config local embedder at 384 dimensions, which
// matches the ONNX model width and the pgvector column.
func DefaultConfig() Config {
	return Config{
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 384,
		ModelDir:   "models",
	}
}

// New builds an Embedder from config. Returns (nil, nil) when disabled so
// callers can treat embeddings as optional. Returns an error for an unknown
// provider or a misconfigured remote provider. logger may be nil (the onnx
// provider falls back to slog.Default()).
func New(cfg Config, logger *slog.Logger) (Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "local":
		return NewLocalEmbedder(cfg.Dimensions), nil
	case "none", "disabled":
		return nil, nil
	case "onnx":
		modelDir := cfg.ModelDir
		if modelDir == "" {
			modelDir = "models"
		}
		// newONNXEmbedder is build-tag switched: the real implementation
		// (onnx.go) requires `-tags onnx` and CGO because onnxruntime_go uses
		// cgo; the default build links a stub (onnx_stub.go) that returns a
		// clear error, keeping the default binary pure-Go/static.
		return newONNXEmbedder(modelDir, logger)
	case "openai":
		if cfg.Model == "" {
			return nil, fmt.Errorf("embeddings: openai provider requires a model")
		}
		if cfg.Dimensions <= 0 {
			return nil, fmt.Errorf("embeddings: openai provider requires dimensions > 0")
		}
		apiKey := ""
		if cfg.APIKeyEnv != "" {
			apiKey = os.Getenv(cfg.APIKeyEnv)
			if apiKey == "" {
				return nil, fmt.Errorf("embeddings: env var %q is empty or unset (required for openai provider)", cfg.APIKeyEnv)
			}
		}
		return NewOpenAIEmbedder(cfg.BaseURL, cfg.Model, apiKey, cfg.Dimensions), nil
	default:
		return nil, fmt.Errorf("embeddings: unknown provider %q", cfg.Provider)
	}
}
