package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
)

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

func (s *PostgresStore) QueryAuditLog(ctx context.Context, orgID string, filters model.AuditFilters) ([]*model.AuditEntry, error) {
	var (
		conds   []string
		args    []any
		argIdx  = 1
	)

	conds = append(conds, fmt.Sprintf("org_id = $%d", argIdx))
	args = append(args, orgID)
	argIdx++

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

	limit := 100
	if filters.Limit > 0 {
		limit = filters.Limit
	}
	offset := 0
	if filters.Offset > 0 {
		offset = filters.Offset
	}

	query := fmt.Sprintf(
		`SELECT id, org_id, COALESCE(project_id::text, ''), COALESCE(actor_id::text, ''), action, COALESCE(target_type, ''), COALESCE(target_id, ''), metadata, created_at
		 FROM audit_log WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		strings.Join(conds, " AND "), argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	var entries []*model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		var metadataBytes []byte
		var projectID, actorID, targetType, targetID string
		if err := rows.Scan(&e.ID, &e.OrgID, &projectID, &actorID, &e.Action, &targetType, &targetID, &metadataBytes, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		e.ProjectID = projectID
		e.ActorID = actorID
		e.TargetType = targetType
		e.TargetID = targetID
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &e.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal audit metadata: %w", err)
			}
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit entries: %w", err)
	}
	return entries, nil
}

// nilIfEmpty returns nil for empty strings so nullable columns get NULL.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

