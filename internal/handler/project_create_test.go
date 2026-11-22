package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func TestProjectCreate_RequiresValidGitRemote(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	for _, body := range []string{
		`{"name":"Repo","slug":"repo-new"}`,
		`{"name":"Repo","slug":"repo-new","repo_url":"   "}`,
		`{"name":"Repo","slug":"repo-new","repo_url":"github.com/acme/repo"}`,
		`{"name":"Repo","slug":"repo-new","repo_url":"http://github.com/acme/repo"}`,
		`{"name":"Repo","slug":"repo-new","remote_key":"manual:client"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(body)).WithContext(adminCtx(orgID, accountID))
		rec := httptest.NewRecorder()
		h.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("create %s status = %d, want 400; body=%s", body, rec.Code, rec.Body.String())
		}
	}

	if _, err := st.GetProjectByRemoteKey(t.Context(), orgID, "manual:client"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("manual project was created: %v", err)
	}
}

// TestProjectCreate_DerivesRemoteAndReturnsEquivalentDuplicate: an equivalent
// remote answers 200 with the existing project, as v0.6.x did.
func TestProjectCreate_DerivesRemoteAndReturnsEquivalentDuplicate(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	first := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Anchored","slug":"anchored","repo_url":"https://github.com/acme/anchored.git"}`)).WithContext(adminCtx(orgID, accountID))
	firstRec := httptest.NewRecorder()
	h.Create(firstRec, first)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201; body=%s", firstRec.Code, firstRec.Body.String())
	}
	var created model.Project
	if err := json.Unmarshal(firstRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if created.RemoteKey != projectpkg.DeriveRemoteKey("https://github.com/acme/anchored.git") {
		t.Fatalf("remote_key = %q", created.RemoteKey)
	}
	if created.RemoteKeyV1 != projectpkg.DeriveLegacyRemoteKey("https://github.com/acme/anchored.git") {
		t.Fatalf("remote_key_v1 = %q", created.RemoteKeyV1)
	}
	if strings.HasPrefix(created.RemoteKey, "manual:") {
		t.Fatalf("created a manual remote key: %q", created.RemoteKey)
	}

	for _, remote := range []string{
		"git@github.com:acme/anchored.git",
		"ssh://git@github.com/acme/anchored.git",
		"https://github.com/ACME/ANCHORED/",
	} {
		duplicate := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Duplicate","slug":"duplicate","repo_url":"`+remote+`"}`)).WithContext(adminCtx(orgID, accountID))
		duplicateRec := httptest.NewRecorder()
		h.Create(duplicateRec, duplicate)
		if duplicateRec.Code != http.StatusOK {
			t.Fatalf("equivalent remote %q status = %d, want 200; body=%s", remote, duplicateRec.Code, duplicateRec.Body.String())
		}
		var existing model.Project
		if err := json.Unmarshal(duplicateRec.Body.Bytes(), &existing); err != nil {
			t.Fatalf("decode existing project: %v", err)
		}
		if existing.ID != created.ID || existing.Slug != "anchored" {
			t.Fatalf("equivalent remote %q returned %+v, want the existing project %s", remote, existing, created.ID)
		}
	}
}
