package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

	watermark := time.Now().UTC()

	if len(results) == 0 {
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
		if err != nil {
			return "", &SyncError{Code: "PROJECT_NOT_FOUND", Status: 404, Msg: "project not found"}
		}
		if p.OrgID != orgID {
			return "", &SyncError{Code: "FORBIDDEN", Status: 403, Msg: "project belongs to a different organization"}
		}
		return p.ID, nil
	}

	claim := req.ProjectClaim
	if claim != nil {
		if bad := hasLocalPath(claim.RemoteKey, claim.Name, claim.RepoSlug); bad != "" {
			return "", &SyncError{Code: "LOCAL_PATH_DETECTED", Status: 400, Msg: fmt.Sprintf("claim field contains local path pattern: %s", bad)}
		}

		existing, err := e.store.GetProjectByRemoteKey(ctx, orgID, claim.RemoteKey)
		if err == nil && existing != nil {
			return existing.ID, nil
		}

		proj, err := e.store.CreateProject(ctx, orgID, claim.Name, toSlug(claim.Name), claim.RemoteKey, accountID)
		if err != nil {
			e.logger.Error("project creation from claim failed", "remote_key", claim.RemoteKey, "error", err)
			return "", &SyncError{Code: "INTERNAL_ERROR", Status: 500, Msg: "failed to create project"}
		}

		e.appendAudit(ctx, orgID, proj.ID, accountID, "sync.project.created", "project", proj.ID, claim.RemoteKey)
		return proj.ID, nil
	}

	return "", &SyncError{Code: "INVALID_REQUEST", Status: 400, Msg: "project_id or project_claim is required"}
}

func (e *SyncEngine) authorize(ctx context.Context, accountID, projectID string) error {
	projects, err := e.store.ListProjectsByTeamAccess(ctx, accountID)
	if err != nil {
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
		filterables[i] = policy.Filterable{
			ID:       p.ID,
			Content:  p.Content,
			Category: p.Category,
		}
	}
	filterResults := e.filter.Filter(filterables)

	results := make([]model.SyncResult, len(pushes))
	for i, push := range pushes {
		fr := filterResults[i]
		if !fr.Accepted {
			results[i] = model.SyncResult{
				ID:     push.ID,
				Status: "rejected",
				Rule:   fr.Rule,
				Detail: fr.Detail,
			}
			e.appendAudit(ctx, orgID, projectID, accountID, "sync.push.rejected", "memory", push.ID, fr.Rule)
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
		}
		if mem.ID == "" {
			mem.ID = newID()
		}

		if err := e.store.UpsertMemory(ctx, mem); err != nil {
			e.logger.Error("upsert memory failed", "id", push.ID, "error", err)
			results[i] = model.SyncResult{
				ID:     push.ID,
				Status: "rejected",
				Rule:   "internal_error",
				Detail: "failed to store memory",
			}
			continue
		}

		results[i] = model.SyncResult{
			ID:     mem.ID,
			Status: "accepted",
		}
		e.appendAudit(ctx, orgID, projectID, accountID, "sync.push.accepted", "memory", mem.ID, "")
	}

	return results, nil
}

func (e *SyncEngine) handleTombstones(ctx context.Context, orgID, projectID string, tombstones []string) ([]model.SyncResult, error) {
	if len(tombstones) == 0 {
		return nil, nil
	}

	results := make([]model.SyncResult, len(tombstones))
	for i, id := range tombstones {
		if err := e.store.SoftDeleteMemory(ctx, id, projectID); err != nil {
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

var claimLocalPatterns = []string{"/home/", "/Users/", `C:\Users\`, "C:/Users/", "~/"}

func hasLocalPath(fields ...string) string {
	for _, f := range fields {
		for _, p := range claimLocalPatterns {
			if strings.Contains(f, p) {
				return p
			}
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
	return result.String()
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
