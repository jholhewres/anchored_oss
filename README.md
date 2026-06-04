# Anchored OSS

> Self-hosted team memory sync for Anchored — local-first, privacy-first, organization-owned knowledge for AI coding agents.

Anchored OSS is the open/self-hosted team layer for Anchored's local-first memory system. The local Anchored binary keeps personal memory, preferences, embeddings, and agent context on the developer's machine. Anchored OSS stores only organization/project-level knowledge that is safe to share with a team.

## Description

Anchored OSS is the self-hosted remote for Anchored. It lets companies run their own shared memory server so development teams can sync project facts, decisions, learnings, summaries, and knowledge graph relationships across Claude Code, Cursor, OpenCode, Gemini CLI, and other MCP-compatible tools.

It is designed as the open/self-hosted counterpart to the future Anchored Cloud service. Both should share the same protocol and privacy model: local-first clients, organization-owned projects, team-based access, and strict guardrails against leaking personal developer data.

## Quick Start

### Quick installers

```bash
# Anchored local MCP memory server / CLI
curl -fsSL https://raw.githubusercontent.com/jholhewres/anchored/main/install/install.sh | bash

# Anchored OSS self-hosted team server
curl -fsSL https://raw.githubusercontent.com/jholhewres/anchored_oss/main/install/install.sh | sh
```

This repository (`jholhewres/anchored_oss`) is the canonical, public source for
the self-hosted server. The running server also serves the same installers from
its embedded `/install` and `/install-oss` endpoints, so deployments can pull
them from their own server without depending on GitHub.

### Docker Compose

```bash
# 1. Bring up Postgres + server
docker compose up -d

# 2. Bootstrap the default org, admin account, and an admin API key.
#    The plain-text key prints to stdout — copy it once, it cannot be retrieved later.
docker compose run --rm server -bootstrap

# 3. Health check
curl http://localhost:8080/v1/health
```

### Local Go

```bash
make db-up      # start only Postgres
make build      # build ./bin/anchored-oss
DATABASE_URL=postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable \
  ./bin/anchored-oss -bootstrap
DATABASE_URL=postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable \
  ./bin/anchored-oss
```

Config can be supplied via `config.yaml` (see `config.example.yaml`), environment variables (`PORT`, `DATABASE_URL`, `CORS_ALLOWED_ORIGINS`), or a `.env` file. `database.dsn` is required.

## Admin Dashboard

The server ships with a React + shadcn/ui admin dashboard embedded directly into the binary. Open `http://localhost:8080` after bootstrap (or complete the first-run onboarding wizard), and you have access to Overview, Projects, Developers, API keys, Guardrails, Audit, and Health screens.

### Building the dashboard

- `make build` — Go-only build. Uses the committed stub `internal/web/dist/index.html`. No Node.js required.
- `make build-prod` — runs `make web-build` first (needs **Node 20+**), then `make build`. Produces a binary with the full UI embedded.
- `docker compose build` / `docker compose up --build` — always rebuilds the UI as a node-alpine stage and embeds it. No host Node.js needed.
- `make web-dev` — starts Vite on port `5173` with `/v1/*` and `/api/*` proxied to `localhost:8080`. Use it together with `make run` for HMR-driven frontend work.

The UI is served by the same Go binary (no separate process or CDN). API paths (`/v1/*`, `/api/v1/*`) take precedence; unknown non-API paths fall back to `index.html` so SPA deep links survive page refreshes.

## Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/health` | none | Liveness + DB status. Returns 503 when DB is down. |
| `GET` | `/v1/me` | bearer | Caller profile (account, scope, org). |
| `GET` | `/v1/stats` | admin | Aggregate counts + recent push activity. |
| `POST` | `/v1/sync` | bearer | Bidirectional sync (push, tombstone, pull). |
| `POST` | `/api/v1/sync/push` | bearer | Compat push for the anchored CLI client. |
| `POST` | `/api/v1/sync/pull` | bearer | Compat pull for the anchored CLI client. |
| `GET` | `/v1/projects` | bearer | List projects the caller has access to. |
| `GET` | `/v1/projects/{id}` | bearer | Project detail (404 when soft-deleted). |
| `GET` | `/v1/projects/{id}/memories` | bearer | Paginated memories for the project. |
| `POST` | `/v1/projects` | admin | Create a project; auto-grants the creator via the org's default team. |
| `DELETE` | `/v1/projects/{id}` | admin | Soft-delete a project. |
| `GET` | `/v1/accounts` | admin | List organisation members. |
| `POST` | `/v1/accounts` | admin | Invite an account (idempotent on email). |
| `GET` | `/v1/teams` | bearer | List teams in the org. |
| `POST` | `/v1/teams` | admin | Create a team. |
| `GET` | `/v1/teams/{id}` | bearer | Team detail with members + project grants. |
| `POST` | `/v1/teams/{id}/members` | admin | Add a team member. |
| `DELETE` | `/v1/teams/{id}/members/{account_id}` | admin | Remove a team member. |
| `GET` | `/v1/api-keys` | admin | List all keys in the org. |
| `POST` | `/v1/api-keys` | admin | Mint a new API key (optional expiry 7d/30d/90d). |
| `DELETE` | `/v1/api-keys/{id}` | admin | Revoke a key. |
| `GET` | `/v1/audit` | admin | Audit entries with project/actor/action/date filters. |
| `GET` | `/v1/guardrails` | admin | List the org's sync-time guardrail rules. |
| `POST` | `/v1/guardrails` | admin | Add a custom guardrail (category block, keyword, or RE2 regex). |
| `PATCH` | `/v1/guardrails/{id}` | admin | Enable/disable or edit a guardrail (builtins toggle-only). |
| `DELETE` | `/v1/guardrails/{id}` | admin | Delete a custom guardrail (builtins cannot be deleted). |
| `GET` `PUT` | `/v1/policies` | admin | Scoring thresholds (quality, near-duplicate). |

