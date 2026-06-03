package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// DefaultGuardrails is the seed set applied when an org is created. The three
// security rules are builtins (toggleable but not deletable); the event /
// preference category blocks are deletable. All start enabled — admins remove
// or adjust them, and can add custom regex/keyword rules on top.
func DefaultGuardrails(orgID string) []*model.Guardrail {
	return []*model.Guardrail{
		{OrgID: orgID, Kind: model.GuardrailSecretDetection, Label: "Secret detection",
			Description: "Reject memories containing API keys, tokens, AWS keys, or credential URIs.",
			Enabled:     true, Builtin: true},
		{OrgID: orgID, Kind: model.GuardrailLocalPathRedaction, Label: "Local path block",
			Description: "Reject memories containing absolute local paths (/home, /Users, C:\\...). Use repo-relative paths.",
			Enabled:     true, Builtin: true},
		{OrgID: orgID, Kind: model.GuardrailUserScopeBlock, Label: "User-scope block",
			Description: "Reject memories scoped to a single user (personal, not team-shared).",
			Enabled:     true, Builtin: true},
		{OrgID: orgID, Kind: model.GuardrailCategory, Value: "event", Label: "Block category: event",
			Description: "Session events are transient and machine-local.", Enabled: true},
		{OrgID: orgID, Kind: model.GuardrailCategory, Value: "preference", Label: "Block category: preference",
			Description: "User preferences are personal, not team knowledge.", Enabled: true},
	}
}

// execer is satisfied by *sql.DB and *sql.Tx, so seeding can run inside the
// org-creation transaction (all-or-nothing: an org never commits with a partial
// guardrail set, which would silently disable security rules in the sync engine).
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// seedDefaultGuardrailsPG inserts the default guardrail set using the given
// execer (typically the org-creation transaction).
func seedDefaultGuardrailsPG(ctx context.Context, ex execer, orgID string) error {
	for _, g := range DefaultGuardrails(orgID) {
		if _, err := ex.ExecContext(ctx,
			`INSERT INTO org_guardrails (id, org_id, kind, value, label, description, enabled, builtin)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			newUUID(), g.OrgID, g.Kind, g.Value, g.Label, g.Description, g.Enabled, g.Builtin,
		); err != nil {
			return fmt.Errorf("seed guardrail %q: %w", g.Kind, err)
		}
	}
	return nil
}

func (s *PostgresStore) ListGuardrails(ctx context.Context, orgID string) ([]*model.Guardrail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, kind, value, label, description, enabled, builtin, created_at, updated_at
		 FROM org_guardrails WHERE org_id = $1
		 ORDER BY builtin DESC, kind, label`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list guardrails: %w", err)
	}
	defer rows.Close()

	out := make([]*model.Guardrail, 0)
	for rows.Next() {
		var g model.Guardrail
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Kind, &g.Value, &g.Label, &g.Description,
			&g.Enabled, &g.Builtin, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan guardrail: %w", err)
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetGuardrail(ctx context.Context, orgID, id string) (*model.Guardrail, error) {
	var g model.Guardrail
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, kind, value, label, description, enabled, builtin, created_at, updated_at
		 FROM org_guardrails WHERE org_id = $1 AND id = $2`, orgID, id,
	).Scan(&g.ID, &g.OrgID, &g.Kind, &g.Value, &g.Label, &g.Description,
		&g.Enabled, &g.Builtin, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get guardrail: %w", err)
	}
	return &g, nil
}

func (s *PostgresStore) CreateGuardrail(ctx context.Context, g *model.Guardrail) error {
	if g.ID == "" {
		g.ID = newUUID()
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO org_guardrails (id, org_id, kind, value, label, description, enabled, builtin)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at, updated_at`,
		g.ID, g.OrgID, g.Kind, g.Value, g.Label, g.Description, g.Enabled, g.Builtin,
	).Scan(&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create guardrail: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateGuardrail(ctx context.Context, g *model.Guardrail) error {
	err := s.db.QueryRowContext(ctx,
		`UPDATE org_guardrails SET value = $3, label = $4, description = $5, enabled = $6, updated_at = now()
		 WHERE org_id = $1 AND id = $2
		 RETURNING kind, builtin, created_at, updated_at`,
		g.OrgID, g.ID, g.Value, g.Label, g.Description, g.Enabled,
	).Scan(&g.Kind, &g.Builtin, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update guardrail: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteGuardrail(ctx context.Context, orgID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM org_guardrails WHERE org_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return fmt.Errorf("delete guardrail: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
