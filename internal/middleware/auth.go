package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/store"
)

type contextKey string

const (
	AccountIDKey contextKey = "account_id"
	OrgIDKey     contextKey = "org_id"
	ScopeKey     contextKey = "scope"
)

func GetAccountID(ctx context.Context) string {
	v, _ := ctx.Value(AccountIDKey).(string)
	return v
}

func GetOrgID(ctx context.Context) string {
	v, _ := ctx.Value(OrgIDKey).(string)
	return v
}

func GetScope(ctx context.Context) string {
	v, _ := ctx.Value(ScopeKey).(string)
	return v
}

// Auth returns middleware that validates Bearer token API keys against the store.
func Auth(st store.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			rawKey := parts[1]
			h := sha256.Sum256([]byte(rawKey))
			keyHash := hex.EncodeToString(h[:])

			apiKey, err := st.GetAPIKeyByHash(r.Context(), keyHash)
			if err != nil || apiKey == nil {
				logger.Warn("auth: key lookup failed", "error", err)
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if apiKey.RevokedAt != nil {
				logger.Warn("auth: revoked key used", "key_id", apiKey.ID)
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
				logger.Warn("auth: expired key used", "key_id", apiKey.ID)
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			ctx := context.WithValue(r.Context(), AccountIDKey, apiKey.AccountID)
			ctx = context.WithValue(ctx, OrgIDKey, apiKey.OrgID)
			ctx = context.WithValue(ctx, ScopeKey, apiKey.Scope)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope returns middleware that checks the scope from context.
// Keys with scope "admin" pass all scope checks.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := GetScope(r.Context())
			if s != scope && s != "admin" {
				jsonError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// jsonError is a local copy to avoid importing handler (which would create a cycle).
func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
