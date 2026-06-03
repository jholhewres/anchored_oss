package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// seedDefaultGuardrailsSQLite inserts the default guardrail set using the given
// execer (the org-creation transaction), so an org never commits with a partial
// guardrail set.
func seedDefaultGuardrailsSQLite(ctx context.Context, ex execer, orgID string) error {
	for _, g := range DefaultGuardrails(orgID) {
		if _, err := ex.ExecContext(ctx,
			`INSERT INTO org_guardrails (id, org_id, kind, value, label, description, enabled, builtin)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			newUUID(), g.OrgID, g.Kind, g.Value, g.Label, g.Description, boolToInt(g.Enabled), boolToInt(g.Builtin),
		); err != nil {
			return fmt.Errorf("seed guardrail %q: %w", g.Kind, err)
		}
	}
	return nil
}

func (s *SQLiteStore) ListGuardrails(ctx context.Context, orgID string) ([]*model.Guardrail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, kind, value, label, description, enabled, builtin, created_at, updated_at
		 FROM org_guardrails WHERE org_id = ?
		 ORDER BY builtin DESC, kind, label`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list guardrails: %w", err)
	}
	defer rows.Close()

	out := make([]*model.Guardrail, 0)
	for rows.Next() {
		g, err := scanSQLiteGuardrail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetGuardrail(ctx context.Context, orgID, id string) (*model.Guardrail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, kind, value, label, description, enabled, builtin, created_at, updated_at
		 FROM org_guardrails WHERE org_id = ? AND id = ?`, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("get guardrail: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get guardrail: %w", err)
		}
		return nil, ErrNotFound
	}
	return scanSQLiteGuardrail(rows)
}

func (s *SQLiteStore) CreateGuardrail(ctx context.Context, g *model.Guardrail) error {
	if g.ID == "" {
		g.ID = newUUID()
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO org_guardrails (id, org_id, kind, value, label, description, enabled, builtin)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING created_at, updated_at`,
		g.ID, g.OrgID, g.Kind, g.Value, g.Label, g.Description, boolToInt(g.Enabled), boolToInt(g.Builtin),
	).Scan(scanTime(&g.CreatedAt), scanTime(&g.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create guardrail: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateGuardrail(ctx context.Context, g *model.Guardrail) error {
	var builtin int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE org_guardrails SET value = ?, label = ?, description = ?, enabled = ?, updated_at = datetime('now')
		 WHERE org_id = ? AND id = ?
		 RETURNING kind, builtin, created_at, updated_at`,
		g.Value, g.Label, g.Description, boolToInt(g.Enabled), g.OrgID, g.ID,
	).Scan(&g.Kind, &builtin, scanTime(&g.CreatedAt), scanTime(&g.UpdatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update guardrail: %w", err)
	}
	g.Builtin = builtin != 0
	return nil
}

func (s *SQLiteStore) DeleteGuardrail(ctx context.Context, orgID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM org_guardrails WHERE org_id = ? AND id = ?`, orgID, id)
	if err != nil {
		return fmt.Errorf("delete guardrail: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanSQLiteGuardrail reads one row; SQLite returns BOOLEAN as int64, so enabled
// and builtin are scanned through int64 temps.
func scanSQLiteGuardrail(rows *sql.Rows) (*model.Guardrail, error) {
	var g model.Guardrail
	var enabled, builtin int64
	if err := rows.Scan(&g.ID, &g.OrgID, &g.Kind, &g.Value, &g.Label, &g.Description,
		&enabled, &builtin, scanTime(&g.CreatedAt), scanTime(&g.UpdatedAt)); err != nil {
		return nil, fmt.Errorf("scan guardrail: %w", err)
	}
	g.Enabled = enabled != 0
	g.Builtin = builtin != 0
	return &g, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
