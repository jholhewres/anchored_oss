# Sync Protocol v1

The Anchored OSS sync protocol is a bidirectional JSON protocol that lets local Anchored clients push and pull team memories over a single HTTP endpoint. It is designed for intermittent connectivity: the client batches local changes, sends them in one request, and receives everything new from the server in the same response.

---

## Authentication

Every request must include an API key in the `Authorization` header:

```http
Authorization: Bearer anch_live_abc123...
```

The API key determines the account, organization, and access scope. Keys are created via `POST /v1/api-keys` and carry one of three scopes:

| Scope | Can push | Can pull | Notes |
|---|---|---|---|
| `sync` | yes | yes | Full read/write |
| `readonly` | no | yes | Pull only |
| `admin` | yes | yes | Passes all scope checks |

Requests with missing or invalid keys receive `401 UNAUTHORIZED`.

---

## Endpoint

```http
POST /v1/sync
Content-Type: application/json
Authorization: Bearer <api-key>
```

There is one endpoint. All sync state flows through it.

---

## Request Format

```json
{
  "project_id": "550e8400-e29b-41d4-a716-446655440000",
  "client_id": "client-abc-123",
  "watermark": "2026-05-22T10:00:00Z",
  "pushes": [],
  "tombstones": [],
  "project_claim": null
}
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `project_id` | `string` | conditional | UUID of an existing project. Required unless `project_claim` is provided. |
| `client_id` | `string` | yes | Stable identifier for the calling client instance. Used for logging and debugging. |
| `watermark` | `string` (RFC 3339) | no | Timestamp of the last known server state. When present, the server returns only memories updated after this time. Omit or send `null` to pull everything. |
| `pushes` | `array[SyncMemory]` | no | Local memories to push to the server. Empty or omitted when the client only wants to pull. |
| `tombstones` | `array[string]` | no | IDs of locally deleted memories that should be tombstoned on the server. |
| `project_claim` | `ProjectClaim` | conditional | Used to resolve or auto-create a project by repository identity. Required unless `project_id` is provided. |

Either `project_id` or `project_claim` must be present. If neither is provided, the server returns `400 INVALID_REQUEST`.

---

## SyncMemory Format

Each item in `pushes` represents a single memory from the local client:

```json
{
  "id": "mem-001",
  "category": "decision",
  "content": "We chose Postgres over SQLite for team sync",
  "content_hash": "sha256:a1b2c3d4e5f6...",
  "keywords": ["database", "postgres", "architecture"],
  "source": "claude-code",
  "author_name": "John",
  "created_at": "2026-05-22T09:30:00Z",
  "updated_at": "2026-05-22T09:30:00Z"
}
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | `string` | yes | Client-assigned unique identifier for this memory. Must be stable across syncs. |
| `category` | `string` | yes | One of: `fact`, `decision`, `learning`, `plan`, `summary`. The server rejects blocked categories (`event`, `preference`). |
| `content` | `string` | yes | The memory text. Must not contain local filesystem paths, secrets, or credentials. |
| `content_hash` | `string` | yes | SHA-256 hash of the content, prefixed with `sha256:`. Used for deduplication. |
| `keywords` | `array[string]` | no | Descriptive tags for search and filtering. |
| `source` | `string` | no | Originating tool name (e.g. `claude-code`, `cursor`, `opencode`). |
| `author_name` | `string` | yes | Display name of the person who created the memory. |
| `created_at` | `string` (RFC 3339) | yes | When the memory was originally created on the client. |
| `updated_at` | `string` (RFC 3339) | yes | When the memory was last modified. Used for conflict resolution. |

---

## ProjectClaim Format

When a client syncs from a repository that does not yet have a project on the server, it can send a claim to auto-create one:

