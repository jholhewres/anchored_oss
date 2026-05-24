# Anchored OSS Memory Lifecycle Sync Roadmap

## Purpose

Anchored OSS is the private Postgres-backed shared memory layer for teams and
organizations using Anchored. It should not replace the local Anchored runtime.
The local binary remains responsible for hot-path context retrieval, embeddings,
token budgeting, local-only memories, and offline operation.

This roadmap defines how Anchored OSS should evolve to preserve and enforce the
same advanced memory lifecycle model as Anchored local while remaining backward
compatible with existing clients and sync payloads.

## Role split

```text
Anchored local
  - complete individual/project memory runtime
  - local SQLite source of truth for the developer
  - hybrid retrieval and context optimization hot path
  - local-only capture, handoff, precompact, bootstrap, dream, retention
  - remote sync filtering and preview

Anchored OSS
  - shared project/team memory authority
  - Postgres source of truth for organization-owned project knowledge
  - org/account/team/project access control
  - sync watermarks, audit, policies, quota, dashboard/API search
  - stores only safe project/team knowledge
```

The server should be optimized for durable shared knowledge, access control, and
replication. It should not be called on every agent prompt.

## Current baseline

Anchored OSS already has:

- Postgres-backed `memories` with `metadata JSONB`.
- Organizations, accounts, teams, projects, API keys, audit log.
- API key scopes: `admin`, `sync`, `readonly`.
- Bidirectional `/v1/sync` protocol with watermark and tombstones.
- Compat `/api/v1/sync/push` and `/api/v1/sync/pull` endpoints.
- Server-side content policy for local paths, secrets, and blocked categories.
- Project claims via stable `remote_key`.
- Dashboard and paginated project memories.

The current protocol stores category/content/keywords/source/author plus optional
metadata on server-side memories, but the canonical sync input does not yet
round-trip lifecycle metadata from local clients.

## Memory lifecycle contract v2

The remote should preserve lifecycle metadata produced by Anchored local. The
initial implementation should carry these values inside `metadata JSONB` to
avoid breaking protocol compatibility.

Canonical metadata shape:

```json
{
  "memory_type": "semantic",
  "kind": "decision",
  "scope": "project",
  "origin": "bootstrap",
  "importance": 0.8,
  "pinned": false,
  "expires_at": null,
  "supersedes": ["old-memory-id"],
  "consolidates": ["memory-a", "memory-b"],
  "context_tier": "L1",
  "confidence": 0.9
}
```

### Server interpretation

| Field | Server responsibility |
|---|---|
| `memory_type` | Enforce shareability policy. |
| `kind` | Allowlist special operational kinds such as project handoff. |
| `scope` | Reject user-scoped data; allow project/team. |
| `origin` | Reject local-only operational origins such as precompact. |
| `importance` | Store and expose for dashboard/API ordering later. |
| `pinned` | Preserve; do not imply server-side retention yet. |
| `expires_at` | Preserve; optional future archival behavior. |
| `supersedes` / `consolidates` | Preserve lineage. |
| `context_tier` | Preserve local ranking hint; server does not rely on it initially. |
| `confidence` | Preserve bootstrap/import confidence. |

## Compatibility strategy

- Keep existing `/v1/sync` fields valid.
- Add optional `metadata` to pushed `SyncMemory`; do not require it.
- Keep compat push/pull endpoints working for older local clients.
- When metadata is missing, infer safe defaults from `category` exactly as today.
- Preserve unknown metadata keys when reading/writing.
- Avoid promoting lifecycle fields to required columns until the protocol is
  stable.
- Add indexes on JSONB expressions only after the metadata exists in production.

## Feature plan

### Phase 1 — Protocol metadata round-trip

Changes:

- Add optional `Metadata any` or `map[string]any` to `model.SyncMemory`.
- Store pushed metadata into `model.Memory.Metadata` during sync.
- Include metadata in `/v1/sync` pull items.
- Update docs in `docs/sync-protocol.md`.
- Add tests for:
  - old request without metadata
  - new request with lifecycle metadata
  - unknown metadata preservation
  - nil/empty metadata behavior

Compat endpoints:

- `/api/v1/sync/push` currently receives the simpler local client shape. Keep it
  working.
- Add optional metadata to the compat structs only when the local client starts
  sending it.
- Do not reject older compat clients that omit metadata.

