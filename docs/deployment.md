# Deployment

Anchored OSS ships as a single static binary with the admin dashboard
embedded. Pick the path that fits your infrastructure.

## Installer script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/jholhewres/anchored_oss/main/install/install.sh | sh
```

The installer:

- downloads the latest release binary (checksum-verified),
- ensures Node.js + PM2 are present (apt/dnf with sudo, nvm fallback) and
  registers the server as a PM2 app with boot persistence,
- generates a default `config.yaml` (SQLite storage, port `8771`, bound to
  `0.0.0.0`),
- leaves org/admin/project creation to the first-run onboarding wizard in
  the dashboard.

A running server also serves the same installers from its embedded
`/install` (anchored CLI) and `/install-oss` (server) endpoints, so
deployments can distribute them from their own host without depending on
GitHub.

## Docker Compose

```bash
# 1. Bring up Postgres + server
docker compose up -d

# 2. Bootstrap the default org, admin account, and an admin API key.
#    The plain-text key prints to stdout — copy it once, it cannot be retrieved later.
docker compose run --rm server -bootstrap

# 3. Health check
curl http://localhost:8080/v1/health
```

## From source

```bash
make db-up      # start only Postgres
make build      # build ./bin/anchored-oss
DATABASE_URL=postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable \
  ./bin/anchored-oss -bootstrap
DATABASE_URL=postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable \
  ./bin/anchored-oss
```

## Configuration

Configuration can be supplied via `config.yaml` (see
[`config.example.yaml`](../config.example.yaml)), environment variables, or a
`.env` file. `database.dsn` (or `DATABASE_URL`) is required.

| Env var | Overrides | Notes |
|---|---|---|
| `PORT` | `server.address` port | Default installer port is `8771`. |
| `DATABASE_URL` | `database.dsn` | `postgres://...` or a SQLite file path. |
| `MODE` | `mode.type` | `selfhosted` (default) or `cloud`. |
| `CORS_ALLOWED_ORIGINS` | `cors.allowed_origins` | Comma-separated explicit origins. |
| `RATE_LIMIT_ENABLED` / `RATE_LIMIT_RPM` | `rate_limit.*` | Token-bucket limiter. |

### Storage backends

- **SQLite** (default for the installer) — zero-ops single file, pure-Go
  driver, ideal for small teams and trials.
- **PostgreSQL** — recommended for production teams; enables pgvector-backed
  semantic search when embeddings are configured.

## Updating

Releases are published on GitHub with checksums. To update a PM2-managed
install:

```bash
cd /tmp
curl -sLO https://github.com/jholhewres/anchored_oss/releases/latest/download/anchored-oss-selfhosted-linux-amd64
curl -sLO https://github.com/jholhewres/anchored_oss/releases/latest/download/checksums-sha256.txt
grep selfhosted-linux-amd64 checksums-sha256.txt | sha256sum -c -
cp ~/.anchored-oss/bin/anchored-oss ~/.anchored-oss/bin/anchored-oss.bak
mv anchored-oss-selfhosted-linux-amd64 ~/.anchored-oss/bin/anchored-oss
chmod +x ~/.anchored-oss/bin/anchored-oss
pm2 restart anchored-oss
curl -s http://localhost:8771/v1/health
```
