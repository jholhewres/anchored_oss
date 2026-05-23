# Execution Plan — Anchored OSS

This plan is designed for implementation in this repository.

## Implementation Principles

- Keep every phase independently testable.
- Prefer small commits by subsystem: skeleton, config, store, auth, policy, sync, packaging.
- Do not implement cloud billing/dashboard before the self-hosted sync path works.
- Treat server-side privacy guardrails as a core feature, not an add-on.

---

## Phase 0 — Repository Baseline

### Deliverables

- `go.mod`
- `cmd/server/main.go`
- `internal/config`
- `internal/version`
- `Makefile`
- basic `README` commands

### Acceptance

- `go test ./...` passes.
- `go run ./cmd/server --help` works.
- No external services required.

### Suggested Commit

`chore: scaffold anchored oss server`

---

## Phase 1 — HTTP Server and Health

### Deliverables

- HTTP server using Go stdlib `net/http`.
- `/v1/health` endpoint.
- request logging middleware.
- JSON response helpers.
- graceful shutdown.

### Acceptance

- `GET /v1/health` returns service name, version, and status.
- server exits cleanly on context cancellation/signals.
- tests cover health endpoint.

### Suggested Commit

`feat: add health endpoint`

---

## Phase 2 — Postgres Migrations

### Deliverables

- migration runner.
- SQL migrations for:
  - accounts
  - organizations
  - teams
  - org_members
  - team_members
  - projects
  - team_project_access
  - api_keys
  - memories
  - audit_log
  - policies
- Docker Compose with Postgres.

### Acceptance

- migrations run idempotently.
- tests can create a temporary database or run against a test DSN.
- schema matches `docs/architecture.md` or documents intentional differences.

### Suggested Commit

`feat: add postgres schema migrations`

---

## Phase 3 — Store Layer

### Deliverables

- domain models under `internal/model`.
- `internal/store` interfaces.
- Postgres implementation for:
  - organizations
  - accounts
  - teams
  - projects
  - memories
  - audit logs

### Acceptance

- create/list/get tests for each core entity.
- memory upsert is idempotent by `(project_id, content_hash)`.
- soft delete/tombstone behavior is tested.

### Suggested Commit

`feat: add postgres store layer`

---

## Phase 4 — API Keys and Auth

### Deliverables

- API key model with hash + prefix.
- key generation command or admin endpoint.
- middleware to authenticate `Authorization: Bearer`.
- scopes:
  - `admin`
  - `sync`
  - `readonly`

### Acceptance

- plaintext API keys are never stored.
- invalid/revoked keys fail.
- readonly keys cannot push writes.
- sync keys can access only authorized projects.

### Suggested Commit

`feat: add api key authentication`

---

## Phase 5 — Authorization

### Deliverables

- org role checks: owner/admin/member.
- team project access checks: reader/writer/maintainer.
- helper methods for checking project read/write access.

### Acceptance

- reader can pull but cannot push.
- writer can push and pull.
- maintainer can delete/review/manage project policy.
- unrelated team cannot access project.

### Suggested Commit

`feat: enforce team project access`

---

## Phase 6 — Remote Safety Filter

### Deliverables

- `internal/policy` remote safety filter.
- detection for:
  - Linux/macOS/Windows absolute user paths
  - home-relative paths
  - local temp/cache/editor paths
  - common secret/token patterns
  - blocked categories
- structured rejection reasons.

### Acceptance

- `/home/alice/...`, `/Users/bob/...`, and `C:\Users\bob\...` are rejected.
- project-relative paths are accepted.
- preferences are rejected unless project policy allows them.
- tests cover each rejection reason.

### Suggested Commit

`feat: add remote safety guardrails`

---

## Phase 7 — Sync Protocol MVP

### Deliverables

- `POST /v1/sync` endpoint.
- request/response DTOs.
- push memory handling.
- pull memory handling by watermark.
- tombstone handling.
- accepted/rejected item results.
- audit log writes.

### Acceptance

- two test clients can exchange memories.
- rejected items do not persist.
- accepted items are audited.
- tombstones propagate.
- repeated sync is idempotent.

### Suggested Commit

`feat: implement sync endpoint`

---

## Phase 8 — Project Auto-Creation

### Deliverables

- `project_claim` support in sync request.
- remote key validation.
- org policy for allowing/disallowing auto-create.
- default team access assignment.

### Acceptance

- project claim with git remote hash can create project.
- project claim with local path is rejected.
- auto-create disabled returns clear error.

### Suggested Commit

`feat: support safe project auto creation`

---

## Phase 9 — Self-Hosted Packaging

### Deliverables

- Dockerfile.
- docker-compose.yml.
- `.env.example`.
- setup docs.
- admin bootstrap command.

### Acceptance

- clean checkout can run self-hosted server with one compose command.
- admin can create org/project/API key.
- local smoke test can call `/v1/health` and `/v1/sync`.

### Suggested Commit

`build: add self-hosted packaging`

---

## Phase 10 — Client Integration Contract

This phase mainly coordinates with the existing `anchored` repo.

### Deliverables

- protocol examples.
- sync fixtures.
- compatibility tests or golden JSON samples.
- documented error codes.

### Acceptance

- Anchored local client can use fixtures to implement `pkg/sync` without ambiguity.
- API rejects unsafe payloads deterministically.

### Suggested Commit

`docs: define client sync contract`

---

## First Implementation Session Recommendation

Start with phases 0 and 1 only:

1. Go module.
2. `cmd/server`.
3. config loading.
4. `/v1/health`.
5. tests.

Do not start Postgres/schema until the server skeleton is clean.
