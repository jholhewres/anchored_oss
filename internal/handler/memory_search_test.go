package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type searchTestStore struct {
	store.Store

	textResults   []*model.Memory
	vectorResults []*model.Memory
	textErr       error
	vectorErr     error
	textCalls     int
	vectorCalls   int
	vectorModel   string
	vectorDims    int
	staleResults  []*model.Memory
	staleErr      error
}

func (s *searchTestStore) SearchMemories(context.Context, string, string, int) ([]*model.Memory, error) {
	s.textCalls++
	return s.textResults, s.textErr
}

func (s *searchTestStore) SearchMemoriesByVector(context.Context, string, []float32, int) ([]*model.Memory, error) {
	s.vectorCalls++
	return s.vectorResults, s.vectorErr
}

func (s *searchTestStore) SearchMemoriesByVectorSpace(_ context.Context, _ string, _ []float32, model string, dims int, _ int) ([]*model.Memory, error) {
	s.vectorCalls++
	s.vectorModel = model
	s.vectorDims = dims
	return s.vectorResults, s.vectorErr
}

func (s *searchTestStore) MemoriesStaleEmbeddingSpace(context.Context, string, int, string, int) ([]*model.Memory, error) {
	return s.staleResults, s.staleErr
}

type searchTestEmbedder struct {
	err   error
	calls int
}

func (e *searchTestEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	return [][]float32{{1, 0, 0}}, nil
}

func (*searchTestEmbedder) Dimensions() int { return 3 }
func (*searchTestEmbedder) Model() string   { return "test-v1" }
func (*searchTestEmbedder) Name() string    { return "test" }

type searchResultBody struct {
	ID            string `json:"id"`
	Rank          int    `json:"rank"`
	EffectiveMode string `json:"effective_mode"`
}

func TestMemorySearch_ModeMatrix(t *testing.T) {
	result := []*model.Memory{{ID: "mem-1", ProjectID: "project-1", Content: "semantic memory"}}

	tests := []struct {
		name           string
		mode           string
		withEmbedder   bool
		wantMode       string
		wantTextCalls  int
		wantVectorCall int
		wantEmbedCalls int
	}{
		{
			name:           "missing mode defaults to semantic with embedder",
			withEmbedder:   true,
			wantMode:       "semantic",
			wantVectorCall: 1,
			wantEmbedCalls: 1,
		},
		{
			name:          "missing mode defaults to text without embedder",
			wantMode:      "text",
			wantTextCalls: 1,
		},
		{
			name:           "explicit semantic uses vector search",
			mode:           "semantic",
			withEmbedder:   true,
			wantMode:       "semantic",
			wantVectorCall: 1,
			wantEmbedCalls: 1,
		},
		{
			name:          "explicit text remains text with embedder",
			mode:          "text",
			withEmbedder:  true,
			wantMode:      "text",
			wantTextCalls: 1,
		},
		{
			name:          "unknown legacy mode remains text",
			mode:          "legacy",
			withEmbedder:  true,
			wantMode:      "text",
			wantTextCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &searchTestStore{textResults: result, vectorResults: result}
			var embedder embeddings.Embedder
			var fakeEmbedder *searchTestEmbedder
			if tt.withEmbedder {
				fakeEmbedder = &searchTestEmbedder{}
				embedder = fakeEmbedder
			}
			handler := NewMemoryHandler(st, nil, nil, embedder, slog.Default())
			rec := performMemorySearch(handler, tt.mode)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("X-Anchored-Effective-Mode"); got != tt.wantMode {
				t.Errorf("effective mode header = %q, want %q", got, tt.wantMode)
			}
			if st.textCalls != tt.wantTextCalls {
				t.Errorf("text calls = %d, want %d", st.textCalls, tt.wantTextCalls)
			}
			if st.vectorCalls != tt.wantVectorCall {
				t.Errorf("vector calls = %d, want %d", st.vectorCalls, tt.wantVectorCall)
			}
			if tt.wantVectorCall > 0 && (st.vectorModel != "test-v1" || st.vectorDims != 3) {
				t.Errorf("semantic space = model %q dims %d, want test-v1/3", st.vectorModel, st.vectorDims)
			}
			gotEmbedCalls := 0
			if fakeEmbedder != nil {
				gotEmbedCalls = fakeEmbedder.calls
			}
			if gotEmbedCalls != tt.wantEmbedCalls {
				t.Errorf("embed calls = %d, want %d", gotEmbedCalls, tt.wantEmbedCalls)
			}

			var body []searchResultBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(body) != 1 {
				t.Fatalf("result count = %d, want 1", len(body))
			}
			if body[0].ID != "mem-1" || body[0].Rank != 1 || body[0].EffectiveMode != tt.wantMode {
				t.Errorf("result metadata = %+v", body[0])
			}
		})
	}
}

func TestMemorySearch_SemanticUnavailable(t *testing.T) {
	st := &searchTestStore{}
	handler := NewMemoryHandler(st, nil, nil, nil, slog.Default())
	rec := performMemorySearch(handler, "semantic")

	assertCodedSearchError(t, rec, http.StatusUnprocessableEntity, "semantic_unavailable")
	if st.textCalls != 0 || st.vectorCalls != 0 {
		t.Fatalf("search calls after unavailable semantic request: text=%d vector=%d", st.textCalls, st.vectorCalls)
	}
}

