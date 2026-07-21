package store

import (
	"context"
	"fmt"
)

func (s *PostgresStore) GetOrgStorageBytes(ctx context.Context, orgID string) (int64, error) {
	var bytes int64
	err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE((
		    SELECT SUM(OCTET_LENGTH(m.content))
		    FROM memories m
		    JOIN projects p ON p.id = m.project_id
		    WHERE p.org_id = $1
		      AND m.deleted_at IS NULL
		      AND p.deleted_at IS NULL
		  ), 0)
		  +
		  COALESCE((
		    SELECT SUM(OCTET_LENGTH(i.response_json::text))
		    FROM memory_write_idempotency i
		    WHERE i.org_scope = $1
		  ), 0)
	`, orgID).Scan(&bytes)
	if err != nil {
		return 0, fmt.Errorf("get org storage bytes: %w", err)
	}
	return bytes, nil
}
