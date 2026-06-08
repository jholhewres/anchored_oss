package sync

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type SyncEngine struct {
	store  store.Store
	logger *slog.Logger
}

func NewSyncEngine(st store.Store, logger *slog.Logger) *SyncEngine {
	return &SyncEngine{
		store:  st,
		logger: logger,
	}
}

func (e *SyncEngine) Sync(ctx context.Context, accountID, orgID string, req *model.SyncRequest) (*model.SyncResponse, error) {
	projectID, err := e.resolveProject(ctx, orgID, accountID, req)
	if err != nil {
		return nil, err
	}

	if err := e.authorize(ctx, accountID, projectID); err != nil {
		return nil, err
	}

	// Capture the watermark before any reads so that writes happening
	// concurrently with this request are picked up by the next sync.
	watermark := time.Now().UTC()

	// Effective policy backs both the batch cap and the optional hints.
	pol, perr := e.store.GetOrgPolicy(ctx, orgID)
	if perr != nil {
		e.logger.Warn("sync: org policy load failed; using server defaults", "org_id", orgID, "error", perr)
		pol = &model.OrgPolicy{MaxMemoriesPerSync: store.DefaultMaxMemoriesPerSync}
	}
	maxPerSync := pol.MaxMemoriesPerSync
	if maxPerSync <= 0 {
		maxPerSync = store.DefaultMaxMemoriesPerSync
	}

	var results []model.SyncResult

	// Batch cap: an over-sized push is rejected wholesale, before any write.
	// This is a hard defense against a mis-scoped client dumping its entire
	// store into the wrong project — partition and retry is the only path.
	if len(req.Pushes) > maxPerSync {
		detail := fmt.Sprintf("push of %d exceeds the per-sync cap of %d; split into smaller batches and retry", len(req.Pushes), maxPerSync)
		for _, p := range req.Pushes {
			results = append(results, model.SyncResult{ID: p.ID, Status: "rejected", Rule: "max_memories_per_sync", Detail: detail})
		}
		// Feed the memory-health rejection counters so an over-cap dump is
		// visible on the dashboard — this is the exact scenario the cap defends
		// against. Best-effort: a failed increment never changes the outcome.
		if err := e.store.IncrementRejectionStat(ctx, orgID, projectID, "max_memories_per_sync", int64(len(req.Pushes))); err != nil {
			e.logger.Error("rejection stat increment failed", "rule", "max_memories_per_sync", "error", err)
		}
	} else {
		pushResults, err := e.handlePushes(ctx, accountID, orgID, projectID, req.Pushes)
		if err != nil {
			return nil, fmt.Errorf("push: %w", err)
		}
		results = append(results, pushResults...)
	}

	tombResults, err := e.handleTombstones(ctx, orgID, projectID, req.Tombstones)
	if err != nil {
		return nil, fmt.Errorf("tombstones: %w", err)
	}
	results = append(results, tombResults...)

	pulls, serverTombstones, err := e.pull(ctx, projectID, req.Watermark)
	if err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}

	if results == nil {
		results = []model.SyncResult{}
	}

	resp := &model.SyncResponse{
		ProjectID:        projectID,
		Pulls:            pulls,
		ServerTombstones: serverTombstones,
		Results:          results,
		Watermark:        watermark,
	}

	// Policy hints and artifact summaries are emitted only to capability-aware
	// clients, so a capability-less client's response stays byte-identical to
	// the pre-negotiation protocol.
	if req.ClientCapabilities != nil {
		resp.Policy = &model.PolicyHints{
			QualityThreshold:   pol.QualityThreshold,
			BlockedCategories:  e.effectiveBlockedCategories(ctx, orgID),
			MaxMemoriesPerSync: maxPerSync,
		}
		if req.ClientCapabilities.ArtifactSummaries {
			resp.ArtifactSummaries = artifactSummariesFromResults(req.Pushes, results)
		}
	}

	return resp, nil
}

// artifactSummariesFromResults collects the unique artifact IDs from accepted
// push memories. The artifact_id is carried in the memory's Metadata map under
// the key "artifact_id". Memories without an artifact_id, or whose push was
// rejected, are silently skipped.
func artifactSummariesFromResults(pushes []model.SyncMemory, results []model.SyncResult) []model.ArtifactSummary {
	seen := make(map[string]struct{})
	var out []model.ArtifactSummary
	for i, r := range results {
		if r.Status != "accepted" {
			continue
		}
		if i >= len(pushes) {
			continue
		}
		meta, ok := pushes[i].Metadata.(map[string]any)
		if !ok {
			continue
		}
		id, ok := meta["artifact_id"].(string)
		if !ok || id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, model.ArtifactSummary{ArtifactID: id})
	}
	return out
}

