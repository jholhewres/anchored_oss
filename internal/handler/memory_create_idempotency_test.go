package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
)

func TestMemoryCreate_IdempotentReplayAndConflict(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Postgres backs the remote memory service","keywords":["postgres"]}`

	first := performMemoryCreate(handler, orgID, actorID, "operation-1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}
	var firstMemory model.Memory
	if err := json.Unmarshal(first.Body.Bytes(), &firstMemory); err != nil {
		t.Fatalf("decode initial memory: %v", err)
	}
	assertMemoryCreateCreated(t, first)

	updated := firstMemory
	updated.Content = "the current memory changed after the original operation"
	updated.ContentHash = "sha256:changed"
	updated.UpdatedAt = updated.UpdatedAt.Add(time.Second)
	if err := st.UpsertMemory(adminCtx(orgID, actorID), &updated); err != nil {
		t.Fatalf("update current memory before replay: %v", err)
	}

	replay := performMemoryCreate(handler, orgID, actorID, "operation-1", body)
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, body=%s", replay.Code, replay.Body.String())
	}
	if got := replay.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Errorf("replay header = %q, want true", got)
	}
	var replayMemory model.Memory
	if err := json.Unmarshal(replay.Body.Bytes(), &replayMemory); err != nil {
		t.Fatalf("decode replay memory: %v", err)
	}
	if replayMemory.ID != firstMemory.ID || !replayMemory.CreatedAt.Equal(firstMemory.CreatedAt) {
		t.Errorf("replay = %+v, want original %+v", replayMemory, firstMemory)
	}
	if replayMemory.Content != firstMemory.Content {
		t.Errorf("replay content = %q, want original snapshot %q", replayMemory.Content, firstMemory.Content)
	}
	assertMemoryCreateCreated(t, replay)

	conflictBody := `{"project_id":"` + projectID + `","category":"fact","content":"different payload"}`
	conflict := performMemoryCreate(handler, orgID, actorID, "operation-1", conflictBody)
	assertCodedSearchError(t, conflict, http.StatusConflict, "idempotency_conflict")

	_, total, err := st.ListMemoriesPaginated(adminCtx(orgID, actorID), projectID, 10, 0, "")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 1 {
		t.Fatalf("memory total = %d, want 1", total)
	}
}

func TestMemoryCreate_IdempotencyConcurrentRequests(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"decision","content":"Use semantic retrieval by default"}`

	const callers = 12
	responses := make(chan *httptest.ResponseRecorder, callers)
	var start sync.WaitGroup
	start.Add(1)
	for i := 0; i < callers; i++ {
		go func() {
			start.Wait()
			responses <- performMemoryCreate(handler, orgID, actorID, "operation-concurrent", body)
		}()
	}
	start.Done()

	var originalID string
	for i := 0; i < callers; i++ {
		response := <-responses
		if response.Code != http.StatusCreated {
			t.Fatalf("concurrent status = %d, body=%s", response.Code, response.Body.String())
		}
		var memory model.Memory
		if err := json.Unmarshal(response.Body.Bytes(), &memory); err != nil {
			t.Fatalf("decode concurrent memory: %v", err)
		}
		if originalID == "" {
			originalID = memory.ID
		}
		if memory.ID != originalID {
			t.Fatalf("concurrent memory ID = %q, want %q", memory.ID, originalID)
		}
	}

	_, total, err := st.ListMemoriesPaginated(
		adminCtx(orgID, actorID),
		projectID,
		20,
		0,
		"",
	)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 1 {
		t.Fatalf("memory total = %d, want 1", total)
	}
}

func TestMemoryCreate_DeletedProjectRedactsReplaySnapshot(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Replay survives project lifecycle changes"}`

	first := performMemoryCreate(handler, orgID, actorID, "operation-deleted-project", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}
	if err := st.SoftDeleteProject(adminCtx(orgID, actorID), projectID); err != nil {
		t.Fatalf("soft delete project: %v", err)
	}

	replay := performMemoryCreate(handler, orgID, actorID, "operation-deleted-project", body)
	if replay.Code != http.StatusGone {
		t.Fatalf("replay after project deletion status = %d, want 410; body=%s",
			replay.Code, replay.Body.String())
	}
	if strings.Contains(replay.Body.String(), "Replay survives project lifecycle changes") {
		t.Fatalf("deleted project replay leaked original content: %s", replay.Body.String())
	}

	blocked := performMemoryCreate(handler, orgID, actorID, "new-operation", body)
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("new operation after project deletion status = %d, want 404; body=%s",
			blocked.Code, blocked.Body.String())
	}
}

