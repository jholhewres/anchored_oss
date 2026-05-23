package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateAccount(ctx context.Context, email, displayName string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO accounts (email, display_name) VALUES ($1, $2) RETURNING id, email, display_name, created_at`,
		email, displayName,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAccountByID(ctx context.Context, id string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at FROM accounts WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAccountByEmail(ctx context.Context, email string) (*model.Account, error) {
	var a model.Account
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at FROM accounts WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.Email, &a.DisplayName, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get account by email: %w", err)
	}
	return &a, nil
}
