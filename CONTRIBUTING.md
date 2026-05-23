# Contributing

Thanks for considering a contribution! Anchored OSS is a small Go server with an embedded React admin UI. Backend-only changes do not require a Node.js toolchain; UI changes do.

## Prerequisites

| For | You need |
|---|---|
| Backend / API / store changes | Go **1.25+**, Docker (for the Postgres dev container) |
| UI changes | Go **1.25+**, **Node 20+**, Docker |
| Building the production binary | Same as UI changes |

## Repo layout

```
cmd/server/         # HTTP server entrypoint + bootstrap
internal/auth/      # API key generation + hashing
internal/config/    # YAML + env config loader
internal/handler/   # REST handlers
internal/middleware/# auth, CORS, body limit, logging, recovery
internal/model/     # shared DTOs
internal/policy/    # guardrails (local paths, secrets, blocked categories)
internal/server/    # http.Server wiring
internal/store/     # Postgres store + migrations
internal/sync/      # bidirectional sync engine
internal/web/       # go:embed wrapper for the dashboard bundle
internal/version/   # build-time version
web/                # Vite + React + Tailwind + shadcn admin UI
docs/               # protocol + error reference + plans
```

## Backend workflow

```bash
make db-up                       # Postgres on :5433
DATABASE_URL=postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable \
  make build
DATABASE_URL=...                ./bin/anchored-oss -bootstrap   # one-off
DATABASE_URL=...                ./bin/anchored-oss              # serve
go vet ./...                    # vet
go test -count=1 -race ./...    # unit tests
make test-integration            # Postgres-backed store tests (//go:build integration)
```

`make build` works without Node.js — the committed stub `internal/web/dist/index.html` keeps `go:embed` happy. The dashboard route just renders the stub until you run a real `make web-build`.

## Frontend workflow

```bash
# Terminal 1 — Go server
make db-up
DATABASE_URL=... make run

# Terminal 2 — Vite dev server (HMR)
make web-dev
# open http://localhost:5173
```

Vite proxies `/v1/*` and `/api/*` to `http://localhost:8080` so the dashboard talks to the same Go API. For a single-binary smoke test, run `make build-prod` (which performs `web-build` then `go build`) or `docker compose up --build`.

## Adding a new endpoint

1. Add (or extend) a method on the `Store` interface in `internal/store/store.go`, then implement it in the matching `internal/store/<area>.go` file. Migrations live in `internal/store/migrations.sql.go` — bump `schemaVersion` and add an entry to the `migrations` map.
2. Create or extend a handler under `internal/handler/`. Use `middleware.GetAccountID/GetOrgID/GetScope` for context, `jsonError`/`jsonResponse` for responses.
3. Wire the route in `internal/server/server.go` with the right `authMW` / `requireAdmin` chain.
4. If the dashboard should consume it, add the typed call to `web/src/lib/api.ts` and a TanStack Query hook in the relevant page.

## Adding a new page

1. Drop a `*.tsx` under `web/src/pages/`.
2. Register the route in `web/src/App.tsx`.
3. Add a nav entry to `web/src/components/layout/Sidebar.tsx` with the right `adminOnly` flag if it's a privileged surface.

## Style

- Backend follows `gofmt` defaults; `go vet ./...` must pass.
- Frontend is TypeScript strict, no inline scripts (CSP forbids them). Prefer existing shadcn/ui primitives in `web/src/components/ui/`.
- Commits should be small and thematic. Open a PR description that explains the *why*, not just the *what*.

## Quick sanity check

Before opening a PR, the same gates CI runs:

```bash
cd web && npm install --no-audit --no-fund && npm run build && cd ..
go mod tidy
go vet ./...
go test -count=1 -race ./...
docker compose build server
```