func TestMemoryCreate_DeletedMemoryReplayDoesNotResurrectOrLeak(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	const secret = "deleted replay content must be redacted"
	body := `{"project_id":"` + projectID + `","category":"fact","content":"` + secret + `"}`

	first := performMemoryCreate(handler, orgID, actorID, "operation-deleted-memory", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}
	var created createMemoryResponse
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode initial response: %v", err)
	}
	if created.Memory == nil {
		t.Fatal("initial response has no memory")
	}
	if err := st.SoftDeleteMemory(
		adminCtx(orgID, actorID),
		created.Memory.ID,
		projectID,
	); err != nil {
		t.Fatalf("soft delete memory: %v", err)
	}

	replay := performMemoryCreate(handler, orgID, actorID, "operation-deleted-memory", body)
	if replay.Code != http.StatusGone {
		t.Fatalf("replay after memory deletion status = %d, want 410; body=%s",
			replay.Code, replay.Body.String())
	}
	if strings.Contains(replay.Body.String(), secret) {
		t.Fatalf("deleted memory replay leaked original content: %s", replay.Body.String())
	}
	_, total, err := st.ListMemoriesPaginated(
		adminCtx(orgID, actorID),
		projectID,
		20,
		0,
		"",
	)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 0 {
		t.Fatalf("live memories after deleted replay = %d, want 0", total)
	}
}

func TestMemoryCreate_ClientIDCannotCrossOrganizationBoundary(t *testing.T) {
	st := newProjectTestStore(t)
	orgA, actorA, projectA := seedProject(t, st)
	ctx := context.Background()
	orgB, err := st.CreateOrganization(ctx, "Other", "other")
	if err != nil {
		t.Fatalf("create second organization: %v", err)
	}
	actorB, err := st.CreateAccount(ctx, "b@other.test", "B", "h")
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}
	projectB, err := st.CreateProject(
		ctx,
		orgB.ID,
		"Other Repo",
		"other-repo",
		"other-key",
		"",
		"",
		actorB.ID,
		"other",
	)
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	const memoryID = "globally-scoped-client-id"
	const originalContent = "organization A owns this private content"

	first := performMemoryCreate(
		handler,
		orgA,
		actorA,
		"",
		`{"id":"`+memoryID+`","project_id":"`+projectA+`","category":"fact","content":"`+originalContent+`"}`,
	)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}
	conflict := performMemoryCreate(
		handler,
		orgB.ID,
		actorB.ID,
		"",
		`{"id":"`+memoryID+`","project_id":"`+projectB.ID+`","category":"fact","content":"organization B replacement"}`,
	)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("cross-organization status = %d, want 409; body=%s",
			conflict.Code, conflict.Body.String())
	}
	if strings.Contains(conflict.Body.String(), originalContent) {
		t.Fatalf("cross-organization conflict leaked original content: %s", conflict.Body.String())
	}

	stored, err := st.GetMemoryByID(adminCtx(orgA, actorA), memoryID)
	if err != nil {
		t.Fatalf("load original memory: %v", err)
	}
	if stored.ProjectID != projectA || stored.Content != originalContent {
		t.Fatalf(
			"cross-organization request mutated original: project=%q content=%q",
			stored.ProjectID,
			stored.Content,
		)
	}
}

func TestMemoryCreate_EarlyReplayBypassesChangedQuota(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	cfg := config.DefaultConfig()
	handler := NewMemoryHandler(st, policy.NewContentFilter(), cfg, nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Replay survives quota changes"}`

	first := performMemoryCreate(handler, orgID, actorID, "operation-quota", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}
	cfg.Quota.MaxStorageBytes = 1

	replay := performMemoryCreate(handler, orgID, actorID, "operation-quota", body)
	assertMemoryCreateReplay(t, first, replay)

	blocked := performMemoryCreate(handler, orgID, actorID, "new-operation", body)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("new operation above quota status = %d, want 403; body=%s",
			blocked.Code, blocked.Body.String())
	}
}

