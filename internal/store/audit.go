package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
)

const auditInsertCols = 7
const auditBatchSize = 2000

func (s *PostgresStore) AppendAudit(ctx context.Context, entry *model.AuditEntry) error {
	var metadataBytes []byte
	if entry.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (org_id, project_id, actor_id, action, target_type, target_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.OrgID, nilIfEmpty(entry.ProjectID), nilIfEmpty(entry.ActorID),
		entry.Action, nilIfEmpty(entry.TargetType), nilIfEmpty(entry.TargetID),
		metadataBytes,
	)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// AppendAudits inserts a batch of audit entries with a single statement.
func (s *PostgresStore) AppendAudits(ctx context.Context, entries []*model.AuditEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for start := 0; start < len(entries); start += auditBatchSize {
		end := start + auditBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		if err := s.appendAuditsChunk(ctx, entries[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) appendAuditsChunk(ctx context.Context, entries []*model.AuditEntry) error {
	args := make([]any, 0, auditInsertCols*len(entries))
	placeholders := make([]string, 0, len(entries))
	for i, e := range entries {
		var metadataBytes []byte
		if e.Metadata != nil {
			b, err := json.Marshal(e.Metadata)
			if err != nil {
				return fmt.Errorf("marshal audit metadata: %w", err)
			}
			metadataBytes = b
		}
		base := i * auditInsertCols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
		))
		args = append(args,
			e.OrgID, nilIfEmpty(e.ProjectID), nilIfEmpty(e.ActorID),
			e.Action, nilIfEmpty(e.TargetType), nilIfEmpty(e.TargetID),
			metadataBytes,
		)
	}
	query := `INSERT INTO audit_log (org_id, project_id, actor_id, action, target_type, target_id, metadata) VALUES ` + strings.Join(placeholders, ",")
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("append audit batch: %w", err)
	}
	return nil
}

const (
	auditDefaultLimit = 50
	auditMaxLimit     = 500
)

// ListAuditEntries reads audit log rows for orgID filtered by an optional
// project/actor/action/target_type/from/to envelope. Limit is clamped to
// [1, auditMaxLimit] with auditDefaultLimit when not specified. Returns the
// total count for pagination plus the current page.
func (s *PostgresStore) ListAuditEntries(ctx context.Context, orgID string, filters model.AuditFilters) ([]*model.AuditEntry, int, error) {
	conds := []string{"org_id = $1"}
	args := []any{orgID}
	argIdx := 2

	if filters.ProjectID != "" {
		conds = append(conds, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, filters.ProjectID)
		argIdx++
	}
	if filters.ActorID != "" {
		conds = append(conds, fmt.Sprintf("actor_id = $%d", argIdx))
		args = append(args, filters.ActorID)
		argIdx++
	}
	if filters.Action != "" {
		conds = append(conds, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filters.Action)
		argIdx++
	}
	if filters.TargetType != "" {
		conds = append(conds, fmt.Sprintf("target_type = $%d", argIdx))
		args = append(args, filters.TargetType)
		argIdx++
	}
	if filters.From != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.From)
		argIdx++
	}
	if filters.To != nil {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filters.To)
		argIdx++
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = auditDefaultLimit
	}
	if limit > auditMaxLimit {
		limit = auditMaxLimit
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	where := strings.Join(conds, " AND ")

	query := fmt.Sprintf(
		`SELECT id, org_id, COALESCE(project_id::text, ''), COALESCE(actor_id::text, ''),
		        action, COALESCE(target_type, ''), COALESCE(target_id, ''),
		        metadata, created_at, COUNT(*) OVER() AS total
		 FROM audit_log
		 WHERE %s
		 ORDER BY created_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()

	entries := make([]*model.AuditEntry, 0, limit)
	total := 0
	for rows.Next() {
		var e model.AuditEntry
		var projectID, actorID, targetType, targetID string
		var metadataBytes []byte
		if err := rows.Scan(
			&e.ID, &e.OrgID, &projectID, &actorID, &e.Action,
			&targetType, &targetID, &metadataBytes, &e.CreatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit entry: %w", err)
		}
		e.ProjectID = projectID
		e.ActorID = actorID
		e.TargetType = targetType
		e.TargetID = targetID
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &e.Metadata); err != nil {
				return nil, 0, fmt.Errorf("unmarshal audit metadata: %w", err)
			}
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit entries: %w", err)
	}
	return entries, total, nil
}

// nilIfEmpty returns nil for empty strings so nullable columns get NULL.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
