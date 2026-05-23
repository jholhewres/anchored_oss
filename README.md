# Anchored OSS

> Self-hosted team memory sync for Anchored — local-first, privacy-first, organization-owned knowledge for AI coding agents.

Anchored OSS is the open/self-hosted team layer for Anchored's local-first memory system. The local Anchored binary keeps personal memory, preferences, embeddings, and agent context on the developer's machine. Anchored OSS stores only organization/project-level knowledge that is safe to share with a team.

## Description

Anchored OSS is the self-hosted remote for Anchored. It lets companies run their own shared memory server so development teams can sync project facts, decisions, learnings, summaries, and knowledge graph relationships across Claude Code, Cursor, OpenCode, Gemini CLI, and other MCP-compatible tools.

It is designed as the open/self-hosted counterpart to the future Anchored Cloud service. Both should share the same protocol and privacy model: local-first clients, organization-owned projects, team-based access, and strict guardrails against leaking personal developer data.

## Quick Start

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

The server ships with a React + shadcn/ui admin dashboard embedded directly into the binary. Open `http://localhost:8080` after bootstrap, paste an admin API key, and you have access to Overview, Projects, Accounts, Teams, API keys, Audit, and Health screens.

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
    └── Projects
        ├── Shared memories
        ├── Knowledge graph
        ├── Policies / guardrails
        └── Audit log
```

- **Account**: human user.
- **Organization**: top-level ownership boundary for members, teams, projects, policies, and billing.
- **Team**: group of organization members with project access. Every org has an auto-managed `default` team; new projects grant write access to it so creators are immediately wired in.
- **Project**: organization-level shared memory scope. Projects may be created manually or automatically claimed from a non-personal repository identifier.

## Privacy Rules

Remote sync is privacy-first. By default, Anchored OSS accepts only project-scoped team knowledge:

- facts
- decisions
- learnings
- plans
- summaries
- knowledge graph triples

The server rejects (per-item, with a `rule` in the response) anything that looks like:

- local filesystem paths (`/home/...`, `/Users/...`, `C:\Users\...`, `~/`, `/tmp/`, `/var/folders/`, `C:\Windows\`, ...)
- secrets: Stripe / GitHub / Slack tokens, AWS access keys, Google API keys, PEM private keys, and credential-bearing URIs (`postgres://user:pass@`, `mongodb://...:...@`, `mysql://...`, `redis://:pass@`)
- categories `event` and `preference` (local-only by design)

Remote references should use repository-relative paths, e.g. `pkg/memory/service.go`, never developer-local absolute paths.

## Layout

```text
anchored_oss/
├── cmd/server/         # HTTP server entrypoint + bootstrap
├── internal/auth/      # API key generation and hashing
├── internal/config/    # YAML + env config loader
├── internal/handler/   # REST handlers (health, sync, projects, api-keys)
├── internal/middleware/# auth, CORS, body limit, logging, recovery
├── internal/model/     # shared DTOs and domain types
├── internal/policy/    # guardrails (local paths, secrets, blocked categories)
├── internal/server/    # http.Server wiring
├── internal/store/     # Postgres store + migrations
├── internal/sync/      # bidirectional sync engine
├── internal/version/   # build-time version
├── docs/               # protocol + error reference
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Documentation

- [Sync Protocol](docs/sync-protocol.md) — bidirectional sync protocol specification
- [Error Codes](docs/error-codes.md) — API error codes and item-level rejection rules

## License

License is still being decided between **AGPLv3** (broad OSI compatibility) and a **source-available no-managed-service license** (block hosted resellers). Until decided, Anchored OSS is shared under "all rights reserved" — companies may run it internally; redistribution requires written permission.