Acceptance criteria:

- Existing clients can push/pull unchanged.
- New clients can push lifecycle metadata and pull it back unchanged.
- Metadata does not bypass existing content/category policies.

### Phase 2 — Policy v2 for lifecycle metadata

Extend `internal/policy` to validate metadata in addition to content/category.

Initial rules:

| Condition | Result |
|---|---|
| `scope=user` | reject |
| `memory_type=episodic` | reject by default |
| `kind=precompact` | reject |
| `origin=precompact` | reject |
| `category=event` | reject, unchanged |
| `category=preference` with personal scope | reject, unchanged |
| `memory_type=semantic` + `scope=project|team` | allow if content passes filters |
| `origin=bootstrap` | allow if semantic and safe |
| `origin=dream` | allow if semantic and safe |
| `kind=handoff` | allow only if explicitly project/team-scoped |

Implementation notes:

- Return item-level sync rejection rules, not whole-request failures, whenever
  possible.
- Suggested new rejection rules:
  - `user_scope_blocked`
  - `episodic_blocked`
  - `operational_kind_blocked`
  - `precompact_blocked`
  - `invalid_lifecycle_metadata`
- Keep existing rule names stable for existing policy failures.

Acceptance criteria:

- Server rejects personal or local-only lifecycle metadata even if local preview
  misses it.
- Server still accepts old safe project memories without metadata.
- Audit records lifecycle rejection reasons.

### Phase 3 — Postgres metadata indexes

The `memories.metadata JSONB` field allows a compatibility-first rollout. Add
indexes only after metadata is being stored.

Suggested migration:

```sql
CREATE INDEX IF NOT EXISTS idx_memories_metadata_scope
ON memories ((metadata->>'scope'))
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memories_metadata_memory_type
ON memories ((metadata->>'memory_type'))
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memories_metadata_kind
ON memories ((metadata->>'kind'))
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memories_project_category_updated
ON memories(project_id, category, updated_at DESC)
WHERE deleted_at IS NULL;
```

Compatibility:

- These indexes do not change data shape.
- They are safe for rows with null metadata.
- Do not add NOT NULL constraints for lifecycle fields.

### Phase 4 — Project/team handoffs

Support project-safe handoffs as optional shared operational memories.

Allowed remote shape:

```json
{
  "category": "summary",
  "metadata": {
    "memory_type": "operational",
    "kind": "handoff",
    "scope": "project",
    "origin": "handoff",
    "expires_at": "..."
  }
}
```

Policy:

- Reject user-scoped handoffs.
- Reject handoffs with local paths/secrets.
- Allow only `scope=project|team`.
- Store `expires_at`; do not hard-delete automatically in the first version.

Dashboard/API:

- Later, expose recent project handoffs separately from semantic knowledge.
- Keep normal sync/pull behavior first.

### Phase 5 — Supersession and consolidation lineage

Preserve lineage metadata from local dream/consolidation.

Initial approach:

- Store `supersedes` and `consolidates` arrays in `metadata JSONB`.
- Do not create a separate table yet.
- Update dashboard/API detail views later to display lineage.

Future robust model:

```sql
CREATE TABLE memory_edges (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  from_memory_id TEXT NOT NULL,
  to_memory_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

Compatibility:

- Existing memories have no lineage and remain valid.
- Superseded memories should not be automatically deleted remotely.
- Local clients decide ranking/demotion after pulling lineage.

### Phase 6 — Better server search with Postgres FTS

Current `SearchMemories` uses `ILIKE`, which is acceptable for small data but
will degrade as shared project memory grows.

Add Postgres full-text search for dashboard/API search, while keeping local
Anchored as the hot retrieval path.

Suggested migration:

```sql
ALTER TABLE memories
ADD COLUMN IF NOT EXISTS search_vector tsvector
GENERATED ALWAYS AS (
  to_tsvector('simple', coalesce(content, '') || ' ' || coalesce(array_to_string(keywords, ' '), ''))
) STORED;

CREATE INDEX IF NOT EXISTS idx_memories_search_vector
ON memories USING GIN(search_vector);
```

Query shape:

```sql
WHERE project_id = $1
  AND deleted_at IS NULL
  AND search_vector @@ plainto_tsquery('simple', $2)
ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $2)) DESC,
         updated_at DESC
