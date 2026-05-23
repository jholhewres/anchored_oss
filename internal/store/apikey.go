package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateAPIKey(ctx context.Context, orgID, accountID, name, keyPrefix, keyHash, scope string, expiresAt *time.Time) (*model.APIKey, error) {
	var k model.APIKey
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO api_keys (org_id, account_id, name, key_prefix, key_hash, scope, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at`,
		orgID, accountID, name, keyPrefix, keyHash, scope, expiresAt,
	).Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &k, nil
}

func (s *PostgresStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error) {
	var k model.APIKey
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at
		 FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
		keyHash,
	).Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return &k, nil
}

// ListAPIKeysByOrg returns every key minted under orgID (active + revoked +
// expired), newest first. Never expose `KeyHash` outside the store — the
// JSON tag is `-` so the field is naturally elided when handlers serialize.
func (s *PostgresStore) ListAPIKeysByOrg(ctx context.Context, orgID string) ([]*model.APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at
		 FROM api_keys WHERE org_id = $1 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys by org: %w", err)
	}
	defer rows.Close()

	out := make([]*model.APIKey, 0)
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.AccountID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}
