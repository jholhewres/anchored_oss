# Development

Anchored OSS is a Go server with an embedded React (Vite + shadcn/ui)
dashboard, compiled into a single static binary.

## Repository layout

```text
anchored_oss/
├── cmd/server/         # HTTP server entrypoint + bootstrap
├── internal/auth/      # API key generation and hashing
├── internal/config/    # YAML + env config loader
├── internal/handler/   # REST handlers (health, sync, projects, api-keys)
├── internal/middleware/# auth, CORS, body limit, logging, recovery
├── internal/model/     # shared DTOs and domain types
├── internal/policy/    # guardrail content filter (secrets, paths, categories, custom rules)
├── internal/server/    # http.Server wiring
├── internal/store/     # Postgres + SQLite stores, interface, and migrations
├── internal/sync/      # bidirectional sync engine
├── internal/version/   # build-time version
├── web/                # React dashboard (Vite)
├── docs/               # protocol, API, deployment, and error reference
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Building

- `make build` — Go-only build (CGO disabled; pure-Go SQLite + Postgres).
  Uses the committed stub `internal/web/dist/index.html`. No Node.js required.
- `make build-prod` — runs `make web-build` first (needs **Node 20+**), then
  `make build`. Produces a binary with the full UI embedded.
- `docker compose build` / `docker compose up --build` — always rebuilds the
  UI as a node-alpine stage and embeds it. No host Node.js needed.
- `make web-dev` — starts Vite on port `5173` with `/v1/*` and `/api/*`
  proxied to `localhost:8080`. Use it together with `make run` for
  HMR-driven frontend work.

The UI is served by the same Go binary (no separate process or CDN). API
paths (`/v1/*`, `/api/v1/*`) take precedence; unknown non-API paths fall
back to `index.html` so SPA deep links survive page refreshes.

## Testing

```bash
go test ./...             # unit + store/integration tests
go vet ./...
cd web && npm run build   # type-check + bundle the dashboard
```

## Conventions

- English-first for code, comments, commits, and docs.
- Conventional Commit messages (`feat:`, `fix:`, `docs:`, `chore:` ...),
  small and thematic.
- Keep the two store backends in parity: any new query must be implemented
  for **both** Postgres (`internal/store/*.go`) and SQLite
  (`internal/store/sqlite_*.go`). The pure-Go SQLite driver returns
  `DATETIME` columns as strings — wrap timestamp scans with
  `scanTime`/`scanNullTime` from `internal/store/sqlite_helpers.go`.
- Add tests for new behavior; preserve the privacy guardrails.

- Keep a Postgres twin for every store rule the SQLite suite checks: write
  the assertions once as a `check…(t, st Store)` helper, call it from the
  SQLite test and from a `…_Postgres` test under `//go:build integration`
  (`freshPostgresStore` gives a twin a database of its own). CI fails when a
  twin is skipped.

## Migrations

Migrations live in `internal/store/migrations.sql.go` (Postgres) and
`internal/store/sqlite_migrations.go` (SQLite), numbered in step on both
backends; `schemaVersion` is the highest applied. 025 is the last one on
`main`. Work on v0.7 runs in parallel, so the next numbers are reserved per
change to keep two branches from taking the same one:

| Migration | Reserved for |
|---|---|
| 026 | Dashboard cookie sessions |
| 027 | First run without a fixed password or open registration |
| 028 | Accounts in several orgs |
| 029–031 | OAuth: discovery and consent, token endpoint, client registration (CIMD/DCR) |
| 032 | Seven memory categories, four memory scopes, lifecycle fields |
| 033 | Sync that keeps the latest edit and never resurrects a deleted memory |
| 034 | `/v1/import/batches` for `anchored migrate` |
| 035 | BM25 search fused with vectors |

A change that needs a migration outside this table takes the next free number
after 035 and adds its row here in the same commit.

See [ARCHITECTURE.md](../ARCHITECTURE.md) for a system map before diving in.
