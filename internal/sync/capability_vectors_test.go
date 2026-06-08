package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// capabilityVectors is the shared contract file mirrored byte-identically in
// the client repo. Keeping both suites pinned to the same vectors is how the
// two repos stay wire-compatible (same approach as the remote-key vectors).
type capabilityVectors struct {
	Vectors []struct {
		Name              string `json:"name"`
		Request           json.RawMessage
		ResponseHasPolicy bool               `json:"response_has_policy"`
		ExpectedPolicy    *model.PolicyHints `json:"expected_policy"`
	} `json:"vectors"`
}

// TestSyncMatchesCapabilityVectors drives the engine with the shared vectors
// and asserts the policy-presence contract holds for each.
func TestSyncMatchesCapabilityVectors(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "sync_capability_vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vs capabilityVectors
	if err := json.Unmarshal(data, &vs); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}

	for _, v := range vs.Vectors {
		if len(v.Request) == 0 {
			continue // shape-only vectors (e.g. over_cap) covered elsewhere
		}
		t.Run(v.Name, func(t *testing.T) {
			eng, _, ctx, orgID, accID, projID := capTestEngine(t)

			var req model.SyncRequest
			if err := json.Unmarshal(v.Request, &req); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}
			// The vector's placeholder project_id is rebound to the seeded one.
			req.ProjectID = projID

			resp, err := eng.Sync(ctx, accID, orgID, &req)
			if err != nil {
				t.Fatalf("sync: %v", err)
			}
			if v.ResponseHasPolicy != (resp.Policy != nil) {
				t.Fatalf("policy presence mismatch: want %v, got %v", v.ResponseHasPolicy, resp.Policy != nil)
			}
			if v.ExpectedPolicy != nil {
				if resp.Policy.QualityThreshold != v.ExpectedPolicy.QualityThreshold {
					t.Errorf("quality: got %v want %v", resp.Policy.QualityThreshold, v.ExpectedPolicy.QualityThreshold)
				}
				if resp.Policy.MaxMemoriesPerSync != v.ExpectedPolicy.MaxMemoriesPerSync {
					t.Errorf("max: got %v want %v", resp.Policy.MaxMemoriesPerSync, v.ExpectedPolicy.MaxMemoriesPerSync)
				}
				for _, want := range v.ExpectedPolicy.BlockedCategories {
					if !containsStr(resp.Policy.BlockedCategories, want) {
						t.Errorf("blocked categories %v missing %q", resp.Policy.BlockedCategories, want)
					}
				}
			}
		})
	}
}
