# PRD — Anchored OSS

## Summary

Anchored OSS is the self-hosted team memory sync server for Anchored. It gives companies an internal shared memory layer for AI coding agents while preserving Anchored's local-first privacy model.

The product lets organizations share project-level knowledge — decisions, facts, learnings, plans, summaries, and knowledge graph relationships — across developers and tools. Personal memories, user preferences, local paths, embeddings, events, and behavioral metadata stay on each developer's machine unless explicitly allowed by policy.

## Problem

Teams using AI coding agents accumulate important project knowledge in fragmented places:

- `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and similar agent files
- long README/docs files that inflate prompt context
- individual local memories created by each developer
- chat transcripts that get compacted or lost
- tribal knowledge spread across people and tools

This creates three issues:

1. **Token waste** — agents repeatedly read large files for small facts.
2. **Knowledge loss** — compaction and session changes drop details.
3. **Team fragmentation** — one developer's AI memory is invisible to another developer.

Anchored already addresses local memory. Anchored OSS addresses shared team memory.

## Goals

- Provide a self-hosted remote memory server for teams.
- Keep local Anchored fully functional without server/cloud access.
- Sync only project-scoped, team-safe knowledge.
- Support accounts, organizations, teams, and organization-owned projects.
- Allow projects to be created manually or automatically from non-personal repository identifiers.
- Enforce server-side guardrails against local path leakage, secrets, and personal data.
- Use the same protocol foundation that Anchored Cloud can use later.

## Non-Goals for MVP

- Billing and subscription management.
- Hosted cloud dashboard.
- SSO/SAML/OIDC.
- Server-side embeddings by default.
- Full visual knowledge graph UI.
- Automatic destructive dream cleanup.
- Replacing local Anchored memory.

## Target Users

### Primary

Engineering teams already using Anchored locally and wanting shared project memory.

### Secondary

Companies evaluating AI coding agent governance and wanting self-hosted control over shared agent context.

### Future

Anchored Cloud users who prefer a managed hosted version of the same server.

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

### Account

A human user. In self-hosted mode, accounts can start as local records created by an admin/API key flow. Cloud can later add email login, magic link, GitHub OAuth, or SSO.

### Organization

Top-level ownership boundary. Organizations own teams, projects, policies, members, and audit logs.

### Team

A group of organization members. Teams grant read/write/maintain access to projects.

### Project

Organization-level shared memory scope. Projects are not owned by individual users. Projects may be manually created or automatically created when the client pushes a project claim using non-personal repository metadata.

## Core User Stories

### Organization Setup

- As an admin, I can create an organization.
- As an admin, I can create teams inside the organization.
- As an admin, I can invite/add members.
- As an admin, I can create API keys for members or projects.

### Project Setup

- As an admin, I can create a project manually.
- As a member, my client can request auto-creation of a project from a safe repository identity.
- As an admin, I can disable automatic project creation.
- As an admin, I can grant teams access to projects.

### Sync

- As a developer, I can push project decisions/facts/learnings from local Anchored to the organization project.
- As a developer, I can pull team-shared project memories into local Anchored.
- As a developer, I can keep working offline and sync later.
- As a developer, I can see whether a sync payload was accepted or rejected.

### Privacy and Guardrails

- As a developer, my local paths and personal preferences are not synced by default.
- As an admin, I can enforce policies that block sensitive categories or content patterns.
- As an admin, I can audit rejected remote writes.
- As an admin, I can ensure embeddings and behavioral metadata never leave clients.

### Audit

- As an admin, I can see who pushed, updated, or deleted shared memories.
- As an admin, I can inspect rejected writes and policy reasons.

## MVP Requirements

### Functional

1. HTTP server with `/v1/health`.
2. Postgres-backed schema and migrations.
3. Account, organization, team, project, team access, memory, API key, and audit log storage.
4. API key authentication.
5. Team/project authorization.
6. `POST /v1/sync` for bidirectional memory sync.
7. Tombstone propagation for soft deletes.
8. Last-write-wins conflict resolution using `updated_at`.
9. Server-side remote safety filter.
10. Server-side category policy enforcement.
11. Docker Compose self-hosted deployment.

### Non-Functional

- Default to safe failure: reject suspicious remote writes.
- Use Go stdlib where possible.
- Keep deployment simple: one server binary + Postgres.
- Keep protocol stable and versioned.
- Store no embeddings by default.
- Avoid persisting personal local paths.

## Privacy Requirements

Remote sync must never store:

- absolute local paths (`/home/user/...`, `/Users/user/...`, `C:\Users\user\...`)
- usernames or home directories
- local machine temp/cache/editor paths
- unscoped personal memories
- personal preferences unless explicitly allowed
- session events
- access counts or last-accessed timestamps
- embeddings
- secrets, tokens, private keys, connection strings

Remote sync may store:

- sanitized project facts
- sanitized decisions
- sanitized learnings
- plans and summaries scoped to a project
- project-relative file references
- knowledge graph triples
- author identity inside the organization
- audit records

## Success Metrics

- A self-hosted server can be launched locally with Docker Compose.
- Two simulated clients can sync project memories through the server.
- Remote payloads with absolute local paths are rejected.
- Remote payloads with secrets are rejected/redacted before persistence.
- Team authorization prevents unauthorized project access.
- All MVP behavior is covered by tests.

## Open Decisions

- Final license: AGPLv3 vs source-available no-managed-service license.
- Whether MVP should expose project/member management only by CLI/API or include a minimal web UI.
- Whether self-hosted MVP needs local account password login or API-key-only admin bootstrap.
- Whether knowledge graph sync ships in MVP or immediately after memory sync.
