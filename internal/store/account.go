package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateAccount(ctx context.Context, email, displayName, passwordHash string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO accounts (email, display_name, password_hash) VALUES ($1, $2, $3) RETURNING id, email, display_name, password_hash, created_at`,
		email, displayName, passwordHash,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAccountByID(ctx context.Context, id string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAccountByEmail(ctx context.Context, email string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account by email: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAccountOrgID(ctx context.Context, accountID string) (string, error) {
	var orgID string
	err := s.db.QueryRowContext(ctx,
		`SELECT org_id FROM org_members WHERE account_id = $1 LIMIT 1`,
		accountID,
	).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get account org: %w", err)
	}
	return orgID, nil
}

func (s *PostgresStore) SetAccountPassword(ctx context.Context, accountID, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET password_hash = $1 WHERE id = $2`,
		passwordHash, accountID,
	)
	if err != nil {
		return fmt.Errorf("set account password: %w", err)
	}
	return nil
}

// GetOrCreateAccountByEmail returns an existing account for the email or
// creates a fresh one. The orgID is the membership context: when the account
// is newly created, the caller is expected to also add the org membership
// (this method does not do it on its own to keep the contract single-purpose).
// The boolean return indicates whether the row was created in this call.
func (s *PostgresStore) GetOrCreateAccountByEmail(ctx context.Context, orgID, email, displayName string) (*model.Account, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var a model.Account
	err = tx.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, &a.CreatedAt)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit tx: %w", err)
		}
		return &a, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("lookup account: %w", err)
	}

	if err := tx.QueryRowContext(ctx,
		`INSERT INTO accounts (email, display_name) VALUES ($1, $2)
		 RETURNING id, email, display_name, password_hash, created_at`,
		email, displayName,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, &a.CreatedAt); err != nil {
		return nil, false, fmt.Errorf("create account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit tx: %w", err)
	}
	return &a, true, nil
}

// ListAccountsByOrg returns all accounts that are members of orgID with their
// org-level role. Sorted by display name.
func (s *PostgresStore) ListAccountsByOrg(ctx context.Context, orgID string) ([]*model.AccountWithRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.email, a.display_name, a.created_at, om.role
		 FROM accounts a
		 JOIN org_members om ON om.account_id = a.id
		 WHERE om.org_id = $1
		 ORDER BY a.display_name`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts by org: %w", err)
	}
	defer rows.Close()

	out := make([]*model.AccountWithRole, 0)
	for rows.Next() {
		var r model.AccountWithRole
		if err := rows.Scan(&r.ID, &r.Email, &r.DisplayName, &r.CreatedAt, &r.Role); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return out, nil
}
