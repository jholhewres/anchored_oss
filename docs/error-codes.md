# Error Codes

Every error from the sync endpoint uses the same JSON shape:

```json
{"error": "ERROR_CODE"}
```

The HTTP status code is the primary signal. The error code string identifies the specific failure for programmatic handling.

---

## Authentication and Authorization

### UNAUTHORIZED

**HTTP 401**

The `Authorization` header is missing, malformed, or contains an invalid or revoked API key.

```json
{"error": "UNAUTHORIZED"}
```

### FORBIDDEN

**HTTP 403**

The authenticated account does not have the required access. This covers three distinct scenarios:

1. **Insufficient scope**: the API key has `readonly` scope but the request includes `pushes` or `tombstones`.
2. **No team access**: the account is not a member of any team with access to the target project.
3. **Cross-org project**: the project belongs to a different organization than the one associated with the API key.

```json
{"error": "FORBIDDEN"}
```

---

## Request Validation

### INVALID_REQUEST

**HTTP 400**

The request body is malformed JSON, missing required fields, or structurally invalid. Triggers include:

- Malformed JSON body
- Both `project_id` and `project_claim` missing (one is required)
- Missing required fields within `pushes` items

```json
{"error": "INVALID_REQUEST"}
```

---

## Project Resolution

### PROJECT_NOT_FOUND

**HTTP 404**

The `project_id` does not match any existing project, or auto-creation from a `project_claim` is not supported yet.

```json
{"error": "PROJECT_NOT_FOUND"}
```

### AUTO_CREATE_DISABLED

**HTTP 403**

The organization has disabled automatic project creation. The client must use an existing `project_id` or ask an org admin to create the project manually.

```json
{"error": "AUTO_CREATE_DISABLED"}
```

---

## Content Policy (Rejection Rules)

These are not top-level errors. They appear inside individual `SyncResult` entries with `"status": "rejected"`. The overall sync request still succeeds (HTTP 200), and accepted items are still written. Only the offending items are rejected.

### local_path_detected

The memory content contains a local filesystem path pattern. The server rejects content with patterns like:

- `/home/...`, `/Users/...`, `C:\Users\...`
- `~/.config/...`, `~/...`

Remote references should use repository-relative paths (e.g. `pkg/memory/service.go`).

```json
{
  "id": "mem-001",
  "status": "rejected",
  "rule": "local_path_detected",
  "detail": "local path pattern detected: /home/alice/.config"
}
```

### secret_detected

The memory content matches known secret patterns: API keys, tokens, private keys, connection strings, or credential fragments.

```json
{
  "id": "mem-002",
  "status": "rejected",
  "rule": "secret_detected",
  "detail": "secret pattern detected: sk_live_..."
}
```

### not_found

The memory ID in `tombstones` does not match any live memory in the project. Either it never existed on the server or it has already been deleted. Reported per-item; the rest of the sync request still succeeds.

```json
{
  "id": "mem-deleted-already",
  "status": "rejected",
  "rule": "not_found",
  "detail": "memory not found or already deleted"
}
```

### blocked_category

The memory uses a category that is not allowed for remote sync. The blocked categories are:

- `event`: session events and behavioral metadata should not leave the local machine
- `preference`: personal preferences should not be pushed to team memory

Allowed categories: `fact`, `decision`, `learning`, `plan`, `summary`.

```json
{
  "id": "mem-003",
  "status": "rejected",
  "rule": "blocked_category",
  "detail": "category \"event\" is not allowed"
}
```

---

## Conflict

### CONFLICT

**HTTP 409**

A memory with the same ID exists in the project but has a different `content_hash`. This indicates the client and server have divergent copies of the same logical memory that cannot be resolved by last-write-wins alone.

```json
{"error": "CONFLICT"}
```

---

## Server Errors

### INTERNAL_ERROR

**HTTP 500**

An unexpected server-side failure. The client should log the error, keep its local state intact, and retry after a backoff period.

```json
{"error": "INTERNAL_ERROR"}
```

---

## Summary Table

| Code | HTTP Status | Level | Description |
|---|---|---|---|
| `UNAUTHORIZED` | 401 | request | Invalid or missing API key |
| `FORBIDDEN` | 403 | request | Insufficient scope or no team access |
| `INVALID_REQUEST` | 400 | request | Malformed JSON or missing fields |
| `PROJECT_NOT_FOUND` | 404 | request | Project ID does not exist |
| `AUTO_CREATE_DISABLED` | 403 | request | Org does not allow auto-creation |
| `CONFLICT` | 409 | request | Hash mismatch on existing memory |
| `INTERNAL_ERROR` | 500 | request | Unexpected server failure |
| `local_path_detected` | n/a | item | Content contains local paths (rejection rule) |
| `secret_detected` | n/a | item | Content contains secret patterns (rejection rule) |
| `blocked_category` | n/a | item | Category not allowed for sync (rejection rule) |
| `not_found` | n/a | item | Tombstone targets a memory that does not exist (rejection rule) |
| `internal_error` | n/a | item | Server failed to persist this specific item (rejection rule) |
