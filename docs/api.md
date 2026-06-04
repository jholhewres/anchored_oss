# API Reference

All endpoints are JSON over HTTP. Authentication uses bearer API keys
(`Authorization: Bearer anc_live_...`) minted from the dashboard or the
bootstrap command.

## API key scopes

| Scope | Capabilities |
|---|---|
| `admin` | Full access; bypasses team-access checks. |
| `sync` | Push and pull memories/triples for accessible projects. |
| `readonly` | Pull only. |

## Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/health` | none | Liveness + DB status. Returns 503 when DB is down. |
| `GET` | `/v1/me` | bearer | Caller profile (account, scope, org). |
| `GET` | `/v1/stats` | admin | Aggregate counts + recent push activity. |
| `POST` | `/v1/sync` | bearer | Bidirectional sync (push, tombstone, pull). |
| `POST` | `/api/v1/sync/push` | bearer | Compat push for the anchored CLI client. |
| `POST` | `/api/v1/sync/pull` | bearer | Compat pull for the anchored CLI client. |
| `GET` | `/v1/projects` | bearer | List projects the caller has access to. |
| `GET` | `/v1/projects/{id}` | bearer | Project detail (404 when soft-deleted). |
| `GET` | `/v1/projects/{id}/memories` | bearer | Paginated memories for the project (`category` filter supported). |
| `GET` | `/v1/projects/{id}/graph` | bearer | Paginated knowledge-graph triples for the project. |
| `POST` | `/v1/projects/{id}/triples` | bearer | Ingest knowledge-graph triples (write access required). |
| `GET` | `/v1/memories/search` | bearer | Project-scoped memory search (`mode=text\|semantic`). |
| `POST` | `/v1/projects` | admin | Create a project; auto-grants the creator via the org's default team. |
| `DELETE` | `/v1/projects/{id}` | admin | Soft-delete a project. |
| `GET` | `/v1/accounts` | admin | List organisation members. |
| `POST` | `/v1/accounts` | admin | Invite an account (idempotent on email). |
| `GET` | `/v1/teams` | bearer | List teams in the org. |
| `POST` | `/v1/teams` | admin | Create a team. |
| `GET` | `/v1/teams/{id}` | bearer | Team detail with members + project grants. |
| `POST` | `/v1/teams/{id}/members` | admin | Add a team member. |
| `DELETE` | `/v1/teams/{id}/members/{account_id}` | admin | Remove a team member. |
| `GET` | `/v1/api-keys` | admin | List all keys in the org. |
| `POST` | `/v1/api-keys` | admin | Mint a new API key (optional expiry 7d/30d/90d). |
| `DELETE` | `/v1/api-keys/{id}` | admin | Revoke a key. |
| `GET` | `/v1/audit` | admin | Audit entries with project/actor/action/date filters. |
| `GET` | `/v1/guardrails` | admin | List the org's sync-time guardrail rules. |
| `POST` | `/v1/guardrails` | admin | Add a custom guardrail (category block, keyword, or RE2 regex). |
| `PATCH` | `/v1/guardrails/{id}` | admin | Enable/disable or edit a guardrail (builtins toggle-only). |
| `DELETE` | `/v1/guardrails/{id}` | admin | Delete a custom guardrail (builtins cannot be deleted). |
| `GET` `PUT` | `/v1/policies` | admin | Scoring thresholds (quality, near-duplicate). |

Sync responses include the resolved `project_id` (servers ≥ v0.4.4) so
clients routing by git-origin `project_claim` can target follow-up
per-project calls such as triple ingest.

## Related

- [Sync Protocol](sync-protocol.md) — bidirectional sync specification
- [Error Codes](error-codes.md) — API error codes and item-level rejection rules
