package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresStore implements Store backed by PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// PoolConfig controls the database/sql connection pool.
type PoolConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (c PoolConfig) withDefaults() PoolConfig {
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 10
	}
	if c.MaxIdleConns < 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = 5 * time.Minute
	}
	return c
}

// NewPostgresStore opens a connection pool, runs migrations, and returns a ready store.
func NewPostgresStore(cfg PoolConfig) (*PostgresStore, error) {
	cfg = cfg.withDefaults()
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres store: dsn is required")
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

// Ping verifies the database connection.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the connection pool.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
