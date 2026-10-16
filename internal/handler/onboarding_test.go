package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/store"
)

func newOnboardingTestStore(t *testing.T) (*store.SQLiteStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "onboarding.db")
	st, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, path
}

func onboardingTableCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open verification db: %v", err)
	}
	defer db.Close()

	counts := make(map[string]int, 4)
	for _, table := range []string{"organizations", "accounts", "api_keys", "projects"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func TestOnboarding_RejectsInvalidProjectsBeforePersisting(t *testing.T) {
	tests := []struct {
		name     string
		projects string
		want     int
	}{
		{name: "zero projects", projects: `[]`, want: http.StatusBadRequest},
		{name: "blank project name", projects: `[{"name":" ","category":"service","repo_url":"https://github.com/acme/repo.git"}]`, want: http.StatusBadRequest},
		{name: "blank remote", projects: `[{"name":"Repo","category":"service","repo_url":"  "}]`, want: http.StatusBadRequest},
		{name: "malformed remote", projects: `[{"name":"Repo","category":"service","repo_url":"github.com/acme/repo"}]`, want: http.StatusBadRequest},
		{name: "equivalent duplicate", projects: `[{"name":"One","category":"service","repo_url":"https://github.com/acme/repo.git"},{"name":"Two","category":"service","repo_url":"git@github.com:acme/repo.git"}]`, want: http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, path := newOnboardingTestStore(t)
			h := NewOnboardingHandler(st, slog.Default())
			body := `{"org":{"name":"Acme","slug":"acme"},"admin":{"email":"admin@acme.test","password":"password123","display_name":"Admin"},"projects":` + tt.projects + `}`
			req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/complete", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.Complete(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.want, rec.Body.String())
			}
			for table, count := range onboardingTableCounts(t, path) {
				if count != 0 {
					t.Errorf("%s rows = %d, want 0", table, count)
				}
			}
		})
	}
}

func TestOnboarding_CreatesOnlyRemoteBackedProjects(t *testing.T) {
	st, path := newOnboardingTestStore(t)
	h := NewOnboardingHandler(st, slog.Default())
	body := `{"org":{"name":"Acme","slug":"acme"},"admin":{"email":"admin@acme.test","password":"password123","display_name":"Admin"},"projects":[{"name":"API","category":"service","repo_url":"https://github.com/acme/api.git"},{"name":"Web","category":"service","repo_url":"ssh://git@github.com/acme/web.git"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/complete", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Complete(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var response onboardingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.APIKey == "" || len(response.Projects) != 2 {
		t.Fatalf("response = %#v", response)
	}
	for _, project := range response.Projects {
		if project.RepoURL == "" || strings.HasPrefix(project.RemoteKey, "manual:") {
			t.Fatalf("project is not remote-backed: %#v", project)
		}
	}
	for table, want := range map[string]int{"organizations": 1, "accounts": 1, "api_keys": 1, "projects": 2} {
		if got := onboardingTableCounts(t, path)[table]; got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/onboarding/complete", strings.NewReader(body))
	secondRec := httptest.NewRecorder()
	h.Complete(secondRec, second)
	if secondRec.Code != http.StatusConflict {
		t.Errorf("second onboarding status = %d, want 409; body=%s", secondRec.Code, secondRec.Body.String())
	}
}
