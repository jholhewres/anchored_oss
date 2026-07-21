// Package semanticspace defines the complete identity of an embedding vector
// space. Keeping this type independent from both persistence and providers
// lets older Store and Embedder interfaces remain unchanged.
package semanticspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const L2Normalization = "l2-v1"

// Identity names every input that can change the meaning of a vector.
type Identity struct {
	Provider      string
	Model         string
	ModelRevision string
	Dimensions    int
	Normalization string
}

// New returns a normalized semantic-space identity.
func New(provider, model, revision string, dimensions int, normalization string) Identity {
	if normalization == "" {
		normalization = L2Normalization
	}
	return Identity{
		Provider:      strings.ToLower(strings.TrimSpace(provider)),
		Model:         strings.TrimSpace(model),
		ModelRevision: strings.TrimSpace(revision),
		Dimensions:    dimensions,
		Normalization: strings.ToLower(strings.TrimSpace(normalization)),
	}
}

// Validate rejects incomplete identities before they can label or select
// vectors. ModelRevision is optional because not every remote provider exposes
// one.
func (i Identity) Validate() error {
	if i.Provider == "" {
		return fmt.Errorf("semantic space provider is required")
	}
	if i.Model == "" {
		return fmt.Errorf("semantic space model is required")
	}
	if i.Dimensions <= 0 {
		return fmt.Errorf("semantic space dimensions must be > 0")
	}
	if i.Normalization == "" {
		return fmt.Errorf("semantic space normalization is required")
	}
	return nil
}

// ID is a stable, opaque identifier. Length-prefixing prevents ambiguous
// concatenations while keeping the persisted value compact and indexable.
func (i Identity) ID() string {
	canonical := fmt.Sprintf(
		"%d:%s|%d:%s|%d:%s|%d|%d:%s",
		len(i.Provider), i.Provider,
		len(i.Model), i.Model,
		len(i.ModelRevision), i.ModelRevision,
		i.Dimensions,
		len(i.Normalization), i.Normalization,
	)
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}
