# Session Handoff — Anchored OSS

Use this file to start a new implementation session in this repository.

## Current State

Repository path:

```text
/home/jhol/Workspace/private/anchored_oss
```

Current files:

```text
.gitignore
README.md
docs/architecture.md
docs/implementation-plan.md
docs/prd.md
docs/execution-plan.md
docs/session-handoff.md
```

No implementation code exists yet.

## Product Decision Summary

- Repo name: `anchored_oss`.
- Purpose: self-hosted/open team sync distribution for Anchored.
- Future Anchored Cloud should use the same core protocol/model.
- Hierarchy: Account → Organization → Teams → Projects.
- Projects belong to organizations, not users.
- Teams grant access to projects.
- Projects may be created manually or auto-created from safe repository identifiers.
- Remote payloads must not include personal paths, usernames, home directories, machine-local details, preferences by default, events, embeddings, or secrets.

## Docs to Read First

1. `docs/prd.md`
2. `docs/architecture.md`
3. `docs/execution-plan.md`
4. `README.md`

## First Implementation Goal

Implement **Phase 0 + Phase 1** from `docs/execution-plan.md`:

- Go module.
- `cmd/server/main.go`.
- config package.
- version package.
- HTTP server using `net/http`.
- `/v1/health` endpoint.
- graceful shutdown.
- tests for health endpoint.
- Makefile with build/test/run targets.

## Constraints

- Keep dependencies minimal.
- Prefer Go stdlib.
- Do not add cloud billing/dashboard yet.
- Do not add Postgres until the server skeleton is clean.
- Do not implement unsafe remote writes.
- Do not store embeddings server-side by default.

## Suggested First Commit

```text
chore: scaffold anchored oss server
```

Then:

```text
feat: add health endpoint
```

## Validation for First Session

```bash
go test ./...
go run ./cmd/server --help
go run ./cmd/server
curl http://localhost:8080/v1/health
```

## Related Source Project

Local Anchored repo:

```text
/home/jhol/Workspace/private/anchored
```

Relevant docs there:

```text
docs/team-sync.md
docs/improvements-roadmap.md
```