func TestMemorySearch_SemanticUnavailableUntilIndexCoverageIsComplete(t *testing.T) {
	st := &searchTestStore{
		staleResults:  []*model.Memory{{ID: "legacy-vector"}},
		vectorResults: []*model.Memory{{ID: "must-not-return"}},
	}
	embedder := &searchTestEmbedder{}
	handler := NewMemoryHandler(st, nil, nil, embedder, slog.Default())
	rec := performMemorySearch(handler, "")

	assertCodedSearchError(t, rec, http.StatusUnprocessableEntity, "semantic_unavailable")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if embedder.calls != 0 || st.vectorCalls != 0 || st.textCalls != 0 {
		t.Fatalf(
			"search ran before semantic coverage: embed=%d vector=%d text=%d",
			embedder.calls,
			st.vectorCalls,
			st.textCalls,
		)
	}
}

func TestMemorySearch_SemanticEmbeddingFailureDoesNotFallback(t *testing.T) {
	for _, mode := range []string{"", "semantic"} {
		name := "default"
		if mode != "" {
			name = "explicit"
		}
		t.Run(name, func(t *testing.T) {
			st := &searchTestStore{
				textResults: []*model.Memory{{ID: "must-not-return"}},
			}
			embedder := &searchTestEmbedder{err: errors.New("provider unavailable")}
			handler := NewMemoryHandler(st, nil, nil, embedder, slog.Default())
			rec := performMemorySearch(handler, mode)

			assertCodedSearchError(t, rec, http.StatusServiceUnavailable, "semantic_query_failed")
			if embedder.calls != 1 {
				t.Errorf("embed calls = %d, want 1", embedder.calls)
			}
			if st.textCalls != 0 || st.vectorCalls != 0 {
				t.Fatalf("search calls after embedding failure: text=%d vector=%d", st.textCalls, st.vectorCalls)
			}
		})
	}
}

func TestMemorySearch_EmptyResponseDeclaresEffectiveMode(t *testing.T) {
	st := &searchTestStore{}
	handler := NewMemoryHandler(st, nil, nil, nil, slog.Default())
	rec := performMemorySearch(handler, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Anchored-Effective-Mode"); got != "text" {
		t.Errorf("effective mode header = %q, want text", got)
	}
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want empty JSON array", got)
	}
}

func TestMemorySearch_ResultMetadataIsBackwardCompatible(t *testing.T) {
	st := &searchTestStore{
		textResults: []*model.Memory{{
			ID:        "mem-1",
			ProjectID: "project-1",
			Category:  "fact",
			Content:   "legacy clients ignore optional result metadata",
		}},
	}
	handler := NewMemoryHandler(st, nil, nil, nil, slog.Default())
	rec := performMemorySearch(handler, "text")

	var legacyBody []*model.Memory
	if err := json.Unmarshal(rec.Body.Bytes(), &legacyBody); err != nil {
		t.Fatalf("decode response as legacy memory array: %v", err)
	}
	if len(legacyBody) != 1 || legacyBody[0].ID != "mem-1" {
		t.Fatalf("legacy response = %+v", legacyBody)
	}
}

func TestMemorySearch_RanksPreserveStoreOrder(t *testing.T) {
	st := &searchTestStore{
		vectorResults: []*model.Memory{
			{ID: "mem-c", ProjectID: "project-1"},
			{ID: "mem-a", ProjectID: "project-1"},
			{ID: "mem-b", ProjectID: "project-1"},
		},
	}
	handler := NewMemoryHandler(st, nil, nil, &searchTestEmbedder{}, slog.Default())
	rec := performMemorySearch(handler, "semantic")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body []searchResultBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for i, wantID := range []string{"mem-c", "mem-a", "mem-b"} {
		if body[i].ID != wantID || body[i].Rank != i+1 {
			t.Errorf("result %d = %+v, want id=%s rank=%d", i, body[i], wantID, i+1)
		}
	}
}

func TestMemorySearch_StoreFailureRemainsGenericServerError(t *testing.T) {
	st := &searchTestStore{textErr: errors.New("database unavailable")}
	handler := NewMemoryHandler(st, nil, nil, nil, slog.Default())
	rec := performMemorySearch(handler, "text")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "search failed" {
		t.Errorf("error = %q, want search failed", body["error"])
	}
}

func TestMemorySearch_VectorStoreFailureDoesNotFallback(t *testing.T) {
	st := &searchTestStore{
		textResults: []*model.Memory{{ID: "must-not-return"}},
		vectorErr:   errors.New("vector index unavailable"),
	}
	embedder := &searchTestEmbedder{}
	handler := NewMemoryHandler(st, nil, nil, embedder, slog.Default())
	rec := performMemorySearch(handler, "semantic")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if embedder.calls != 1 || st.vectorCalls != 1 {
		t.Errorf("semantic path calls: embed=%d vector=%d, want 1 each", embedder.calls, st.vectorCalls)
	}
	if st.textCalls != 0 {
		t.Fatalf("text calls = %d, want no lexical fallback", st.textCalls)
	}
}

func performMemorySearch(handler *MemoryHandler, mode string) *httptest.ResponseRecorder {
	target := "/v1/memories/search?project_id=project-1&q=memory"
	if mode != "" {
		target += "&mode=" + mode
	}
	ctx := context.WithValue(context.Background(), middleware.ScopeKey, "admin")
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)
	return rec
}

func assertCodedSearchError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, status, rec.Body.String())
	}
	var body codedErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != code {
		t.Errorf("error code = %q, want %q", body.Error.Code, code)
	}
	if body.Error.Message == "" {
		t.Error("error message is empty")
	}
}
