# Anchored OSS Architecture

Anchored OSS provides organization-scoped remote memory sync for Anchored.

It is not a replacement for the local Anchored binary. The local binary remains the source of personal memory, local embeddings, offline behavior, and AI tool integration. Anchored OSS is a shared remote for project knowledge.

---

## Core Invariants

1. **Local-first**: local Anchored must work without this server.
2. **Organization-owned projects**: remote projects belong to an organization, not an individual developer.
3. **Teams grant access**: teams mediate read/write access to projects.
4. **No personal path leakage**: remote payloads must never contain developer-local paths or usernames.
5. **No embeddings on server by default**: clients embed pulled memories locally.
6. **Server-side policy is authoritative**: the client filters for UX, but the server enforces guardrails.

---

## Data Model

```sql
CREATE TABLE accounts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email        TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    slug       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, slug)
);

CREATE TABLE org_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, account_id)
);

CREATE TABLE team_members (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (team_id, account_id)
);

CREATE TABLE projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    remote_key TEXT NOT NULL,
    created_by UUID REFERENCES accounts(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, slug),
    UNIQUE(org_id, remote_key)
);

CREATE TABLE team_project_access (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'writer',
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (team_id, project_id)
);

CREATE TABLE memories (
    id           TEXT PRIMARY KEY,
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category     TEXT NOT NULL,
    content      TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    keywords     TEXT[],
    source       TEXT,
    author_id    UUID REFERENCES accounts(id),
    author_name  TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    deleted_at   TIMESTAMPTZ,
    metadata     JSONB
);

CREATE INDEX idx_memories_project_updated ON memories(project_id, updated_at);
CREATE UNIQUE INDEX idx_memories_content_hash_project
    ON memories(content_hash, project_id)
    WHERE deleted_at IS NULL;

CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id  UUID REFERENCES projects(id) ON DELETE CASCADE,
    actor_id    UUID REFERENCES accounts(id),
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ DEFAULT now()
);
```

---

## Sync Protocol

Single bidirectional endpoint:

```http
POST /v1/sync
Authorization: Bearer <api-key>
Content-Type: application/json
```

The request contains:

- organization/project identity or project claim
- client ID
- last server watermark
- local changes to push
- tombstones
- optional knowledge graph triples

The response contains:

- remote memories to pull
- server tombstones
- accepted/rejected push results
- new watermark

Conflict resolution starts simple: last-write-wins by `updated_at`, with tombstones for soft deletes.

---

## Automatic Project Creation

Clients may request project creation when pushing from an unknown repository, but only using non-personal identifiers:

```json
{
  "project_claim": {
    "name": "anchored",
    "remote_key": "git:sha256:...",
    "git_host": "github.com",
    "repo_slug": "jholhewres/anchored"
  }
}
```

The server must reject claims containing local paths, home directories, or usernames.

---

## API Surface

Initial endpoints:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/sync` | Push/pull memories |
| `GET` | `/v1/health` | Version and health |
| `POST` | `/v1/projects` | Create project |
| `GET` | `/v1/projects` | List accessible projects |
| `POST` | `/v1/api-keys` | Create API key |
| `DELETE` | `/v1/api-keys/{id}` | Revoke API key |
| `GET` | `/v1/audit` | Query audit log |

Dashboard and billing endpoints are later layers, not MVP requirements.