LIMIT $3
```

Compatibility:

- Preserve `/v1/memories/search?q=` response shape.
- Fall back to `ILIKE` when FTS query is empty or migration is unavailable.
- Do not add vector search to the server until there is a clear dashboard/API
  need. Local vector search remains the primary quality layer.

### Phase 7 — Remote knowledge graph

The README already positions projects as containing shared memories and
knowledge graph. Implement after metadata v2 and FTS.

Proposed schema:

```sql
CREATE TABLE kg_edges (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  subject TEXT NOT NULL,
  predicate TEXT NOT NULL,
  object TEXT NOT NULL,
  source_memory_id TEXT,
  confidence REAL DEFAULT 1.0,
  created_at TIMESTAMPTZ DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
```

Sync options:

1. Extend `/v1/sync` with optional `kg_pushes`, `kg_tombstones`, `kg_pulls`.
2. Or add separate KG endpoints after memory sync stabilizes.

Recommendation:

- Keep KG out of the first lifecycle milestone.
- First preserve memory metadata/lineage so local clients can continue indexing
  the local KG after pull.

### Phase 8 — Audit and dashboard observability

Add lifecycle-aware observability.

Audit metadata should include:

- `memory_type`
- `kind`
- `scope`
- rejection rule
- client_id
- accepted/rejected counts by category/type

Dashboard additions:

- Semantic vs operational counts.
- Recent project/team handoffs.
- Rejected syncs by rule.
- Memories by origin (`manual`, `bootstrap`, `dream`, `remote`).
- Stale or expired operational memories.
- Top contributors by project.

Compatibility:

- Existing audit entries remain valid.
- New dashboard cards are additive.

## Retention model for the server

Anchored OSS should be conservative. Local retention may compact or expire data
aggressively; remote should preserve shared project knowledge unless a server
policy says otherwise.

Default remote behavior:

| Type | Remote behavior |
|---|---|
| `semantic` | preserve |
| `episodic` | reject by default |
| `operational` | allowlist only, usually handoff |
| `pinned` | preserve flag, no automatic behavior initially |
| `expires_at` | store and optionally hide expired operational items later |

No automatic hard deletes in the initial lifecycle rollout.

## Coordinated rollout with Anchored local

### Milestone 1 — Compatibility-first metadata

Anchored local:

- Adds metadata v2 parser/writer.
- Sync preview uses lifecycle rules.
- Existing compat sync still works.

Anchored OSS:

- Accepts and preserves optional sync metadata.
- Enforces server-side metadata policy.
- Docs and tests updated.

### Milestone 2 — Operational continuity

Anchored local:

- Handoff and PreCompact snapshots.
- Context optimizer prioritizes recent operational context.

Anchored OSS:

- Optionally allows project/team handoffs.
- Dashboard can later show shared handoffs.

### Milestone 3 — Project seeding and shared knowledge

Anchored local:

- Bootstrap creates semantic project memories.
- Sync preview can push project-safe bootstrap output.

Anchored OSS:

- Stores bootstrap-origin metadata.
- Adds FTS for project memory search.

### Milestone 4 — Consolidation lineage

Anchored local:

- Dream emits supersession/consolidation metadata.

Anchored OSS:

- Preserves lineage metadata.
- Adds lineage APIs/UI later if needed.

## Backward compatibility checklist

- Old clients can continue using `/api/v1/sync/push` and `/api/v1/sync/pull`.
- Old `/v1/sync` requests without metadata remain valid.
- New lifecycle metadata is optional.
- Unknown metadata keys are preserved.
- Existing blocked categories and safety filters remain enforced.
- No existing API response removes fields.
- Any new response fields are additive.
- Migrations must be additive and safe for null metadata.
- Server policy rejects unsafe new metadata but does not reject old safe memories
  simply because lifecycle fields are absent.

## Suggested implementation order

1. Add `metadata` to `model.SyncMemory` and round-trip tests.
2. Store metadata in `handlePushes` and compat adapters when supplied.
3. Add lifecycle metadata policy validation.
4. Update sync protocol docs and error-code docs.
5. Add JSONB expression indexes.
6. Add project/team handoff allowlist.
7. Replace `ILIKE` search with Postgres FTS.
8. Preserve lineage from local dream.
9. Add dashboard lifecycle summaries.
10. Add remote KG only after memory lifecycle sync is stable.
