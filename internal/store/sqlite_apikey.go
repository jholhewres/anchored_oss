package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) CreateAPIKey(ctx context.Context, orgID, accountID, name, keyPrefix, keyHash, scope string, expiresAt *time.Time) (*model.APIKey, error) {
	id := newUUID()
	var k model.APIKey
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO api_keys (id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at`,
		id, orgID, accountID, name, keyPrefix, keyHash, scope, expiresAt,
	).Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, scanNullTime(&k.ExpiresAt), scanTime(&k.CreatedAt), scanNullTime(&k.RevokedAt))
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &k, nil
}

func (s *SQLiteStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error) {
	var k model.APIKey
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at
		 FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL`,
		keyHash,
	).Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, scanNullTime(&k.ExpiresAt), scanTime(&k.CreatedAt), scanNullTime(&k.RevokedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return &k, nil
}

func (s *SQLiteStore) ListAPIKeysByOrg(ctx context.Context, orgID string) ([]*model.APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at
		 FROM api_keys WHERE org_id = ? ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys by org: %w", err)
	}
	defer rows.Close()

	out := make([]*model.APIKey, 0)
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, scanNullTime(&k.ExpiresAt), scanTime(&k.CreatedAt), scanNullTime(&k.RevokedAt)); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = datetime('now') WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RevokeSessionKeys(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = datetime('now') WHERE account_id = ? AND name = 'session' AND revoked_at IS NULL`,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("revoke session keys: %w", err)
	}
	return nil
}
