# Anchored OSS

> Self-hosted team memory sync for Anchored — local-first, privacy-first, organization-owned knowledge for AI coding agents.

Anchored OSS is the open/self-hosted team layer for Anchored's local-first memory system. The local Anchored binary keeps personal memory, preferences, embeddings, and agent context on the developer's machine. Anchored OSS stores only organization/project-level knowledge that is safe to share with a team.

## Description

Anchored OSS is the self-hosted remote for Anchored. It lets companies run their own shared memory server so development teams can sync project facts, decisions, learnings, summaries, and knowledge graph relationships across Claude Code, Cursor, OpenCode, Gemini CLI, and other MCP-compatible tools.

It is designed as the open/self-hosted counterpart to the future Anchored Cloud service. Both should share the same protocol and privacy model: local-first clients, organization-owned projects, team-based access, and strict guardrails against leaking personal developer data.

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
- **Team**: group of organization members with project access.
- **Project**: organization-level shared memory scope. Projects may be created manually or automatically from non-personal repository identifiers.

## Privacy Rules

Remote sync is privacy-first. By default, Anchored OSS accepts only project-scoped team knowledge:

- facts
- decisions
- learnings
- plans
- summaries
- knowledge graph triples

Remote tools must not insert or sync:

- local filesystem paths (`/home/user/...`, `/Users/user/...`, `C:\Users\user\...`)
- usernames, home directories, or machine-local environment details
- unscoped personal memories
- personal preferences unless explicitly opted in
- session events, access patterns, or behavioral metadata
- embeddings
- secrets, credentials, tokens, private keys, connection strings

Remote references should use repository-relative paths, for example `pkg/memory/service.go`, never developer-local absolute paths.

## Planned Components

```text
anchored_oss/
├── cmd/server/              # HTTP server entrypoint
├── internal/api/            # REST handlers
├── internal/auth/           # accounts, API keys, invite flow
├── internal/store/          # Postgres store + migrations
├── internal/model/          # shared DTOs and domain types
├── internal/policy/         # guardrails and remote safety enforcement
├── internal/sync/           # bidirectional sync protocol
├── internal/dream/          # server-side dedup/contradiction jobs
├── docs/                    # architecture and implementation plans
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Initial Scope

The first implementation should stay intentionally small:

1. Postgres schema for accounts, organizations, teams, projects, project access, memories, and audit log.
2. API key auth.
3. `POST /v1/sync` bidirectional JSON protocol.
4. Server-side guardrails for remote-safe content.
5. Docker Compose for self-hosted deployment.

Cloud-only capabilities such as billing, hosted dashboard, and scheduled dream jobs can layer on top of the same server later.

## Documentation

- [PRD](docs/prd.md) — product requirements and MVP boundaries
- [Architecture](docs/architecture.md) — system design, schema, API surface
- [Execution Plan](docs/execution-plan.md) — implementation phases and acceptance criteria
- [Implementation Plan](docs/implementation-plan.md) — compact phased checklist
- [Session Handoff](docs/session-handoff.md) — context for continuing in a new session

## License Direction

Companies should be able to run Anchored OSS internally for their own teams. Third parties should not be able to repackage the project and sell a competing hosted Anchored service.

The license decision is still open:

- **AGPLv3** if OSI open source is more important.
- **Source-available no-managed-service license** if explicitly blocking resale/hosted competitors is more important.