// effectiveBlockedCategories returns the category names the org's guardrails
// reject at sync time, mirroring filterForOrg's category logic. An org with no
// guardrail rows falls back to the code default blocked set.
func (e *SyncEngine) effectiveBlockedCategories(ctx context.Context, orgID string) []string {
	guards, err := e.store.ListGuardrails(ctx, orgID)
	if err != nil || len(guards) == 0 {
		return append([]string(nil), policy.DefaultBlockedCategories...)
	}
	cats := make([]string, 0)
	for _, g := range guards {
		if g.Enabled && g.Kind == model.GuardrailCategory && g.Value != "" {
			cats = append(cats, g.Value)
		}
	}
	return cats
}

func (e *SyncEngine) resolveProject(ctx context.Context, orgID, accountID string, req *model.SyncRequest) (string, error) {
	if req.ProjectID != "" {
		p, err := e.store.GetProjectByID(ctx, req.ProjectID)
		if errors.Is(err, store.ErrNotFound) {
			return "", &SyncError{Code: "PROJECT_NOT_FOUND", Status: 404, Msg: "project not found"}
		}
		if err != nil {
			e.logger.Error("project lookup failed", "project_id", req.ProjectID, "error", err)
			return "", &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "project lookup failed"}
		}
		if p.OrgID != orgID {
			return "", &SyncError{Code: "FORBIDDEN", Status: 403, Msg: "project belongs to a different organization"}
		}
		return p.ID, nil
	}

	claim := req.ProjectClaim
	if claim == nil {
		return "", &SyncError{Code: "INVALID_REQUEST", Status: 400, Msg: "project_id or project_claim is required"}
	}

	if bad := hasLocalPathInClaim(claim); bad != "" {
		return "", &SyncError{Code: "LOCAL_PATH_DETECTED", Status: 400, Msg: fmt.Sprintf("claim field contains local path pattern: %s", bad)}
	}
	if claim.RemoteKey == "" || claim.Name == "" {
		return "", &SyncError{Code: "INVALID_REQUEST", Status: 400, Msg: "project_claim requires name and remote_key"}
	}

	existing, err := e.store.GetProjectByRemoteKey(ctx, orgID, claim.RemoteKey)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		e.logger.Error("project by remote_key failed", "remote_key", claim.RemoteKey, "error", err)
		return "", &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "project lookup failed"}
	}

	return "", &SyncError{
		Code:   "PROJECT_NOT_FOUND",
		Status: 404,
		Msg:    fmt.Sprintf("no project with remote_key %q — create one in the dashboard first, or use project_id to target an existing project", claim.RemoteKey),
	}
}

func (e *SyncEngine) authorize(ctx context.Context, accountID, projectID string) error {
	// Admin keys bypass team-access checks but are still bounded by the
	// org match in resolveProject.
	if middleware.GetScope(ctx) == "admin" {
		return nil
	}
	projects, err := e.store.ListProjectsByTeamAccess(ctx, accountID)
	if err != nil {
		e.logger.Error("authorize: list projects failed", "account_id", accountID, "error", err)
		return &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "authorization check failed"}
	}
	for _, p := range projects {
		if p.ID == projectID {
			return nil
		}
	}
	return &SyncError{Code: "FORBIDDEN", Status: 403, Msg: "no team access to this project"}
}