func TestMemoryCreate_EarlyReplayBypassesChangedContentPolicy(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	cfg := config.DefaultConfig()
	initial := NewMemoryHandler(st, policy.NewContentFilter(), cfg, nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Replay survives policy changes"}`

	first := performMemoryCreate(initial, orgID, actorID, "operation-policy", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}

	blockingFilter := policy.NewContentFilterFromConfig(policy.Config{
		BlockedCategories: []string{"fact"},
		QualityThreshold:  policy.RemoteQualityThreshold,
	})
	changed := NewMemoryHandler(st, blockingFilter, cfg, nil, slog.Default())
	replay := performMemoryCreate(changed, orgID, actorID, "operation-policy", body)
	assertMemoryCreateReplay(t, first, replay)

	blocked := performMemoryCreate(changed, orgID, actorID, "new-operation", body)
	if blocked.Code != http.StatusBadRequest {
		t.Fatalf("new operation under changed policy status = %d, want 400; body=%s",
			blocked.Code, blocked.Body.String())
	}
}

func TestMemoryCreate_ProjectClaimParticipatesFullyInIdempotencyHash(t *testing.T) {
	tests := []struct {
		name    string
		changed string
	}{
		{
			name:    "name",
			changed: `{"project_claim":{"name":"Renamed","remote_key":"claim-key","git_host":"github.com","repo_slug":"org/repo"},"category":"fact","content":"Claim identity is immutable"}`,
		},
		{
			name:    "remote key",
			changed: `{"project_claim":{"name":"Claim Repo","remote_key":"different-key","git_host":"github.com","repo_slug":"org/repo"},"category":"fact","content":"Claim identity is immutable"}`,
		},
		{
			name:    "git host",
			changed: `{"project_claim":{"name":"Claim Repo","remote_key":"claim-key","git_host":"gitlab.com","repo_slug":"org/repo"},"category":"fact","content":"Claim identity is immutable"}`,
		},
		{
			name:    "repo slug",
			changed: `{"project_claim":{"name":"Claim Repo","remote_key":"claim-key","git_host":"github.com","repo_slug":"org/other"},"category":"fact","content":"Claim identity is immutable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newProjectTestStore(t)
			orgID, actorID, _ := seedProject(t, st)
			handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
			original := `{"project_claim":{"name":"Claim Repo","remote_key":"claim-key","git_host":"github.com","repo_slug":"org/repo"},"category":"fact","content":"Claim identity is immutable"}`

			first := performMemoryCreate(handler, orgID, actorID, "operation-claim", original)
			if first.Code != http.StatusCreated {
				t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
			}
			replay := performMemoryCreate(handler, orgID, actorID, "operation-claim", original)
			assertMemoryCreateReplay(t, first, replay)

			conflict := performMemoryCreate(handler, orgID, actorID, "operation-claim", tt.changed)
			assertCodedSearchError(t, conflict, http.StatusConflict, "idempotency_conflict")
		})
	}
}

func TestMemoryCreate_WithoutIdempotencyKeyKeepsLegacyBehavior(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Legacy writes remain independent"}`

	first := performMemoryCreate(handler, orgID, actorID, "", body)
	second := performMemoryCreate(handler, orgID, actorID, "", body)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("legacy statuses = %d/%d; bodies=%s / %s",
			first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var firstMemory, secondMemory model.Memory
	if err := json.Unmarshal(first.Body.Bytes(), &firstMemory); err != nil {
		t.Fatalf("decode first legacy response: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondMemory); err != nil {
		t.Fatalf("decode second legacy response: %v", err)
	}
	if firstMemory.ID == secondMemory.ID {
		t.Fatalf("legacy writes unexpectedly reused ID %q", firstMemory.ID)
	}

	_, total, err := st.ListMemoriesPaginated(adminCtx(orgID, actorID), projectID, 10, 0, "")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 2 {
		t.Fatalf("memory total = %d, want 2", total)
	}
}

func TestMemoryCreate_ReportsUpdateForExistingID(t *testing.T) {
	for _, operationID := range []string{"", "operation-update-existing"} {
		name := "legacy"
		if operationID != "" {
			name = "idempotent"
		}
		t.Run(name, func(t *testing.T) {
			st := newProjectTestStore(t)
			orgID, actorID, projectID := seedProject(t, st)
			handler := NewMemoryHandler(
				st,
				policy.NewContentFilter(),
				config.DefaultConfig(),
				nil,
				slog.Default(),
			)
			seed := &model.Memory{
				ID:          "existing-memory",
				ProjectID:   projectID,
				Category:    "fact",
				Content:     "old content",
				ContentHash: "sha256:old",
				AuthorID:    actorID,
				AuthorName:  "Test",
				CreatedAt:   time.Now().UTC().Add(-time.Hour),
				UpdatedAt:   time.Now().UTC().Add(-time.Hour),
			}
			if err := st.UpsertMemory(adminCtx(orgID, actorID), seed); err != nil {
				t.Fatalf("seed existing memory: %v", err)
			}

			body := `{"id":"existing-memory","project_id":"` + projectID +
				`","category":"fact","content":"replacement content"}`
			response := performMemoryCreate(handler, orgID, actorID, operationID, body)
			if response.Code != http.StatusCreated {
				t.Fatalf("update status = %d, body=%s", response.Code, response.Body.String())
			}
			assertMemoryCreatedFlag(t, response, false)

			if operationID != "" {
				replay := performMemoryCreate(handler, orgID, actorID, operationID, body)
				if replay.Code != http.StatusCreated {
					t.Fatalf("replay status = %d, body=%s", replay.Code, replay.Body.String())
				}
				assertMemoryCreatedFlag(t, replay, false)
				if got := replay.Header().Get("Idempotency-Replayed"); got != "true" {
					t.Fatalf("replay header = %q, want true", got)
				}
			}
		})
	}
}

func TestMemoryCreate_ReadonlyScopeCannotReplay(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Authorization still applies to replays"}`

	first := performMemoryCreate(handler, orgID, actorID, "operation-readonly", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("initial status = %d, body=%s", first.Code, first.Body.String())
	}

	replay := performMemoryCreateWithScope(
		handler,
		orgID,
		actorID,
		"readonly",
		"operation-readonly",
		body,
	)
	if replay.Code != http.StatusForbidden {
		t.Fatalf("readonly replay status = %d, want 403; body=%s",
			replay.Code, replay.Body.String())
	}
	if got := replay.Header().Get("Idempotency-Replayed"); got != "" {
		t.Fatalf("readonly replay header = %q, want empty", got)
	}
}

func TestMemoryCreate_RejectsUnsafeIdempotencyKeys(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, actorID, projectID := seedProject(t, st)
	handler := NewMemoryHandler(st, policy.NewContentFilter(), config.DefaultConfig(), nil, slog.Default())
	body := `{"project_id":"` + projectID + `","category":"fact","content":"Validate operation identity"}`

	for _, key := range []string{
		strings.Repeat("a", 129),
		"contains space",
		"line\nbreak",
		"não-ascii",
	} {
		response := performMemoryCreate(handler, orgID, actorID, key, body)
		assertCodedSearchError(t, response, http.StatusBadRequest, "invalid_idempotency_key")
	}

	boundary := performMemoryCreate(
		handler,
		orgID,
		actorID,
		strings.Repeat("a", 128),
		body,
	)
	if boundary.Code != http.StatusCreated {
		t.Fatalf("128-byte key status = %d, body=%s", boundary.Code, boundary.Body.String())
	}

	valid := performMemoryCreate(
		handler,
		orgID,
		actorID,
		"request_01~retry:node/a+b.c",
		body,
	)
	if valid.Code != http.StatusCreated {
		t.Fatalf("safe key status = %d, body=%s", valid.Code, valid.Body.String())
	}
}

func performMemoryCreate(
	handler *MemoryHandler,
	orgID, actorID, operationID, body string,
) *httptest.ResponseRecorder {
	return performMemoryCreateWithScope(
		handler, orgID, actorID, "admin", operationID, body,
	)
}

func assertMemoryCreateCreated(t *testing.T, response *httptest.ResponseRecorder) {
	assertMemoryCreatedFlag(t, response, true)
}

func assertMemoryCreatedFlag(t *testing.T, response *httptest.ResponseRecorder, want bool) {
	t.Helper()
	var payload struct {
		Created bool `json:"created"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode created flag: %v", err)
	}
	if payload.Created != want {
		t.Fatalf("created = %v, want %v; body=%s", payload.Created, want, response.Body.String())
	}
}

func performMemoryCreateWithScope(
	handler *MemoryHandler,
	orgID, actorID, scope, operationID, body string,
) *httptest.ResponseRecorder {
	ctx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, middleware.AccountIDKey, actorID)
	ctx = context.WithValue(ctx, middleware.ScopeKey, scope)
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(body)).
		WithContext(ctx)
	if operationID != "" {
		req.Header.Set("Idempotency-Key", operationID)
	}
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	return rec
}

func assertMemoryCreateReplay(
	t *testing.T,
	first, replay *httptest.ResponseRecorder,
) {
	t.Helper()
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, body=%s", replay.Code, replay.Body.String())
	}
	if got := replay.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Fatalf("replay header = %q, want true", got)
	}
	var firstMemory, replayMemory model.Memory
	if err := json.Unmarshal(first.Body.Bytes(), &firstMemory); err != nil {
		t.Fatalf("decode initial response: %v", err)
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayMemory); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if replayMemory.ID != firstMemory.ID || !replayMemory.CreatedAt.Equal(firstMemory.CreatedAt) {
		t.Fatalf("replay = %+v, want original %+v", replayMemory, firstMemory)
	}
}
