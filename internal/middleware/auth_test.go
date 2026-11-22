package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// roleStore serves one key and the role its account holds, or no membership.
type roleStore struct {
	store.Store
	key     *model.APIKey
	role    string // "" means the account is not a member of the key's org
	revoked bool
}

func (s *roleStore) RevokeAPIKey(context.Context, string, string) error {
	s.revoked = true
	return nil
}

func (s *roleStore) GetAPIKeyByHash(context.Context, string) (*model.APIKey, error) {
	return s.key, nil
}

func (s *roleStore) GetOrgMemberRole(context.Context, string, string) (string, error) {
	if s.role == "" {
		return "", store.ErrNotFound
	}
	return s.role, nil
}

// TestAuthChecksTheKeyAgainstItsAccountsRole: a key authenticates only while
// its account is a member of the key's org with a role at or above the key's
// scope.
func TestAuthChecksTheKeyAgainstItsAccountsRole(t *testing.T) {
	for _, tc := range []struct {
		scope, role string
		want        int
	}{
		{"admin", "admin", http.StatusOK},
		{"sync", "member", http.StatusOK},
		{"readonly", "readonly", http.StatusOK},
		{"admin", "sync", http.StatusUnauthorized},
		{"sync", "readonly", http.StatusUnauthorized},
		{"readonly", "", http.StatusUnauthorized},
	} {
		st := &roleStore{
			key:  &model.APIKey{ID: "k", OrgID: "o", AccountID: "a", Scope: tc.scope, Name: "laptop"},
			role: tc.role,
		}
		h := Auth(st, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		req.Header.Set("Authorization", "Bearer anc_live_test")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("scope %q with role %q = %d, want %d", tc.scope, tc.role, rec.Code, tc.want)
		}
		if refused := tc.want != http.StatusOK; st.revoked != refused {
			t.Errorf("scope %q with role %q: revoked = %t, want %t", tc.scope, tc.role, st.revoked, refused)
		}
	}
}