// filterForOrg builds the per-org content filter from its guardrail set. An org
// with no guardrail rows (created before the guardrail manager, or a seed
// failure) falls back to the legacy default filter, so enforcement is never
// weaker than before. With rows present, disabling every category guardrail
// legitimately blocks no categories.
func (e *SyncEngine) filterForOrg(ctx context.Context, orgID string) *policy.ContentFilter {
	guards, err := e.store.ListGuardrails(ctx, orgID)
	if err != nil {
		e.logger.Warn("sync: guardrails load failed; using safe default filter", "org_id", orgID, "error", err)
		return policy.NewContentFilter()
	}

	var quality float64
	if pol, perr := e.store.GetOrgPolicy(ctx, orgID); perr == nil {
		quality = pol.QualityThreshold
	}

	if len(guards) == 0 {
		// Pre-migration / unseeded org: legacy default (all security on, default
		// blocked categories).
		return policy.NewContentFilterWithConfig(nil, quality)
	}

	cfg := policy.Config{QualityThreshold: quality}
	cats := make([]string, 0)
	for _, g := range guards {
		if !g.Enabled {
			continue
		}
		switch g.Kind {
		case model.GuardrailSecretDetection:
			cfg.SecretDetection = true
		case model.GuardrailLocalPathRedaction:
			cfg.PathRedaction = true
		case model.GuardrailUserScopeBlock:
			cfg.UserScopeBlock = true
		case model.GuardrailCategory:
			if g.Value != "" {
				cats = append(cats, g.Value)
			}
		case model.GuardrailRegex:
			re, cerr := regexp.Compile(g.Value)
			if cerr != nil {
				e.logger.Warn("sync: skipping invalid regex guardrail", "id", g.ID, "error", cerr)
				continue
			}
			cfg.CustomRules = append(cfg.CustomRules, policy.CustomRule{Label: g.Label, Re: re})
		case model.GuardrailKeyword:
			if g.Value == "" {
				continue
			}
			cfg.CustomRules = append(cfg.CustomRules, policy.CustomRule{
				Label: g.Label,
				Re:    regexp.MustCompile("(?i)" + regexp.QuoteMeta(g.Value)),
			})
		}
	}
	cfg.BlockedCategories = cats
	return policy.NewContentFilterFromConfig(cfg)
}

func (e *SyncEngine) handlePushes(ctx context.Context, accountID, orgID, projectID string, pushes []model.SyncMemory) ([]model.SyncResult, error) {
	if len(pushes) == 0 {
		return nil, nil
	}

	filterables := make([]policy.Filterable, len(pushes))
	for i, p := range pushes {
		var meta map[string]any
		if m, ok := p.Metadata.(map[string]any); ok {
			meta = m
		}
		filterables[i] = policy.Filterable{
			ID:       p.ID,
			Content:  p.Content,
			Category: p.Category,
			Metadata: meta,
		}
	}
	// Enforce the org's guardrail set (security toggles, blocked categories,
	// custom regex/keyword rules). Falls back to the default filter on error.
	filterResults := e.filterForOrg(ctx, orgID).Filter(filterables)

	results := make([]model.SyncResult, len(pushes))
	accepted := make([]*model.Memory, 0, len(pushes))
	auditEntries := make([]*model.AuditEntry, 0, len(pushes))
	rejectedByRule := make(map[string]int64)

	for i, push := range pushes {
		fr := filterResults[i]
		if !fr.Accepted {
			results[i] = model.SyncResult{
				ID:     push.ID,
				Status: "rejected",
				Rule:   fr.Rule,
				Detail: fr.Detail,
			}
			rejectedByRule[fr.Rule]++
			auditEntries = append(auditEntries, buildAudit(orgID, projectID, accountID, "sync.push.rejected", "memory", push.ID, fr.Rule))
			continue
		}

		mem := &model.Memory{
			ID:          push.ID,
			ProjectID:   projectID,
			Category:    push.Category,
			Content:     push.Content,
			ContentHash: push.ContentHash,
			Keywords:    push.Keywords,
			Source:      push.Source,
			AuthorID:    accountID,
			AuthorName:  push.AuthorName,
			CreatedAt:   push.CreatedAt,
			UpdatedAt:   push.UpdatedAt,
			Metadata:    push.Metadata,
		}
		if mem.ID == "" {
			mem.ID = newID()
		}
		accepted = append(accepted, mem)
		results[i] = model.SyncResult{ID: mem.ID, Status: "accepted"}
		auditEntries = append(auditEntries, buildAudit(orgID, projectID, accountID, "sync.push.accepted", "memory", mem.ID, ""))
	}

	if len(accepted) > 0 {
		if err := e.store.UpsertMemories(ctx, accepted); err != nil {
			e.logger.Error("batch upsert memories failed", "count", len(accepted), "error", err)
			// Re-flag every accepted slot as rejected so the client knows
			// nothing landed; partial batches are inherently atomic per chunk
			// but we cannot tell which row triggered the violation from here.
			rejectedIDs := make(map[string]bool, len(accepted))
			for _, m := range accepted {
				rejectedIDs[m.ID] = true
			}
			for i := range results {
				if results[i].Status == "accepted" && rejectedIDs[results[i].ID] {
					results[i] = model.SyncResult{
						ID:     results[i].ID,
						Status: "rejected",
						Rule:   "internal_error",
						Detail: "failed to store memory batch",
					}
				}
			}
			// Trim audit entries for accepted-but-now-rejected items.
			pruned := auditEntries[:0]
			for _, a := range auditEntries {
				if a.Action == "sync.push.accepted" && rejectedIDs[a.TargetID] {
					continue
				}
				pruned = append(pruned, a)
			}
			auditEntries = pruned
		}
	}

	if err := e.store.AppendAudits(ctx, auditEntries); err != nil {
		e.logger.Error("audit batch append failed", "count", len(auditEntries), "error", err)
	}

	// Feed the memory-health rejection counters. Best-effort: a failed
	// increment never changes the sync outcome.
	for rule, n := range rejectedByRule {
		if err := e.store.IncrementRejectionStat(ctx, orgID, projectID, rule, n); err != nil {
			e.logger.Error("rejection stat increment failed", "rule", rule, "error", err)
		}
	}

	return results, nil
}

