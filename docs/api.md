# API Reference

Anchored OSS exposes a single JSON-over-HTTP API. Every endpoint returns
`application/json`, and every state-changing call is authenticated with a
bearer API token. This document is generated from the live route table in
`internal/server/server.go` and the request/response structs in
`internal/handler/*` and `internal/model/*`.

## Overview

- **Base URL** — the server listens on the address in your config
  (`server.address`, default `:8080`; the `PORT` env var overrides it). The
  examples below use `https://anchored.example.com` for a deployed instance and
  `http://localhost:8771` for local development. Substitute your own host.
- **Content type** — send `Content-Type: application/json` on any request with
  a body. Responses are always JSON.
- **Versioning** — all application routes are under the `/v1` prefix. Three
  compatibility routes for the CLI sync client live under `/api/v1` (see
  [Sync](#sync)). There is no separate version-negotiation header; breaking
  changes would land under a new path prefix.
- **Time format** — all timestamps are RFC 3339 / ISO 8601 in UTC
  (e.g. `2026-07-20T12:00:00Z`).

The curl examples assume a token in the `ANCHORED_TOKEN` environment variable
and a base URL in `ANCHORED_URL`:

```bash
export ANCHORED_URL="https://anchored.example.com"
export ANCHORED_TOKEN="anc_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

---

## Authentication

Anchored uses **bearer tokens only** — there are no cookies or sessions in the
API. (The dashboard "session" is just an auto-minted `admin`-scope token that
the browser stores; it flows through the exact same header.) Every
authenticated request carries:

```http
Authorization: Bearer anc_live_<64 hex characters>
```

A token is the literal prefix `anc_live_` followed by 64 hex characters
(32 random bytes). Tokens are **SHA-256 hashed at rest** — the plaintext is
shown exactly once at creation and can never be recovered afterward. Validation
happens in `internal/middleware/auth.go`: the presented token is hashed and
looked up; a missing, malformed, unknown, revoked, or expired token yields:

```json
{"error": "UNAUTHORIZED"}
```

with HTTP `401`.

### Obtaining a personal token

**The API endpoint for minting tokens (`POST /v1/api-keys`) is admin-only.** As
a regular user you do **not** call it. Instead:

1. Sign in to the dashboard (`$ANCHORED_URL`).
2. Open the **API Keys** page.
3. Create a key, choose a scope and expiry, and copy the plaintext token
   **immediately** — it is displayed only once.

Use that token as `$ANCHORED_TOKEN` for every call below. If you are an org
admin automating key creation, see
[`POST /v1/api-keys`](#post-v1api-keys-admin) in the Admin section.

### Scopes

Every token carries exactly one scope. `admin` bypasses all scope and
team-access checks; `readonly` is rejected on any write.

| Scope | Read | Write (push, save, triples) | Admin routes | Notes |
|---|---|---|---|---|
| `admin` | yes | yes | yes | Full access; bypasses team-access and scope checks. |
| `sync` | yes | yes | no | Read + write within projects the account can reach. |
| `readonly` | yes | no | no | Pull/search/list only; writes return `403 FORBIDDEN`. |

### Expiry

At mint time a key is given one of four expiry windows: **never** (empty),
**7d**, **30d**, or **90d**. An expired key is treated exactly like an invalid
one (`401 UNAUTHORIZED`).

---

## Multi-tenancy & scoping

Anchored is organized as **organization → team → project**. Everything is
derived from the token:

- The **organization is always taken from the token**, never from a request
  body or query parameter. A token can only ever act within its own org.
- Targeting a project that belongs to a **different org** returns `403`
  (`project belongs to a different organization`).
- **Team access**: a non-admin account sees and can act on only the projects
  granted to a team it belongs to. Requests for other projects return `403`
  (`no team access to this project`).
- **`admin` scope bypasses** team-access checks entirely and can reach every
  project in its org.

Practically: `readonly`/`sync` tokens are scoped to the calling account's team
grants; `admin` tokens see the whole org.

---

## Endpoint reference

Auth column legend: **none** = unauthenticated, **bearer** = any valid token,
**admin** = `admin` scope required, **write** = any non-`readonly` token.

### Health

#### `GET /v1/health` — none

Liveness plus database status. Returns `503` when the DB ping fails.

```bash
curl -s "$ANCHORED_URL/v1/health"
```

```json
{
  "service": "anchored-oss",
  "version": "v0.5.8",
  "status": "ok",
  "db_status": "ok",
  "timestamp": "2026-07-20T12:00:00Z"
}
```

On DB failure: HTTP `503`, `"status": "degraded"`, `"db_status": "down"`.

---

### Me

#### `GET /v1/me` — bearer

Returns the caller's identity. **This should be the first call any token client
makes** — it confirms the token is valid and reveals the account, org, and
scope the token acts as.

```bash
curl -s "$ANCHORED_URL/v1/me" \
  -H "Authorization: Bearer $ANCHORED_TOKEN"
```

```json
{
  "account_id": "0c8f...",
  "org_id": "3a71...",
  "org_slug": "acme",
  "scope": "sync",
  "email": "dev@acme.com",
  "display_name": "Dev One"
}
```

`org_slug`, `email`, and `display_name` are omitted if unavailable.

---

### Memories

Individual memories are created here and read via project listing
([`GET /v1/projects/{id}/memories`](#get-v1projectsidmemories--bearer)) or
[search](#get-v1memoriessearch--bearer). **There is no `GET /v1/memories/{id}`
endpoint** — fetch a single memory by listing or searching its project.

Allowed `category` values for a saved memory are **`fact`, `decision`, `plan`,
`summary`, `learning`**. The categories `event` and `preference` are explicitly
rejected (they are local-only and must not reach team memory).

#### `POST /v1/memories` — write

Create (upsert) a single memory. `readonly` tokens get `403`.

Request body:

| Field | Type | Required | Notes |
|---|---|---|---|
| `project_id` | string | one of these | Target project UUID. |
| `project_claim` | object | one of these | `{name, remote_key, git_host?, repo_slug?}` — resolves/creates a project by git-origin remote key when `project_id` is absent. |
| `id` | string | no | Client-supplied ID; server generates one if omitted. Reusing an ID that belongs to another project returns `409`. |
| `category` | string | yes | `fact` \| `decision` \| `plan` \| `summary` \| `learning`. |
| `content` | string | yes | The memory text. Runs through the content filter (secret/local-path rejection). |
| `keywords` | string[] | no | Optional tags. |
| `source` | string | no | Free-form provenance label. |

Optional header **`Idempotency-Key`** (≤128 safe-ASCII chars): a replay with the
same key + identical payload returns the original result and adds the header
`Idempotency-Replayed: true`. Reusing a key with a **different** payload returns
`409` `idempotency_conflict`.

```bash
curl -s -X POST "$ANCHORED_URL/v1/memories" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 2026-07-20-import-001" \
  -d '{
    "project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
    "category": "decision",
    "content": "We standardized on Postgres for the OSS server store.",
    "keywords": ["postgres", "storage"],
    "source": "adr-0007"
  }'
```

Response `201`:

```json
{
  "id": "9b1c...",
  "project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
  "category": "decision",
  "content": "We standardized on Postgres for the OSS server store.",
  "content_hash": "sha256:...",
  "keywords": ["postgres", "storage"],
  "source": "adr-0007",
  "author_id": "0c8f...",
  "author_name": "Dev One",
  "created_at": "2026-07-20T12:00:00Z",
  "updated_at": "2026-07-20T12:00:00Z",
  "created": true
}
```

`created` is `false` when the write updated an existing memory instead of
inserting a new one.

#### `GET /v1/memories/search` — bearer

Project-scoped search. Both `project_id` and `q` are **required**.

| Query param | Required | Default | Notes |
|---|---|---|---|
| `project_id` | yes | — | Project UUID to search within. |
| `q` | yes | — | Query text. |
| `limit` | no | `20` | Capped at `100`. |
| `mode` | no | see below | `text` or `semantic`. |

`mode` defaults to `semantic` when the server has an embedder configured,
otherwise `text`. Any value other than `semantic` is treated as `text`. The
resolved mode is returned in the **`X-Anchored-Effective-Mode`** response
header and in each result's `effective_mode` field.

If semantic mode is requested (or defaulted) but unavailable — no embedder, or
the project index is still rebuilding — the call returns `422`:

```json
{"error": {"code": "semantic_unavailable", "message": "semantic search is unavailable because no embedder is configured"}}
```

(During an index rebuild the same code is returned with a `Retry-After: 1`
header.)

```bash
curl -s -G "$ANCHORED_URL/v1/memories/search" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  --data-urlencode "project_id=3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4" \
  --data-urlencode "q=which database did we pick" \
  --data-urlencode "limit=5"
```

Response `200` — an **array** of full memories, each with a 1-based `rank` and
the `effective_mode`:

```json
[
  {
    "id": "9b1c...",
    "project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
    "category": "decision",
    "content": "We standardized on Postgres for the OSS server store.",
    "content_hash": "sha256:...",
    "keywords": ["postgres", "storage"],
    "source": "adr-0007",
    "author_id": "0c8f...",
    "author_name": "Dev One",
    "created_at": "2026-07-20T12:00:00Z",
    "updated_at": "2026-07-20T12:00:00Z",
    "rank": 1,
    "effective_mode": "semantic"
  }
]
```

---

### Projects & knowledge graph

#### `GET /v1/projects` — bearer

Lists the projects the caller can reach (team grants for non-admins; all org
projects for admins). Returns a JSON array of project objects.

```bash
curl -s "$ANCHORED_URL/v1/projects" \
  -H "Authorization: Bearer $ANCHORED_TOKEN"
```

```json
[
  {
    "id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
    "org_id": "3a71...",
    "name": "Anchored OSS",
    "slug": "anchored-oss",
    "category": "service",
    "remote_key": "…",
    "remote_key_v1": "…",
    "repo_url": "https://github.com/acme/anchored_oss",
    "created_by": "0c8f...",
    "created_at": "2026-05-01T09:00:00Z"
  }
]
```

#### `GET /v1/projects/{id}` — bearer

Single active project (`404` when soft-deleted or not found; `400` when `{id}`
is not a UUID). Same object shape as above.

#### `GET /v1/projects/{id}/memories` — bearer

Paginated memories for a project.

| Query param | Default | Notes |
|---|---|---|
| `limit` | `20` | Page size. |
| `offset` | `0` | Page offset. |
| `category` | — | Optional exact-category filter. |

```bash
curl -s -G "$ANCHORED_URL/v1/projects/3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4/memories" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  --data-urlencode "limit=50" \
  --data-urlencode "category=decision"
```

```json
{
  "memories": [ { "id": "9b1c...", "category": "decision", "content": "…" } ],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

#### `GET /v1/projects/{id}/graph` — bearer

Paginated knowledge-graph triples for the project. `limit` defaults to `50`,
`offset` to `0`.

```json
{
  "triples": [
    {
      "id": "…",
      "subject": "anchored_oss",
      "predicate": "depends_on",
      "object": "postgres",
      "confidence": 0.9,
      "project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
      "created_at": "2026-07-20T12:00:00Z"
    }
  ],
  "total": 12,
  "limit": 50,
  "offset": 0
}
```

#### `POST /v1/projects/{id}/triples` — write

Ingest a batch of knowledge-graph triples (max 1000 per request). `readonly`
tokens get `403`; the caller must have write access to the project.

```bash
curl -s -X POST "$ANCHORED_URL/v1/projects/3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4/triples" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "triples": [
      {"subject": "anchored_oss", "predicate": "depends_on", "object": "postgres", "confidence": 0.9}
    ]
  }'
```

Response `200` — a per-batch summary; failed items are counted and described,
the batch still returns `200`:

```json
{"accepted": 1, "rejected": 0}
```

#### `GET /v1/projects/{id}/memory-health` — bearer

Anti context-poisoning health view for one project (lifecycle counts, noisy
sources, rejection pressure, volume anomalies). Any token with team access
(admin bypasses). Results are cached ~60s server-side.

---

### Chat (optional RAG)

Available only when the server has both an embedder and a chat provider
configured; otherwise it reports itself disabled rather than erroring.

#### `GET /v1/chat/status` — bearer

```bash
curl -s -G "$ANCHORED_URL/v1/chat/status" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  --data-urlencode "project_id=3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4"
```

```json
{"enabled": true, "model": "gpt-4o-mini", "index_state": "ready"}
```

`index_state` is one of `disabled`, `project_required` (enabled but no
`project_id` given), `ready`, `rebuilding`, or `error`.

#### `POST /v1/chat` — bearer

Grounded question answering over a project's memories.

```bash
curl -s -X POST "$ANCHORED_URL/v1/chat" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4", "query": "which database did we choose and why?"}'
```

```json
{
  "answer": "You standardized on Postgres for the OSS server store [1].",
  "sources": [
    {"id": "9b1c...", "category": "decision", "snippet": "We standardized on Postgres…"}
  ]
}
```

Returns `503` when chat or embeddings are not configured, `422`
`semantic_unavailable` while the index is rebuilding, `502` on a provider error.

---

### Sync

The bidirectional sync protocol for Anchored CLI clients. Full request/response
semantics live in the dedicated spec — see
[Sync Protocol](sync-protocol.md). Field shapes are defined in
`internal/model/sync.go`.

- `POST /v1/sync` — bearer. Canonical bidirectional protocol: `pushes`,
  `tombstones`, and watermark-based `pulls` in one call. `readonly` tokens may
  pull but are rejected (`403`) if they include `pushes`/`tombstones`. The
  response echoes the resolved `project_id` so claim-routed clients can make
  follow-up per-project calls (e.g. triple ingest).
- `POST /api/v1/sync/push` — bearer. Compatibility split-push for the CLI.
- `POST /api/v1/sync/pull` — bearer. Compatibility split-pull for the CLI.

---

### Task threads (personal kanban)

Private to the calling account — both routes derive the account from the token,
there is no way to read another account's threads, and there is no admin bypass.

- `GET /v1/me/task-threads` — bearer → `{"threads": [ … ]}`.
- `PUT /v1/me/task-threads` — bearer. Bulk upsert (≤100 threads) →
  `{"saved": <n>}`.

---

### Admin & ops

All routes below require **`admin`** scope unless noted. The org is always taken
from the token.

#### `POST /v1/api-keys` — admin

Mint a token for an account in the org. **Admin-only** — regular users mint
personal tokens through the dashboard (see
[Authentication](#obtaining-a-personal-token)).

Request: `{"name": "...", "scope": "sync|readonly|admin", "account_id": "<uuid>", "expires_in": "" | "7d" | "30d" | "90d"}`.

```bash
curl -s -X POST "$ANCHORED_URL/v1/api-keys" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "ci-bot", "scope": "sync", "account_id": "0c8f...", "expires_in": "90d"}'
```

Response `201` — the plaintext `key` is shown **once**:

```json
{
  "id": "…",
  "name": "ci-bot",
  "key": "anc_live_…64hex…",
  "scope": "sync",
  "created_at": "2026-07-20T12:00:00Z",
  "expires_at": "2026-10-18T12:00:00Z"
}
```

Other key routes: `GET /v1/api-keys` (list all keys in the org; hashes never
exposed), `DELETE /v1/api-keys/{id}` (revoke → `204`).

#### `GET /v1/stats` — admin

Aggregate org counters plus recent push activity (`DashboardStats`):

```json
{
  "accounts": 5, "organizations": 1, "teams": 2, "projects": 8,
  "memories_live": 1240, "keys_active": 6, "audit_entries_24h": 37,
  "recent_pushes": [ {"project_id": "…", "project_name": "…", "count": 12, "last_push": "2026-07-20T11:00:00Z"} ]
}
```

#### `GET /v1/quota` — admin

```json
{"storage_bytes": 1048576, "max_storage_bytes": 0, "memory_count": 1240}
```

`max_storage_bytes` of `0` means unlimited (no quota configured).

#### Accounts & teams

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/accounts` | admin | List org members (with roles). |
| `POST` | `/v1/accounts` | admin | Create/invite an account (`{email, display_name, password}` → `{id, email, display_name, created}`). |
| `PATCH` | `/v1/accounts/{id}` | admin | Update `{display_name, role}`. |
| `DELETE` | `/v1/accounts/{id}` | admin | Remove an account. |
| `GET` | `/v1/accounts/{id}/projects` | admin | Projects the account can reach. |
| `PUT` | `/v1/accounts/{id}/projects` | admin | Set the account's project grants (`{project_ids: []}`). |
| `GET` | `/v1/teams` | bearer | List teams in the org. |
| `POST` | `/v1/teams` | admin | Create a team (`{name, slug}`). |
| `GET` | `/v1/teams/{id}` | bearer | Team detail: members + project grants. |
| `POST` | `/v1/teams/{id}/members` | admin | Add member (`{account_id}` → `204`). |
| `DELETE` | `/v1/teams/{id}/members/{account_id}` | admin | Remove member → `204`. |

#### Invites

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/v1/invites` | admin | Create an invite (`{email, display_name, role}` → `{id, invite_url, expires_at}`). |
| `GET` | `/v1/invites` | admin | List pending invites. |
| `DELETE` | `/v1/invites/{id}` | admin | Revoke an invite. |
| `GET` | `/v1/invites/accept/{token}` | none | Validate an invite token → `{valid, …}`. |
| `POST` | `/v1/invites/accept/{token}` | none | Accept (`{password}` → `{api_key, account_id}`). |

#### Projects (admin lifecycle)

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/v1/projects` | admin | Create a project (`{name, slug, remote_key?, repo_url?, category?}`); idempotent on `remote_key`; grants the creator via the org default team. |
| `PATCH` | `/v1/projects/{id}` | admin | Partial update (`{name?, slug?, repo_url?, category?}`); setting `repo_url` recomputes remote keys, clearing it unlinks the repo. |
| `DELETE` | `/v1/projects/{id}` | admin | Soft-delete → `204`. |
| `DELETE` | `/v1/projects/{id}/memories` | admin | Bulk-tombstone memories in a time window; requires at least one of `since`/`until` (RFC3339) → `{deleted: <n>}`. |
| `GET` | `/v1/orgs/memory-health` | admin | Org-wide memory-health aggregate. |

#### Policies & guardrails

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/policies` | admin | Effective org policy: `{blocked_categories, quality_threshold, near_dup_threshold, max_memories_per_sync, always_on}`. |
| `PUT` | `/v1/policies` | admin | Update thresholds/blocked categories. |
| `GET` | `/v1/guardrails` | admin | List sync-time guardrail rules (builtins + custom). |
| `POST` | `/v1/guardrails` | admin | Add a custom rule (`kind` ∈ `category`\|`regex`\|`keyword`, `value` required). |
| `PATCH` | `/v1/guardrails/{id}` | admin | Enable/disable or edit; builtins are toggle-only. |
| `DELETE` | `/v1/guardrails/{id}` | admin | Delete a custom rule (builtins cannot be deleted). |

The `always_on` guardrails (`secret_detection`, `local_path_redaction`,
`user_scope_block`) are enforced in code and cannot be removed.

#### Audit & self-update

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/audit` | admin | Audit log with `project`/`actor`/`action`/`target_type`/`from`/`to`/`limit`/`offset` filters → `{entries, total, limit, offset}`. |
| `GET` | `/v1/admin/update/check` | admin | `{current_version, latest_version, update_available}`. |
| `POST` | `/v1/admin/update/apply` | admin | Trigger a self-update → `202 {"status": "updating"}`. |

#### Unauthenticated setup & auth

These support first-run bootstrap and the dashboard login/registration flow.
The auth routes are rate-limited.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/bootstrap-status` | none | `{bootstrapped: bool}` — whether any account exists yet. |
| `POST` | `/v1/onboarding/complete` | none | First-run admin/org creation. |
| `POST` | `/v1/auth/login` | none | Email+password → mints an `admin` session token `{api_key, account_id, org_id, scope}`. |
| `POST` | `/v1/auth/register` | none | Self-service registration (when enabled). |
| `GET` | `/install`, `/install-oss` | none | Serve the install shell scripts. |

---

## Quick start for a personal token

With `$ANCHORED_URL` and `$ANCHORED_TOKEN` exported (token minted from the
dashboard API Keys page):

```bash
# 1. Confirm the token and see who you are.
curl -s "$ANCHORED_URL/v1/me" \
  -H "Authorization: Bearer $ANCHORED_TOKEN"

# 2. Find a project you can reach.
curl -s "$ANCHORED_URL/v1/projects" \
  -H "Authorization: Bearer $ANCHORED_TOKEN"

# 3. Save a memory into it (needs a non-readonly token).
curl -s -X POST "$ANCHORED_URL/v1/memories" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4",
    "category": "learning",
    "content": "Semantic search falls back to text when no embedder is set."
  }'

# 4. Search it back.
curl -s -G "$ANCHORED_URL/v1/memories/search" \
  -H "Authorization: Bearer $ANCHORED_TOKEN" \
  --data-urlencode "project_id=3f2a9c14-6b8e-4d2a-9f1c-77aa10b2c3d4" \
  --data-urlencode "q=semantic search fallback"
```

---

## Error handling

Anchored uses **two error shapes**, depending on the endpoint.

**1. Flat (most endpoints)** — the HTTP status is the primary signal, the string
identifies the failure:

```json
{"error": "UNAUTHORIZED"}
```

Common flat codes: `UNAUTHORIZED` (401), `FORBIDDEN` (403), `INVALID_REQUEST`
(400), `PROJECT_NOT_FOUND` (404), `CONFLICT` (409), `QUOTA_EXCEEDED` (403),
`INTERNAL_ERROR` (500). Some validation errors return a human-readable message
in the same `{"error": "..."}` field (e.g. `"content is required"`).

**2. Nested (memory endpoints)** — `POST /v1/memories` idempotency failures and
`GET /v1/memories/search` semantic failures use a coded object:

```json
{"error": {"code": "semantic_unavailable", "message": "semantic search is unavailable because no embedder is configured"}}
```

Nested codes include `semantic_unavailable` (422), `semantic_query_failed`
(503), `idempotency_conflict` (409), `memory_id_conflict` (409),
`invalid_idempotency_key` (400), `idempotency_replay_unavailable` (410),
`idempotency_unavailable` (501).

The full catalog of sync error codes and the per-item rejection rules
(`local_path_detected`, `secret_detected`, `blocked_category`, `not_found`) is
in [Error Codes](error-codes.md).

### Known constraints

- **No `GET /v1/memories/{id}`** — read individual memories via project listing
  or search.
- **Token minting is admin-only** (`POST /v1/api-keys`); regular users create
  personal tokens through the dashboard.
- **Category restrictions** — saved memories must be `fact`, `decision`,
  `plan`, `summary`, or `learning`; `event` and `preference` are rejected.
- **Bearer only** — no cookie/session auth; the org is always derived from the
  token, never from the request.

---

## Related

- [Sync Protocol](sync-protocol.md) — bidirectional sync specification.
- [Error Codes](error-codes.md) — sync error codes and per-item rejection rules.
- [`skill/anchored-teams`](../skill/anchored-teams/) — a ready-made Claude Code
  skill that drives this API from an agent using a personal token (curl-based;
  reads `ANCHORED_OSS_URL` / `ANCHORED_OSS_TOKEN`).
</content>
</invoke>
