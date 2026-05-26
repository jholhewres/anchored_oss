package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type SyncEngine struct {
	store  store.Store
	filter *policy.ContentFilter
	logger *slog.Logger
}

func NewSyncEngine(st store.Store, f *policy.ContentFilter, logger *slog.Logger) *SyncEngine {
	return &SyncEngine{
		store:  st,
		filter: f,
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

	var results []model.SyncResult

	pushResults, err := e.handlePushes(ctx, accountID, orgID, projectID, req.Pushes)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	results = append(results, pushResults...)

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

	return &model.SyncResponse{
		Pulls:            pulls,
		ServerTombstones: serverTombstones,
		Results:          results,
		Watermark:        watermark,
	}, nil
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

	slug := toSlug(claim.Name)
	if slug == "" {
		return "", &SyncError{Code: "INVALID_REQUEST", Status: 400, Msg: "project_claim.name does not produce a valid slug"}
	}

	proj, err := e.store.CreateProject(ctx, orgID, claim.Name, slug, claim.RemoteKey, accountID, "other")
	if err != nil {
		e.logger.Error("project creation from claim failed", "remote_key", claim.RemoteKey, "error", err)
		return "", &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "failed to create project"}
	}

	// Grant the creator access via the org's default team so the very
	// same sync request can proceed past authorize().
	if err := e.store.EnsureCreatorProjectAccess(ctx, orgID, accountID, proj.ID); err != nil {
		e.logger.Error("grant creator access failed", "project_id", proj.ID, "error", err)
		return "", &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "failed to grant project access"}
	}

	e.appendAudit(ctx, orgID, proj.ID, accountID, "sync.project.created", "project", proj.ID, claim.RemoteKey)
	return proj.ID, nil
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
	filterResults := e.filter.Filter(filterables)

	results := make([]model.SyncResult, len(pushes))
	accepted := make([]*model.Memory, 0, len(pushes))
	auditEntries := make([]*model.AuditEntry, 0, len(pushes))

	for i, push := range pushes {
		fr := filterResults[i]
		if !fr.Accepted {
			results[i] = model.SyncResult{
				ID:     push.ID,
				Status: "rejected",
				Rule:   fr.Rule,
				Detail: fr.Detail,
			}
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

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
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
