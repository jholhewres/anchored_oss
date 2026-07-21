package embeddings

import "github.com/jholhewres/anchored_oss/internal/semanticspace"

// ModelRevisioner is an optional additive provider capability. Providers whose
// artifacts have a stable revision can expose it without changing Embedder.
type ModelRevisioner interface {
	ModelRevision() string
}

// SemanticProviderIdentityer lets an implementation distinguish compatible
// protocol endpoints that share the same legacy Name. For example, two
// OpenAI-compatible base URLs are not assumed to serve an identical model
// merely because both return "openai" from Name().
type SemanticProviderIdentityer interface {
	SemanticProviderIdentity() string
}

// SemanticSpace returns the complete vector-space identity for an embedder.
func SemanticSpace(embedder Embedder) semanticspace.Identity {
	provider := embedder.Name()
	if identityProvider, ok := embedder.(SemanticProviderIdentityer); ok {
		provider = identityProvider.SemanticProviderIdentity()
	}
	revision := ""
	if revisioner, ok := embedder.(ModelRevisioner); ok {
		revision = revisioner.ModelRevision()
	}
	return semanticspace.New(
		provider,
		embedder.Model(),
		revision,
		embedder.Dimensions(),
		semanticspace.L2Normalization,
	)
}
