package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) CreateAccount(ctx context.Context, email, displayName, passwordHash string) (*model.Account, error) {
	id := newUUID()
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO accounts (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)
		 RETURNING id, email, display_name, password_hash, created_at`,
		id, email, displayName, passwordHash,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, scanTime(&a.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &a, nil
}

func (s *SQLiteStore) GetAccountByID(ctx context.Context, id string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE id = ?`,
		id,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, scanTime(&a.CreatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return &a, nil
}

func (s *SQLiteStore) GetAccountByEmail(ctx context.Context, email string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE email = ?`,
		email,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, scanTime(&a.CreatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account by email: %w", err)
	}
	return &a, nil
}

func (s *SQLiteStore) GetAccountOrgID(ctx context.Context, accountID string) (string, error) {
	var orgID string
	err := s.db.QueryRowContext(ctx,
		`SELECT org_id FROM org_members WHERE account_id = ? LIMIT 1`,
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

func (s *SQLiteStore) SetAccountPassword(ctx context.Context, accountID, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET password_hash = ? WHERE id = ?`,
		passwordHash, accountID,
	)
	if err != nil {
		return fmt.Errorf("set account password: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetOrCreateAccountByEmail(ctx context.Context, orgID, email, displayName string) (*model.Account, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var a model.Account
	err = tx.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, created_at FROM accounts WHERE email = ?`,
		email,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.PasswordHash, scanTime(&a.CreatedAt))
	if err == nil {
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit tx: %w", err)
		}
		return &a, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("lookup account: %w", err)
	}

	id := newUUID()
	var pwHash sql.NullString
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO accounts (id, email, display_name) VALUES (?, ?, ?)
		 RETURNING id, email, display_name, password_hash, created_at`,
		id, email, displayName,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &pwHash, scanTime(&a.CreatedAt)); err != nil {
		return nil, false, fmt.Errorf("create account: %w", err)
	}
	a.PasswordHash = pwHash.String

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit tx: %w", err)
	}
	return &a, true, nil
}

func (s *SQLiteStore) ListAccountsByOrg(ctx context.Context, orgID string) ([]*model.AccountWithRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.email, a.display_name, a.created_at, om.role
		 FROM accounts a
		 JOIN org_members om ON om.account_id = a.id
		 WHERE om.org_id = ?
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
		if err := rows.Scan(&r.ID, &r.Email, &r.DisplayName, scanTime(&r.CreatedAt), &r.Role); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return out, nil
}
