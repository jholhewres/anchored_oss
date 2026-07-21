package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/chat"
	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/model"
)

type readinessChatProvider struct {
	calls int
}

func (p *readinessChatProvider) Complete(
	_ context.Context,
	_ string,
	_ []chat.Message,
) (string, error) {
	p.calls++
	return "grounded answer", nil
}

func (*readinessChatProvider) Model() string { return "readiness-test" }
func (*readinessChatProvider) Name() string  { return "test" }

func TestChatSemanticReadinessGateAndStatus(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	ctx := adminCtx(orgID, actorID)
	now := time.Now().UTC()
	memory := &model.Memory{
		ID:          "chat-readiness-memory",
		ProjectID:   projectID,
		Category:    "decision",
		Content:     "Use semantic retrieval for the project chat endpoint.",
		ContentHash: "sha256:chat-readiness",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	embedder := embeddings.NewLocalEmbedder(8)
	provider := &readinessChatProvider{}
	handler := &ChatHandler{
		store:    st,
		embedder: embedder,
		provider: provider,
		logger:   slog.Default(),
	}

	status := performChatStatus(handler, ctx, projectID)
	if got := chatIndexState(t, status); got != "rebuilding" {
		t.Fatalf("index state before embedding = %q, want rebuilding", got)
	}
	response := performChatComplete(handler, ctx, projectID, "What retrieval mode is used?")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("chat while rebuilding status = %d, want 422; body=%s",
			response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "semantic_unavailable") {
		t.Fatalf("chat while rebuilding body = %s, want semantic_unavailable", response.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("chat provider calls while rebuilding = %d, want 0", provider.calls)
	}

	vector, err := embeddings.EmbedOne(ctx, embedder, memory.Content)
	if err != nil {
		t.Fatalf("embed memory: %v", err)
	}
	if err := st.UpdateMemoryEmbeddingInSpace(
		ctx,
		memory.ID,
		vector,
		embeddings.SemanticSpace(embedder),
	); err != nil {
		t.Fatalf("store memory embedding: %v", err)
	}

	status = performChatStatus(handler, ctx, projectID)
	if got := chatIndexState(t, status); got != "ready" {
		t.Fatalf("index state after embedding = %q, want ready", got)
	}
	response = performChatComplete(handler, ctx, projectID, "What retrieval mode is used?")
	if response.Code != http.StatusOK {
		t.Fatalf("ready chat status = %d, want 200; body=%s",
			response.Code, response.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("ready chat provider calls = %d, want 1", provider.calls)
	}
}

func performChatStatus(
	handler *ChatHandler,
	ctx context.Context,
	projectID string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/chat/status?project_id="+projectID,
		nil,
	).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.Status(response, request)
	return response
}

func chatIndexState(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("chat status = %d, body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		IndexState string `json:"index_state"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat status: %v", err)
	}
	return payload.IndexState
}

func performChatComplete(
	handler *ChatHandler,
	ctx context.Context,
	projectID, query string,
) *httptest.ResponseRecorder {
	body, _ := json.Marshal(chatRequest{ProjectID: projectID, Query: query})
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat",
		strings.NewReader(string(body)),
	).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.Complete(response, request)
	return response
}
