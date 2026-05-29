package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIEmbedder calls any OpenAI-compatible /v1/embeddings endpoint (OpenAI,
// z.ai, OpenRouter, ...). The base URL, model, dimension, and API key are
// supplied by config; vectors are L2-normalized before return so they share the
// same cosine space as the local embedder.
type OpenAIEmbedder struct {
	baseURL string
	model   string
	apiKey  string
	dims    int
	client  *http.Client
}

// NewOpenAIEmbedder builds an HTTP embedder. baseURL defaults to OpenAI's API
// when empty; dims must match the model's output width (e.g. 1536 for
// text-embedding-3-small).
func NewOpenAIEmbedder(baseURL, model, apiKey string, dims int) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIEmbedder{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		dims:    dims,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *OpenAIEmbedder) Dimensions() int { return e.dims }
func (e *OpenAIEmbedder) Model() string   { return e.model }
func (e *OpenAIEmbedder) Name() string    { return "openai" }

type openAIRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(openAIRequest{Model: e.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("embeddings: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embeddings: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("embeddings: decode response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := "unknown error"
		if parsed.Error != nil {
			msg = parsed.Error.Message
		}
		return nil, fmt.Errorf("embeddings: provider returned %d: %s", resp.StatusCode, msg)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("embeddings: expected %d vectors, got %d", len(texts), len(parsed.Data))
	}

	out := make([][]float32, len(parsed.Data))
	for i, d := range parsed.Data {
		out[i] = l2normalize(d.Embedding)
	}
	return out, nil
}
