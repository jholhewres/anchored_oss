package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func TestPasswordEntryTokensAccessPersonalAPIKeys(t *testing.T) {
	t.Run("registration", func(t *testing.T) {
		st := newSessionCredentialStore(t)
		h := NewRegisterHandler(st, slog.Default())
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"email":"new@example.test","password":"password123","display_name":"New User","org_name":"New Org"}`))
		rec := httptest.NewRecorder()
		h.Register(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("register = %d: %s", rec.Code, rec.Body.String())
		}
		var response registerResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		assertSessionCredential(t, st, response.APIKey, "admin", http.StatusOK)
	})

	t.Run("invite acceptance", func(t *testing.T) {
		st := newSessionCredentialStore(t)
		ctx := context.Background()
		org, err := st.CreateOrganization(ctx, "Invited Org", "invited-org")
		if err != nil {
			t.Fatal(err)
		}
		invitationToken := "invite-token-for-session-test"
		if _, err := st.CreateInvite(ctx, org.ID, "invited@example.test", "Invited User", "sync", hashInviteToken(invitationToken), time.Now().UTC().Add(time.Hour), ""); err != nil {
			t.Fatal(err)
		}
		h := NewInviteHandler(st, slog.Default())
		req := httptest.NewRequest(http.MethodPost, "/v1/invites/accept/"+invitationToken, strings.NewReader(`{"password":"password123"}`))
		req.SetPathValue("token", invitationToken)
		rec := httptest.NewRecorder()
		h.Accept(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("accept invite = %d: %s", rec.Code, rec.Body.String())
		}
		var response acceptInviteResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		assertSessionCredential(t, st, response.APIKey, "sync", http.StatusOK)
	})
}

func TestOnboardingDefaultKeyDoesNotAccessPersonalAPIKeys(t *testing.T) {
	st := newSessionCredentialStore(t)
	h := NewOnboardingHandler(st, slog.Default())
	body := `{"org":{"name":"Onboarded Org","slug":"onboarded-org"},"admin":{"email":"onboarded@example.test","password":"password123","display_name":"Onboarded User"},"projects":[{"name":"App","category":"app","repo_url":"https://github.com/acme/app.git"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/complete", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Complete(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("onboarding = %d: %s", rec.Code, rec.Body.String())
	}
	var response onboardingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	key, err := st.GetAPIKeyByHash(context.Background(), auth.HashAPIKey(response.APIKey))
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "default" {
		t.Fatalf("onboarding key name = %q, want default", key.Name)
	}
	assertSessionCredential(t, st, response.APIKey, "admin", http.StatusForbidden)
}

func assertSessionCredential(t *testing.T, st *store.SQLiteStore, rawKey, wantScope string, wantStatus int) {
	t.Helper()
	key, err := st.GetAPIKeyByHash(context.Background(), auth.HashAPIKey(rawKey))
	if err != nil {
		t.Fatal(err)
	}
	if wantStatus == http.StatusOK && (key.Name != middleware.SessionAPIKeyName || key.Scope != wantScope) {
		t.Fatalf("session credential = name=%q scope=%q", key.Name, key.Scope)
	}
	personal := NewPersonalAPIKeyHandler(st, slog.Default())
	protected := middleware.Auth(st, slog.Default())(middleware.RequireSession(http.HandlerFunc(personal.List)))
	req := httptest.NewRequest(http.MethodGet, "/v1/me/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("personal API key access = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
}

func newSessionCredentialStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "session-credentials.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
