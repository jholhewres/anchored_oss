package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AnthropicProvider talks to the Anthropic /messages API. The system prompt is
// a top-level field (not a message), and the response carries content blocks.
type AnthropicProvider struct {
	baseURL   string
	model     string
	apiKey    string
	maxTokens int
	client    *http.Client
}

func NewAnthropicProvider(baseURL, model, apiKey string, maxTokens int) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	return &AnthropicProvider{
		baseURL:   baseURL,
		model:     model,
		apiKey:    apiKey,
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *AnthropicProvider) Model() string { return p.model }
func (p *AnthropicProvider) Name() string  { return "anthropic" }

func (p *AnthropicProvider) Complete(ctx context.Context, system string, msgs []Message) (string, error) {
	payload := map[string]any{
		"model":      p.model,
		"max_tokens": p.maxTokens,
		"messages":   msgs,
	}
	if system != "" {
		payload["system"] = system
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("chat: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat: request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("chat: decode response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := "unknown error"
		if parsed.Error != nil {
			msg = parsed.Error.Message
		}
		return "", fmt.Errorf("chat: provider returned %d: %s", resp.StatusCode, msg)
	}
	if len(parsed.Content) == 0 {
		return "", fmt.Errorf("chat: provider returned no content")
	}
	return parsed.Content[0].Text, nil
}
