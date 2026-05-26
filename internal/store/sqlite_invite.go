package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) CreateInvite(ctx context.Context, orgID, email, displayName, role, tokenHash string, expiresAt time.Time, createdBy string) (*model.Invite, error) {
	id := newUUID()
	var createdByVal any
	if createdBy != "" {
		createdByVal = createdBy
	}
	var inv model.Invite
	var createdByNull sql.NullString
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO invites (id, org_id, email, display_name, role, token_hash, expires_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at`,
		id, orgID, email, displayName, role, tokenHash, expiresAt, createdByVal,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.DisplayName, &inv.Role, &inv.TokenHash,
		scanTime(&inv.ExpiresAt), scanNullTime(&inv.AcceptedAt), &createdByNull, scanTime(&inv.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}
	if createdByNull.Valid {
		inv.CreatedBy = createdByNull.String
	}
	return &inv, nil
}

func (s *SQLiteStore) GetInviteByTokenHash(ctx context.Context, tokenHash string) (*model.Invite, error) {
	var inv model.Invite
	var createdByNull sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at
		 FROM invites WHERE token_hash = ?`,
		tokenHash,
	).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.DisplayName, &inv.Role, &inv.TokenHash,
		scanTime(&inv.ExpiresAt), scanNullTime(&inv.AcceptedAt), &createdByNull, scanTime(&inv.CreatedAt))
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

func (s *SQLiteStore) ListInvitesByOrg(ctx context.Context, orgID string) ([]*model.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, email, display_name, role, token_hash, expires_at, accepted_at, created_by, created_at
		 FROM invites WHERE org_id = ? AND accepted_at IS NULL
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
			scanTime(&inv.ExpiresAt), scanNullTime(&inv.AcceptedAt), &createdByNull, scanTime(&inv.CreatedAt)); err != nil {
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

func (s *SQLiteStore) DeleteInvite(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) MarkInviteAccepted(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE invites SET accepted_at = datetime('now') WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark invite accepted: %w", err)
	}
	return nil
}
