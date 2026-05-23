# Implementation Plan

Implementation should proceed in small, testable phases.

---

## Phase 1 — Repository Skeleton

- Go module
- CLI entrypoint at `cmd/server`
- Config loading
- Health endpoint
- Docker Compose with Postgres

Acceptance:

- `go test ./...` passes
- server starts locally
- `/v1/health` returns version/status

---

## Phase 2 — Storage Foundation

- Postgres migrations
- Store interfaces
- Project/account/team schema
- Audit log append API

Acceptance:

- migrations are idempotent
- store tests cover create/list/get flows

---

## Phase 3 — Auth and Access

- API key generation and hashing
- account/org/team/project access checks
- scoped keys: `sync`, `admin`, `readonly`

Acceptance:

- invalid/revoked keys fail
- readonly keys cannot push
- team access controls project visibility

---

## Phase 4 — Sync MVP

- `POST /v1/sync`
- push/pull memories
- watermark handling
- tombstones
- last-write-wins conflict behavior

Acceptance:

- two clients can exchange project memories through the server
- duplicate content hashes are idempotent
- soft deletes propagate

---

## Phase 5 — Guardrails

- remote-safe content filter
- path/personal-data blocking
- category policy enforcement
- server-side sanitizer

Acceptance:

- absolute local paths are rejected or redacted
- preferences are blocked unless explicitly allowed
- secrets are never persisted in shared memory

---

## Phase 6 — Self-Hosted Packaging

- Dockerfile
- docker-compose.yml
- Makefile
- README setup flow

Acceptance:

- a team can run the server with one compose command
- API key can be generated for a user/project

---

## Later

- dashboard
- billing
- cloud auth
- scheduled dream jobs
- graph viewer
- policy editor
