// Package chat provides the optional internal AI chat abstraction. It is a
// pre-context helper (not the product's core): given a question, the chat
// endpoint retrieves relevant team memories via semantic search and asks a
// configured LLM provider to answer grounded in them.
//
// Providers share a credential-by-env-var convention with the embeddings
// package. Two implementations ship:
//   - "openai": any OpenAI-compatible /chat/completions endpoint (OpenAI, z.ai,
//     OpenRouter, ...).
//   - "anthropic": the Anthropic /messages API.
package chat

import "context"

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider is a chat-completion backend.
type Provider interface {
	// Complete returns the assistant's reply given a system prompt and the
	// conversation so far.
	Complete(ctx context.Context, system string, msgs []Message) (string, error)
	// Model identifies the underlying model.
	Model() string
	// Name is the provider name ("openai", "anthropic").
	Name() string
}