func buildAudit(orgID, projectID, actorID, action, targetType, targetID, detail string) *model.AuditEntry {
	metadata := map[string]string{}
	if detail != "" {
		metadata["detail"] = detail
	}
	return &model.AuditEntry{
		OrgID:      orgID,
		ProjectID:  projectID,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadata,
	}
}

func (e *SyncEngine) handleTombstones(ctx context.Context, orgID, projectID string, tombstones []string) ([]model.SyncResult, error) {
	if len(tombstones) == 0 {
		return nil, nil
	}

	results := make([]model.SyncResult, len(tombstones))
	for i, id := range tombstones {
		err := e.store.SoftDeleteMemory(ctx, id, projectID)
		if errors.Is(err, store.ErrNotFound) {
			results[i] = model.SyncResult{
				ID:     id,
				Status: "rejected",
				Rule:   "not_found",
				Detail: "memory not found or already deleted",
			}
			continue
		}
		if err != nil {
			e.logger.Error("tombstone failed", "id", id, "error", err)
			results[i] = model.SyncResult{
				ID:     id,
				Status: "rejected",
				Rule:   "internal_error",
				Detail: "failed to delete memory",
			}
			continue
		}
		results[i] = model.SyncResult{
			ID:     id,
			Status: "accepted",
		}
		e.appendAudit(ctx, orgID, projectID, "", "sync.tombstone.accepted", "memory", id, "")
	}
	return results, nil
}

func (e *SyncEngine) pull(ctx context.Context, projectID string, watermark *time.Time) ([]model.Memory, []string, error) {
	if watermark == nil {
		return nil, nil, nil
	}

	memories, err := e.store.GetMemoriesUpdatedSince(ctx, projectID, *watermark)
	if err != nil {
		return nil, nil, fmt.Errorf("query updated memories: %w", err)
	}

	pulls := make([]model.Memory, len(memories))
	for i, m := range memories {
		pulls[i] = *m
	}

	tombstones, err := e.store.GetTombstonesSince(ctx, projectID, *watermark)
	if err != nil {
		return nil, nil, fmt.Errorf("query tombstones: %w", err)
	}

	return pulls, tombstones, nil
}

func (e *SyncEngine) appendAudit(ctx context.Context, orgID, projectID, actorID, action, targetType, targetID, detail string) {
	metadata := map[string]string{}
	if detail != "" {
		metadata["detail"] = detail
	}
	entry := &model.AuditEntry{
		OrgID:      orgID,
		ProjectID:  projectID,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadata,
	}
	if err := e.store.AppendAudit(ctx, entry); err != nil {
		e.logger.Error("audit append failed", "action", action, "target", targetID, "error", err)
	}
}

func hasLocalPathInClaim(claim *model.ProjectClaim) string {
	for _, f := range []string{claim.RemoteKey, claim.Name, claim.RepoSlug} {
		if found, pattern := policy.ContainsLocalPath(f); found {
			return pattern
		}
	}
	return ""
}

func toSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	return strings.Trim(result.String(), "-")
}

var idFallbackCounter uint64

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand is effectively infallible on supported platforms; if it
		// ever fails, fall back to a time+counter value instead of crashing the
		// request path. These IDs are row identifiers, not secrets.
		binary.BigEndian.PutUint64(b, uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(b[8:], atomic.AddUint64(&idFallbackCounter, 1))
	}
	return hex.EncodeToString(b)
}

type SyncError struct {
	Code   string
	Status int
	Msg    string
}

func (e *SyncError) Error() string {
	return e.Msg
}
