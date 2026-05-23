// Package auth contains identity primitives shared across handlers and bootstrap.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// APIKeyPrefix is the human-readable prefix of every generated API key.
const APIKeyPrefix = "anc_live_"

// GenerateAPIKey returns a fresh API key in plain text along with its
// derived short prefix and SHA-256 hash. The plain text is only returned
// once and must never be persisted.
func GenerateAPIKey() (full, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	full = APIKeyPrefix + hex.EncodeToString(b)
	prefix = full[:12]
	h := sha256.Sum256([]byte(full))
	hash = hex.EncodeToString(h[:])
	return full, prefix, hash, nil
}

// HashAPIKey returns the SHA-256 hex digest of a presented key for lookup.
func HashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
