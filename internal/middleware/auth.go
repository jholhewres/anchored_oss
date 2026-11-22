package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type contextKey string

// SessionAPIKeyName is reserved for the per-account session credential issued
// by a password-authenticated dashboard entry flow; a new login revokes the
// prior credential with this name.
const SessionAPIKeyName = "session"

const (
	AccountIDKey  contextKey = "account_id"
	OrgIDKey      contextKey = "org_id"
	ScopeKey      contextKey = "scope"
	APIKeyNameKey contextKey = "api_key_name"
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

// GetAPIKeyName returns the authenticated credential's server-assigned name.
// It is set only by Auth and is used where a route requires a login session
// rather than an arbitrary persistent API key.
func GetAPIKeyName(ctx context.Context) string {
	v, _ := ctx.Value(APIKeyNameKey).(string)
	return v
}

// Auth returns middleware that validates Bearer token API keys against the store.
func Auth(st store.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeUnauthorized(w)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeUnauthorized(w)
				return
			}

			keyHash := auth.HashAPIKey(parts[1])
			apiKey, err := st.GetAPIKeyByHash(r.Context(), keyHash)
			if errors.Is(err, store.ErrNotFound) || apiKey == nil {
				writeUnauthorized(w)
				return
			}
			if err != nil {
				logger.Error("auth: key lookup failed", "error", err)
				writeUnauthorized(w)
				return
			}

			if apiKey.RevokedAt != nil {
				logger.Warn("auth: revoked key used", "key_id", apiKey.ID)
				writeUnauthorized(w)
				return
			}

			if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
				logger.Warn("auth: expired key used", "key_id", apiKey.ID)
				writeUnauthorized(w)
				return
			}

			// A key never outranks its account. Role changes revoke the keys
			// above the new role, and minting refuses them, but a key minted
			// in a race with a demotion, or kept from before those rules, is
			// refused here, as is a key whose account left the org.
			role, err := st.GetOrgMemberRole(r.Context(), apiKey.OrgID, apiKey.AccountID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				logger.Error("auth: role lookup failed", "error", err)
				writeUnauthorized(w)
				return
			}
			if err != nil || !auth.ScopeAllowedForRole(role, apiKey.Scope) {
				// Revoked as well as refused, so a later promotion does not
				// bring the key back.
				logger.Warn("auth: key above its account's role, revoking", "key_id", apiKey.ID, "scope", apiKey.Scope, "role", role)
				if rerr := st.RevokeAPIKey(r.Context(), apiKey.OrgID, apiKey.ID); rerr != nil && !errors.Is(rerr, store.ErrNotFound) {
					logger.Error("auth: revoke over-scoped key failed", "key_id", apiKey.ID, "error", rerr)
				}
				writeUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), AccountIDKey, apiKey.AccountID)
			ctx = context.WithValue(ctx, OrgIDKey, apiKey.OrgID)
			ctx = context.WithValue(ctx, ScopeKey, apiKey.Scope)
			ctx = context.WithValue(ctx, APIKeyNameKey, apiKey.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireSession restricts a route to the credential minted by a
// password-authenticated dashboard entry flow. It deliberately does not
// require an admin scope: a session retains its member role while still being
// allowed to manage that account's own API keys.
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetAPIKeyName(r.Context()) != SessionAPIKeyName {
			writeForbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope returns middleware that checks the scope from context.
// Keys with scope "admin" pass all scope checks.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := GetScope(r.Context())
			if s != scope && s != "admin" {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED")
}

func writeForbidden(w http.ResponseWriter) {
	writeJSONError(w, http.StatusForbidden, "FORBIDDEN")
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// jsonError keeps the previous symbol available for legacy callers
// within the middleware package; new code should use writeJSONError.
func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSONError(w, status, msg)
}