```json
{
  "project_claim": {
    "name": "anchored",
    "remote_key": "git:sha256:abc123...",
    "git_host": "github.com",
    "repo_slug": "jholhewres/anchored"
  }
}
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | yes | Human-readable project name. |
| `remote_key` | `string` | yes | Unique key derived from the repository. Prefixed with `git:` followed by a hash. Used to match an existing project or create a new one. |
| `git_host` | `string` | no | Git hosting provider hostname (e.g. `github.com`, `gitlab.com`). |
| `repo_slug` | `string` | no | Repository path in `org/repo` format. |

The server validates that the claim does not contain local paths, home directories, or usernames.

---

## Response Format

```json
{
  "pulls": [],
  "server_tombstones": [],
  "results": [],
  "watermark": "2026-05-22T10:05:00Z"
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `pulls` | `array[Memory]` | Server memories updated after the request watermark. Empty when the client is up to date. |
| `server_tombstones` | `array[string]` | IDs of memories that were deleted on the server. The client should remove these locally. |
| `results` | `array[SyncResult]` | Outcome for each item in `pushes` and `tombstones`, in order. Always present, even if empty. |
| `watermark` | `string` (RFC 3339) | New watermark reflecting the server's current state. The client must store this and send it in the next sync request. |

### Memory Format (pull items)

Each item in `pulls` is a full server-side memory:

```json
{
  "id": "mem-099",
  "project_id": "550e8400-e29b-41d4-a716-446655440000",
  "category": "learning",
  "content": "net/http ServeMux supports method+path routing since Go 1.22",
  "content_hash": "sha256:9988776655...",
  "keywords": ["go", "http", "routing"],
  "source": "opencode",
  "author_id": "660e8400-e29b-41d4-a716-446655440001",
  "author_name": "Alice",
  "created_at": "2026-05-22T08:00:00Z",
  "updated_at": "2026-05-22T08:00:00Z",
  "deleted_at": null,
  "metadata": null
}
```

| Field | Type | Description |
|---|---|---|
| `id` | `string` | Server-assigned memory ID. |
| `project_id` | `string` | UUID of the project this memory belongs to. |
| `category` | `string` | Memory category. |
| `content` | `string` | Memory text. |
| `content_hash` | `string` | SHA-256 hash of the content. |
| `keywords` | `array[string]` | Tags for search. |
| `source` | `string` | Tool that created the memory. |
| `author_id` | `string` | UUID of the account that authored the memory. |
| `author_name` | `string` | Display name of the author. |
| `created_at` | `string` (RFC 3339) | Creation timestamp. |
| `updated_at` | `string` (RFC 3339) | Last modification timestamp. |
| `deleted_at` | `string` (RFC 3339) | Soft-delete timestamp. `null` for live memories. |
| `metadata` | `any` | Optional arbitrary metadata. `null` when absent. |

---

## SyncResult Format

Each pushed or tombstoned item gets a result entry:

**Accepted:**

```json
{
  "id": "mem-001",
  "status": "accepted"
}
```

**Rejected:**

```json
{
  "id": "mem-bad-001",
  "status": "rejected",
  "rule": "local_path_detected",
  "detail": "/home/alice/.config/anchored"
}
```

| Field | Type | Description |
|---|---|---|
| `id` | `string` | ID of the pushed or tombstoned item. |
| `status` | `string` | `"accepted"` or `"rejected"`. |
| `rule` | `string` | Rejection rule name. Present only when `status` is `"rejected"`. One of: `local_path_detected`, `secret_detected`, `blocked_category`. |
| `detail` | `string` | Human-readable explanation of the rejection. Present only when `status` is `"rejected"`. |

---

## Watermark Behavior

Watermarks enable incremental sync. The flow is:

1. Client stores the watermark from the previous sync response (or starts with no watermark).
2. Client sends a request with `watermark` set to the stored value.
3. Server returns only memories with `updated_at` after that watermark.
4. Response includes a new `watermark`. Client persists it for the next sync.

If `watermark` is omitted or `null`, the server returns all non-deleted memories for the project. This is useful for initial sync or full re-syncs.

---

## Conflict Resolution

The server uses **last-write-wins** based on `updated_at`:

- If a pushed memory has an `updated_at` newer than the existing server copy, the server overwrites it.
- If the server copy is newer, the server keeps its version and returns it in `pulls` so the client can update locally.
- Tombstoned memories are soft-deleted (`deleted_at` is set). Tombstones propagate to other clients via `server_tombstones`.

---

## Idempotency

The `content_hash` field enables content-based deduplication:

- If a push item has a `content_hash` that already exists in the same project, and the hash matches, the server treats it as a no-op. The item is marked `accepted`, but no new row is written.
- This means retrying the same sync request is safe: the server will not create duplicates.
- The unique index is scoped per project: `UNIQUE(content_hash, project_id) WHERE deleted_at IS NULL`.

---

## Scope Requirements

The API key scope controls what operations are allowed:

- **`sync` scope**: full push and pull. The client can send `pushes` and `tombstones`, and receives `pulls` and `server_tombstones`.
- **`readonly` scope**: pull only. If the request includes any `pushes` or `tombstones`, the server returns `403 FORBIDDEN`.
- **`admin` scope**: same as `sync`, plus access to management endpoints.

---

## Error Responses

Errors are returned as JSON:

```json
{"error": "ERROR_CODE"}
```

The HTTP status code and error code together identify the failure. See [error-codes.md](./error-codes.md) for the full catalog.

---

## Sequence Diagram

```text
Client                                    Server
  |                                         |
  |  POST /v1/sync                          |
  |  { watermark, pushes, tombstones }      |
  |---------------------------------------->|
  |                                         |
  |                               validate auth & scope
  |                               resolve project
  |                               filter pushes (policy)
  |                               write accepted pushes
  |                               apply tombstones
  |                               query pulls since watermark
  |                                         |
  |  { pulls, results, watermark }          |
  |<----------------------------------------|
  |                                         |
  store new watermark                       |
  apply pulls locally                       |
  remove tombstoned memories                |
```
