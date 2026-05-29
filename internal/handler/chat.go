package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/ai/chat"
	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// ChatHandler implements the optional RAG chat endpoint. It retrieves relevant
// team memories via semantic search and asks the configured LLM to answer
// grounded in them, citing the memories used. Both the embedder and the chat
// provider are optional; when either is missing the endpoint reports the
// feature as disabled rather than erroring opaquely.
type ChatHandler struct {
	store    store.Store
	embedder embeddings.Embedder
	provider chat.Provider
	logger   *slog.Logger
}

// NewChatHandler wires the optional RAG chat routes. embedder is the
// process-shared embedder (injected, may be nil); the chat provider is built
// here from cfg.Chat. When either is nil the endpoint reports the feature as
// disabled.
func NewChatHandler(st store.Store, cfg *config.Config, embedder embeddings.Embedder, logger *slog.Logger) *ChatHandler {
	provider, err := chat.New(cfg.Chat)
	if err != nil {
		logger.Error("chat handler: provider disabled (config error)", "error", err)
	}
	return &ChatHandler{store: st, embedder: embedder, provider: provider, logger: logger}
}

type chatRequest struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
}

type chatSource struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Snippet  string `json:"snippet"`
}

type chatResponse struct {
	Answer  string       `json:"answer"`
	Sources []chatSource `json:"sources"`
}

func (h *ChatHandler) Complete(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		jsonError(w, http.StatusServiceUnavailable, "chat is not enabled on this server")
		return
	}
	if h.embedder == nil {
		jsonError(w, http.StatusServiceUnavailable, "embeddings are required for chat retrieval")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.ProjectID == "" || req.Query == "" {
		jsonError(w, http.StatusBadRequest, "project_id and query are required")
		return
	}
	if !h.checkAccess(w, r, req.ProjectID) {
		return
	}

	vec, err := embeddings.EmbedOne(r.Context(), h.embedder, req.Query)
	if err != nil {
		h.logger.Error("chat: embed query failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "retrieval failed")
		return
	}
	mems, err := h.store.SearchMemoriesByVector(r.Context(), req.ProjectID, vec, 8)
	if err != nil {
		h.logger.Error("chat: retrieval failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "retrieval failed")
		return
	}

	var ctxBuilder strings.Builder
	sources := make([]chatSource, 0, len(mems))
	for i, m := range mems {
		fmt.Fprintf(&ctxBuilder, "[%d] (%s) %s\n", i+1, m.Category, m.Content)
		sources = append(sources, chatSource{ID: m.ID, Category: m.Category, Snippet: truncateRunes(m.Content, 160)})
	}

	system := "You are an assistant for a software team. Answer the user's question using ONLY the numbered team memories below. " +
		"Cite the memories you used as [n]. If the memories don't contain the answer, say so plainly.\n\nTEAM MEMORIES:\n" + ctxBuilder.String()

	answer, err := h.provider.Complete(r.Context(), system, []chat.Message{{Role: chat.RoleUser, Content: req.Query}})
	if err != nil {
		h.logger.Error("chat: provider failed", "error", err)
		jsonError(w, http.StatusBadGateway, "chat provider error")
		return
	}

	jsonResponse(w, http.StatusOK, chatResponse{Answer: answer, Sources: sources})
}

// Status reports whether the chat feature is usable so the dashboard can
// show or hide the chat UI without probing the completion endpoint.
func (h *ChatHandler) Status(w http.ResponseWriter, r *http.Request) {
	enabled := h.provider != nil && h.embedder != nil
	model := ""
	if h.provider != nil {
		model = h.provider.Model()
	}
	jsonResponse(w, http.StatusOK, map[string]any{"enabled": enabled, "model": model})
}

func (h *ChatHandler) checkAccess(w http.ResponseWriter, r *http.Request, projectID string) bool {
	if middleware.GetScope(r.Context()) == "admin" {
		return true
	}
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "missing account context")
		return false
	}
	ok, err := h.store.HasProjectAccess(r.Context(), accountID, projectID)
	if err != nil {
		h.logger.Error("chat: access check failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "access check failed")
		return false
	}
	if !ok {
		jsonError(w, http.StatusForbidden, "no team access to this project")
		return false
	}
	return true
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