API key scopes: `admin` (full + bypasses team-access checks), `sync` (push + pull), `readonly` (pull only).

## Why This Exists

AI coding tools currently rely on scattered project files like `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, giant docs, or long prompt context. That burns tokens, gets stale, and loses detail during compaction.

Anchored solves the local memory side. Anchored OSS solves the team side: a shared remote memory layer that keeps project knowledge available to every developer and every agent without pushing personal preferences or machine-local context into the team database.

## Product Model

```text
Account
└── Organization
    ├── Teams
    │   ├── Members
    │   └── Permissions
    ├── Guardrails (org-wide, admin-managed)
    ├── Audit log
    └── Projects
        ├── Shared memories
        └── Knowledge graph
```

- **Account**: human user.
- **Organization**: top-level ownership boundary for members, teams, projects, policies, and billing.
- **Team**: group of organization members with project access. Every org has an auto-managed `default` team; new projects grant write access to it so creators are immediately wired in.
- **Project**: organization-level shared memory scope. Projects may be created manually or automatically claimed from a non-personal repository identifier.

## Guardrails (privacy-first, configurable)

Remote sync is privacy-first. Each organization has a **guardrail set** — a list
of sync-time rules enforced per-item (rejections carry a `rule` in the response).
Admins manage it from the **Guardrails** screen; every org is seeded with a useful
default set that can be disabled, adjusted, or extended.

Seeded defaults:

- **Secret detection** — Stripe / GitHub / Slack tokens, AWS access keys, Google API keys, PEM private keys, credential-bearing URIs (`postgres://user:pass@`, `mongodb://...:...@`, `mysql://...`, `redis://:pass@`).
- **Local path block** — `/home/...`, `/Users/...`, `C:\Users\...`, `~/`, `/tmp/`, `/var/folders/`, `C:\Windows\`, ... Use repository-relative paths (e.g. `pkg/memory/service.go`).
- **User-scope block** — memories scoped to a single developer (personal, not team).
- **Category blocks** — `event` and `preference` are local-only by default.

Admins can additionally create **custom rules**: block extra categories, or reject
content matching a **keyword** (case-insensitive) or an **RE2 regex** (e.g. internal
codenames or ticket IDs). The default-accepted, team-shareable categories are
facts, decisions, learnings, plans, summaries, and knowledge-graph triples.

## Layout

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
├── docs/               # protocol + error reference
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Documentation

- [Architecture](ARCHITECTURE.md) — system overview, client↔server flow, onboarding, and feature map (with diagrams)
- [Sync Protocol](docs/sync-protocol.md) — bidirectional sync protocol specification
- [Error Codes](docs/error-codes.md) — API error codes and item-level rejection rules

## Contributing

Contributions are welcome. Anchored OSS is a Go server with an embedded React
dashboard.

1. **Build & test**

   ```bash
   make build          # Go-only build (CGO disabled; pure-Go SQLite + Postgres)
   go test ./...       # unit + store/integration tests
   go vet ./...
   cd web && npm run build   # type-check + bundle the dashboard
   ```

2. **Conventions**
   - English-first for code, comments, commits, and docs.
   - Conventional Commit messages (`feat:`, `fix:`, `docs:`, `chore:` ...), small and thematic.
   - Keep the two store backends in parity: any new query must be implemented for
     **both** Postgres (`internal/store/*.go`) and SQLite (`internal/store/sqlite_*.go`).
     SQLite returns `DATETIME` as strings — wrap timestamp scans with `scanTime`/`scanNullTime`.
   - Add tests for new behavior; preserve the privacy guardrails.

3. **Workflow** — open an issue to discuss non-trivial changes first, branch from
   `main`, ensure `go test ./...`, `go vet ./...`, and `npm run build` are green, then
   open a pull request describing the change and how you verified it.

See [ARCHITECTURE.md](ARCHITECTURE.md) for a map of the codebase before diving in.

## License

Anchored OSS is **source-available** under the **Functional Source License,
Version 1.1, with an Apache 2.0 future grant** ([`FSL-1.1-ALv2`](LICENSE)).

In short: you may read, run, self-host (including inside a company), modify, and
redistribute the Software for any purpose **except a Competing Use** — i.e. you
may not offer it to others as a commercial product or service that substitutes
for Anchored. Two years after each version is released, that version
automatically converts to the **Apache License 2.0**.

See [LICENSE](LICENSE) for the full terms. Learn more about the FSL at
<https://fsl.software>.
