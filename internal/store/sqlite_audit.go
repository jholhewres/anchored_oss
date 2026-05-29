package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// PurgeAuditOlderThan deletes audit entries older than before, returning the
// number of rows removed.
func (s *SQLiteStore) PurgeAuditOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < ?`, before.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("purge audit: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *SQLiteStore) AppendAudit(ctx context.Context, entry *model.AuditEntry) error {
	var metadataBytes []byte
	if entry.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
	}

	id := newUUID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, org_id, project_id, actor_id, action, target_type, target_id, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, entry.OrgID, nilIfEmpty(entry.ProjectID), nilIfEmpty(entry.ActorID),
		entry.Action, nilIfEmpty(entry.TargetType), nilIfEmpty(entry.TargetID),
		metadataBytes,
	)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AppendAudits(ctx context.Context, entries []*model.AuditEntry) error {
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

func (s *SQLiteStore) appendAuditsChunk(ctx context.Context, entries []*model.AuditEntry) error {
	args := make([]any, 0, auditInsertCols*len(entries))

	rowPlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?)"
	placeholders := make([]string, 0, len(entries))

	for _, e := range entries {
		var metadataBytes []byte
		if e.Metadata != nil {
			b, err := json.Marshal(e.Metadata)
			if err != nil {
				return fmt.Errorf("marshal audit metadata: %w", err)
			}
			metadataBytes = b
		}
		placeholders = append(placeholders, rowPlaceholder)
		args = append(args,
			newUUID(), e.OrgID, nilIfEmpty(e.ProjectID), nilIfEmpty(e.ActorID),
			e.Action, nilIfEmpty(e.TargetType), nilIfEmpty(e.TargetID),
			metadataBytes,
		)
	}

	query := `INSERT INTO audit_log (id, org_id, project_id, actor_id, action, target_type, target_id, metadata) VALUES ` + strings.Join(placeholders, ",")
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("append audit batch: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAuditEntries(ctx context.Context, orgID string, filters model.AuditFilters) ([]*model.AuditEntry, int, error) {
	conds := []string{"org_id = ?"}
	args := []any{orgID}

	if filters.ProjectID != "" {
		conds = append(conds, "project_id = ?")
		args = append(args, filters.ProjectID)
	}
	if filters.ActorID != "" {
		conds = append(conds, "actor_id = ?")
		args = append(args, filters.ActorID)
	}
	if filters.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, filters.Action)
	}
	if filters.TargetType != "" {
		conds = append(conds, "target_type = ?")
		args = append(args, filters.TargetType)
	}
	if filters.From != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, *filters.From)
	}
	if filters.To != nil {
		conds = append(conds, "created_at <= ?")
		args = append(args, *filters.To)
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
		`SELECT id, org_id, COALESCE(project_id, ''), COALESCE(actor_id, ''),
		        action, COALESCE(target_type, ''), COALESCE(target_id, ''),
		        metadata, created_at, COUNT(*) OVER() AS total
		 FROM audit_log
		 WHERE %s
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		where,
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
			&targetType, &targetID, &metadataBytes, scanTime(&e.CreatedAt), &total,
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
