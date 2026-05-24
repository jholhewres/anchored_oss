package setup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const dockerComposeYAML = `version: "3.8"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: anchored
      POSTGRES_PASSWORD: anchored
      POSTGRES_DB: anchored_oss
    ports:
      - "5432:5432"
    volumes:
      - anchored_pgdata:/var/lib/postgresql/data
volumes:
  anchored_pgdata:
`

// GenerateDockerCompose writes a docker-compose.yml for PostgreSQL into the
// current working directory and returns the DSN that matches the compose file.
func GenerateDockerCompose() (dsn string, err error) {
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "", fmt.Errorf("docker-compose.yml already exists; please remove it first or configure PostgreSQL manually")
	}
	if err := os.WriteFile("docker-compose.yml", []byte(dockerComposeYAML), 0o644); err != nil {
		return "", fmt.Errorf("write docker-compose.yml: %w", err)
	}
	return "postgres://anchored:anchored@localhost:5432/anchored_oss?sslmode=disable", nil
}

// WaitForPostgres polls the given DSN until the database accepts connections
// or the timeout expires.
func WaitForPostgres(dsn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		err = db.PingContext(ctx)
		cancel()
		db.Close()
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("postgres not ready after %s", timeout)
}
