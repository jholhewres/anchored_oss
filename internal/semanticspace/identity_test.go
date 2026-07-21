package semanticspace

import "testing"

func TestIdentityIncludesEverySemanticInput(t *testing.T) {
	base := New("onnx", "model", "revision-a", 384, L2Normalization)
	if err := base.Validate(); err != nil {
		t.Fatalf("validate base identity: %v", err)
	}
	if base.ID() != New(" ONNX ", "model", "revision-a", 384, " L2-V1 ").ID() {
		t.Fatal("canonical spelling changed semantic-space identity")
	}

	variants := []Identity{
		New("openai", "model", "revision-a", 384, L2Normalization),
		New("onnx", "other-model", "revision-a", 384, L2Normalization),
		New("onnx", "model", "revision-b", 384, L2Normalization),
		New("onnx", "model", "revision-a", 768, L2Normalization),
		New("onnx", "model", "revision-a", 384, "none-v1"),
	}
	for _, variant := range variants {
		if variant.ID() == base.ID() {
			t.Fatalf("identity collision for variant %+v", variant)
		}
	}
}
