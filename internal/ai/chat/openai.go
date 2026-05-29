package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIProvider talks to any OpenAI-compatible /chat/completions endpoint
// (OpenAI, z.ai, OpenRouter, ...).
type OpenAIProvider struct {
	baseURL   string
	model     string
	apiKey    string
	maxTokens int
	client    *http.Client
}

func NewOpenAIProvider(baseURL, model, apiKey string, maxTokens int) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	return &OpenAIProvider{
		baseURL:   baseURL,
		model:     model,
		apiKey:    apiKey,
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenAIProvider) Model() string { return p.model }
func (p *OpenAIProvider) Name() string  { return "openai" }

func (p *OpenAIProvider) Complete(ctx context.Context, system string, msgs []Message) (string, error) {
	all := make([]Message, 0, len(msgs)+1)
	if system != "" {
		all = append(all, Message{Role: RoleSystem, Content: system})
	}
	all = append(all, msgs...)

	body, err := json.Marshal(map[string]any{
		"model":      p.model,
		"messages":   all,
		"max_tokens": p.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("chat: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat: request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
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
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("chat: provider returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
