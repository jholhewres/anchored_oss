package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateInvite(ctx context.Context, orgID, email, displayName, role, tokenHash string, expiresAt time.Time, createdBy string) (*model.Invite, error) {
	var inv model.Invite
	var createdByNull sql.NullString
	if createdBy != "" {
		createdByNull = sql.NullString{String: createdBy, Valid: true}
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO invites (org_id, email, display_name, role, token_hash, expires_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at`,
		orgID, email, displayName, role, tokenHash, expiresAt, createdByNull,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.DisplayName, &inv.Role, &inv.TokenHash,
		&inv.ExpiresAt, &inv.AcceptedAt, &createdByNull, &inv.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}
	if createdByNull.Valid {
		inv.CreatedBy = createdByNull.String
	}
	return &inv, nil
}

func (s *PostgresStore) GetInviteByTokenHash(ctx context.Context, tokenHash string) (*model.Invite, error) {
	var inv model.Invite
	var createdByNull sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at
		 FROM invites WHERE token_hash = $1`,
		tokenHash,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.DisplayName, &inv.Role, &inv.TokenHash,
		&inv.ExpiresAt, &inv.AcceptedAt, &createdByNull, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get invite by token hash: %w", err)
	}
	if createdByNull.Valid {
		inv.CreatedBy = createdByNull.String
	}
	return &inv, nil
}

func (s *PostgresStore) ListInvitesByOrg(ctx context.Context, orgID string) ([]*model.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at
		 FROM invites WHERE org_id = $1 AND accepted_at IS NULL
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	var out []*model.Invite
	for rows.Next() {
		var inv model.Invite
		var createdByNull sql.NullString
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.DisplayName, &inv.Role, &inv.TokenHash,
			&inv.ExpiresAt, &inv.AcceptedAt, &createdByNull, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		if createdByNull.Valid {
			inv.CreatedBy = createdByNull.String
		}
		out = append(out, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invites: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) DeleteInvite(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) MarkInviteAccepted(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE invites SET accepted_at = now() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark invite accepted: %w", err)
	}
	return nil
}
