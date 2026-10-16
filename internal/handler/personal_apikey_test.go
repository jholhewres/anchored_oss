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
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func TestPersonalAPIKeys_OwnerScopeValidationAndSecretLifetime(t *testing.T) {
	st, org, accountA, accountB := newPersonalAPIKeyTestStore(t)
	h := NewPersonalAPIKeyHandler(st, slog.Default())
	ctxA := personalAPIKeyContext(org.ID, accountA.ID)

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/me/api-keys", strings.NewReader(body)).WithContext(ctxA)
		rec := httptest.NewRecorder()
		h.Create(rec, req)
		return rec
	}

	// Omitting expires_in produces a persistent, but bounded, 90-day key.
	rec := post(`{"name":"laptop sync","scope":"sync"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !strings.HasPrefix(created.Key, auth.APIKeyPrefix) {
		t.Fatalf("creation response missing one-time secret: %s", rec.Body.String())
	}
	expiresAt, err := time.Parse(time.RFC3339, created.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expiry %q: %v", created.ExpiresAt, err)
	}
	if delta := time.Until(expiresAt); delta < 89*24*time.Hour || delta > 90*24*time.Hour {
		t.Fatalf("default expiry = %v, want approximately 90d", delta)
	}

	stored, err := st.ListAPIKeysByOwner(context.Background(), org.ID, accountA.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored keys = %#v, %v", stored, err)
	}
	if stored[0].KeyHash == created.Key || stored[0].KeyHash != auth.HashAPIKey(created.Key) {
		t.Fatal("plaintext API key was not stored exclusively as a hash")
	}
	audits, _, err := st.ListAuditEntries(context.Background(), org.ID, model.AuditFilters{Action: "api_key.personal_created"})
	if err != nil || len(audits) != 1 {
		t.Fatalf("personal create audit = %#v, %v", audits, err)
	}
	auditJSON, err := json.Marshal(audits[0].Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(auditJSON), created.Key) || strings.Contains(string(auditJSON), stored[0].KeyHash) {
		t.Fatalf("personal create audit leaked secret material: %s", auditJSON)
	}

	// account_id is rejected rather than silently trusted or ignored. Admin and
	// overlong expirations are rejected before any key is issued.
	for _, body := range []string{
		`{"name":"session","scope":"sync"}`,
		`{"name":"bad","scope":"sync","account_id":"` + accountB.ID + `"}`,
		`{"name":"bad","scope":"admin"}`,
		`{"name":"bad","scope":"readonly","expires_in":"366d"}`,
		`{"name":"bad","scope":"readonly","expires_in":"1000000d"}`,
	} {
		if got := post(body); got.Code != http.StatusBadRequest {
			t.Fatalf("POST %s = %d, want 400: %s", body, got.Code, got.Body.String())
		}
	}

	// An account-B key in the same organization is invisible to account A.
	other, err := st.CreateAPIKey(context.Background(), org.ID, accountB.ID, "B private", "anc_live_", "b-hash", "readonly", nil)
	if err != nil {
		t.Fatal(err)
	}
	ownerAdmin, err := st.CreateAPIKey(context.Background(), org.ID, accountA.ID, "bootstrap admin", "anc_live_", "admin-hash", "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	otherOrg, err := st.CreateOrganization(context.Background(), "Other org", "other-personal-api-key-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(context.Background(), otherOrg.ID, accountA.ID, "member"); err != nil {
		t.Fatal(err)
	}
	crossOrg, err := st.CreateAPIKey(context.Background(), otherOrg.ID, accountA.ID, "Other org private", "anc_live_", "other-org-hash", "readonly", nil)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/v1/me/api-keys", nil).WithContext(ctxA)
	getRec := httptest.NewRecorder()
	h.List(getRec, get)
	if getRec.Code != http.StatusOK || strings.Contains(getRec.Body.String(), other.ID) || strings.Contains(getRec.Body.String(), crossOrg.ID) || strings.Contains(getRec.Body.String(), ownerAdmin.ID) {
		t.Fatalf("owner-scoped list = %d: %s", getRec.Code, getRec.Body.String())
	}
	if strings.Contains(getRec.Body.String(), created.Key) || strings.Contains(getRec.Body.String(), "key_hash") {
		t.Fatalf("GET leaked secret material: %s", getRec.Body.String())
	}

	// Same response for foreign and absent IDs prevents an existence oracle.
	var notFoundBody string
	for _, id := range []string{other.ID, crossOrg.ID, ownerAdmin.ID, "00000000-0000-0000-0000-000000000000"} {
		req := httptest.NewRequest(http.MethodDelete, "/v1/me/api-keys/"+id, nil).WithContext(ctxA)
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.Revoke(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("revoke %s = %d, want 404: %s", id, rec.Code, rec.Body.String())
		}
		if notFoundBody == "" {
			notFoundBody = rec.Body.String()
		} else if rec.Body.String() != notFoundBody {
			t.Fatalf("foreign and missing revokes leaked different responses: %q != %q", rec.Body.String(), notFoundBody)
		}
	}
	foreign, err := st.ListAPIKeysByOwner(context.Background(), org.ID, accountB.ID)
	if err != nil || len(foreign) != 1 || foreign[0].RevokedAt != nil {
		t.Fatalf("foreign key changed after rejected revoke: %#v, %v", foreign, err)
	}
	crossOrgKeys, err := st.ListAPIKeysByOwner(context.Background(), otherOrg.ID, accountA.ID)
	if err != nil || len(crossOrgKeys) != 1 || crossOrgKeys[0].RevokedAt != nil {
		t.Fatalf("cross-org key changed after rejected revoke: %#v, %v", crossOrgKeys, err)
	}
	orgKeys, err := st.ListAPIKeysByOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range orgKeys {
		if key.ID == ownerAdmin.ID && key.RevokedAt != nil {
			t.Fatal("personal revoke changed the owner's admin key")
		}
	}

	// The plaintext secret is intentionally creation-only: GET emits metadata.
	if !strings.Contains(getRec.Body.String(), created.ID) || strings.Contains(getRec.Body.String(), `"key":`) {
		t.Fatalf("GET personal keys had the wrong response shape: %s", getRec.Body.String())
	}
}

func TestPersonalAPIKeys_RevokedAndExpiredKeysFailAuthentication(t *testing.T) {
	st, org, accountA, _ := newPersonalAPIKeyTestStore(t)
	h := NewPersonalAPIKeyHandler(st, slog.Default())
	ctxA := personalAPIKeyContext(org.ID, accountA.ID)

	create := httptest.NewRequest(http.MethodPost, "/v1/me/api-keys", strings.NewReader(`{"name":"revocable","scope":"readonly"}`)).WithContext(ctxA)
	created := httptest.NewRecorder()
	h.Create(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var result struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	protected := middleware.Auth(st, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	authenticate := func(key string) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/me/api-keys", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := authenticate(result.Key); got != http.StatusNoContent {
		t.Fatalf("new key authentication = %d, want 204", got)
	}
	revoke := httptest.NewRequest(http.MethodDelete, "/v1/me/api-keys/"+result.ID, nil).WithContext(ctxA)
	revoke.SetPathValue("id", result.ID)
	revoked := httptest.NewRecorder()
	h.Revoke(revoked, revoke)
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("self revoke = %d: %s", revoked.Code, revoked.Body.String())
	}
	if got := authenticate(result.Key); got != http.StatusUnauthorized {
		t.Fatalf("revoked key authentication = %d, want 401", got)
	}
	revokeAudits, _, err := st.ListAuditEntries(context.Background(), org.ID, model.AuditFilters{Action: "api_key.personal_revoked"})
	if err != nil || len(revokeAudits) != 1 || revokeAudits[0].TargetID != result.ID {
		t.Fatalf("personal revoke audit = %#v, %v", revokeAudits, err)
	}
	revokeAuditJSON, err := json.Marshal(revokeAudits[0].Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(revokeAuditJSON), result.Key) || strings.Contains(string(revokeAuditJSON), auth.HashAPIKey(result.Key)) {
		t.Fatalf("personal revoke audit leaked secret material: %s", revokeAuditJSON)
	}

	expiredAt := time.Now().UTC().Add(-time.Minute)
	expiredRaw, expiredPrefix, expiredHash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIKey(context.Background(), org.ID, accountA.ID, "expired", expiredPrefix, expiredHash, "readonly", &expiredAt); err != nil {
		t.Fatal(err)
	}
	if got := authenticate(expiredRaw); got != http.StatusUnauthorized {
		t.Fatalf("expired key authentication = %d, want 401", got)
	}
}

func newPersonalAPIKeyTestStore(t *testing.T) (*store.SQLiteStore, *model.Organization, *model.Account, *model.Account) {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "personal-api-keys.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Personal API key test", "personal-api-key-test")
	if err != nil {
		t.Fatal(err)
	}
	accountA, err := st.CreateAccount(ctx, "a@example.test", "Account A", "hash")
	if err != nil {
		t.Fatal(err)
	}
	accountB, err := st.CreateAccount(ctx, "b@example.test", "Account B", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []*model.Account{accountA, accountB} {
		if err := st.AddOrgMember(ctx, org.ID, account.ID, "member"); err != nil {
			t.Fatal(err)
		}
	}
	return st, org, accountA, accountB
}

func personalAPIKeyContext(orgID, accountID string) context.Context {
	ctx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, middleware.AccountIDKey, accountID)
	return context.WithValue(ctx, middleware.ScopeKey, "admin")
}
