# Client ↔ server contract — anchored v0.21 + Anchored OSS v0.7

Status: **draft**, agreed between the client and server workstreams. Items
marked **[Jhol]** wait for the maintainer's confirmation (decisions D1–D5 of the
redesign are provisional until then).

Compatibility rule: every item is additive. A v0.21 client against a 0.5.7 /
0.6.x server, and a 0.19.x / 0.20.x client against a v0.7 server, keep today's
behaviour.
Changes on the way from 0.6.x to v0.7 that narrow access or alter an answer
(org isolation, key and session rules, onboarding, skills headers, rate
limiting) are listed in [docs/migration-v0.7.md](../migration-v0.7.md).

## 1. Shared module (C8)

- Module `github.com/jholhewres/anchored/protocol`, its own `go.mod`, pure Go
  (the server builds with `CGO_ENABLED=0`). Lives in the client repo; tagged
  `protocol/vX.Y.Z`. Until the first tag the server uses a local `replace`.
- Contents:
  - `remotekey`: git origin → remote key, with the shared test vectors (today in
    `internal/project/remotekey_test.go` and the client's equivalents).
  - `redaction`: secret and local-path filters used before anything leaves the
    machine and again on ingest.
  - `tokenizer`: the client's pure-Go WordPiece tokenizer with its golden set
    (fixes the v0.19 bug the server copy still has). ONNX stays out.
  - Tool and skill schemas (section 5), sync v2 types (section 7), the
    capabilities document (section 2).

## 2. `GET /v1/capabilities`

- Public (no credential), like `/v1/health`, so `anchored remote add <url>` can
  detect the server before logging in. Returns nothing org-specific.
- `Cache-Control: max-age=300` and an `ETag`; the client caches it 5 min per
  remote.
- 404 means a legacy server (0.5.7 / 0.6.x): the client keeps today's behaviour
  (static `anc_live_` key, compat sync, probe-free skill fallback).

```json
{
  "instance_id": "8f6c…-uuid",
  "version": "v0.7.0",
  "api_level": 2,
  "features": {
    "mcp": {
      "endpoint": "/mcp",
      "protocol_versions": ["2026-07-28", "2025-11-25", "2025-06-18"],
      "tools": ["anchored_context", "anchored_search", "anchored_save",
                "anchored_update", "anchored_forget", "anchored_graph",
                "anchored_task", "anchored_skill"],
      "skills_extension": true
    },
    "oauth": {
      "issuer": "https://mem.example.com",
      "authorization_server_metadata": "/.well-known/oauth-authorization-server",
      "protected_resource_metadata": "/.well-known/oauth-protected-resource/mcp",
      "scopes": ["memories:read", "memories:write", "skills:read", "admin"],
      "device_code": false
    },
    "legacy_api_keys": true,
    "skills": { "rest": true },
    "sync": { "v2": true, "tombstones": true, "import_batches": true },
    "search": { "bm25": true, "hybrid": true },
    "semantic": { "available": true, "provider": "local", "space_id": "…",
                  "client_vectors": true },
    "memory": {
      "categories": ["fact", "preference", "decision", "event", "learning",
                     "plan", "summary"],
      "scopes": ["user", "project", "team", "org"]
    }
  }
}
```

- `instance_id` is a stable UUID created at first boot and stored in the DB. The
  client keys its keychain entries and the skills' server identity on it.
- A feature the server does not have is `false` or absent; the client never
  infers capabilities from 404/422 when this document exists.

## 3. Authorization

The instance is both the OAuth Authorization Server (AS) and the protected
resource (RS). Minimal AS, only the subset MCP needs **[Jhol: D2]**.

- Issuer: the configured public base URL (`https://host`, no path), never
  derived from the `Host` header. It must stay stable across upgrades, because
  clients key their registrations on it.
- Discovery: RFC 9728 protected resource metadata at
  `/.well-known/oauth-protected-resource/mcp` and
  `/.well-known/oauth-protected-resource`, with `resource` exactly
  `<base>/mcp` and `authorization_servers: [issuer]`; RFC 8414 AS metadata at
  `/.well-known/oauth-authorization-server` with `response_types_supported:
  ["code"]`, `grant_types_supported: ["authorization_code", "refresh_token"]`,
  `code_challenge_methods_supported: ["S256"]`,
  `token_endpoint_auth_methods_supported: ["none"]`,
  `client_id_metadata_document_supported: true`, `registration_endpoint`,
  `revocation_endpoint`, `authorization_response_iss_parameter_supported: true`
  and `scopes_supported` (the four scopes plus `offline_access`).
- The auth gate runs in the HTTP layer, before the MCP SDK. A missing or invalid
  token answers `401` with `WWW-Authenticate: Bearer resource_metadata="…",
  scope="…"`; a missing scope answers `403` with `error="insufficient_scope"`
  and every scope the call needs. `/mcp` is an exact route (no subtree
  redirect).
- Flows: authorization code + PKCE (S256 only). No implicit, no password grant.
  `iss` is returned on every authorization response, success or error
  (RFC 9207). A refresh token is issued when `offline_access` is requested.
- Clients:
  - `anchored-cli`: a public client built into every instance; redirect
    `http://127.0.0.1:{any port}/callback` (RFC 8252 §7.3); no registration.
  - Claude, ChatGPT, Codex, Cursor: Client ID Metadata Documents (CIMD), with
    Dynamic Client Registration (RFC 7591) kept for clients that still need it
    (Cursor, and Claude's fallback). CIMD documents are fetched SSRF-safe
    (https only, no private or link-local addresses, size cap).
  - Redirect URIs match exactly, except loopback (`127.0.0.1`, `[::1]`,
    `localhost`), where the port is ignored.
  - The consent screen names the `client_id` host and the redirect host, and
    warns when every redirect is localhost.
- Audience: one RS for the whole instance. `resource` may be `<base>` or
  `<base>/mcp`; the token is valid on `/mcp`, `/v1/*` and `/api/v1/sync/*` of
  that instance and nowhere else.
- Scopes: `memories:read`, `memories:write`, `skills:read`, `admin`. The grant
  also carries the chosen org and the allowed projects.
- Tokens: opaque, stored hashed. Access `anc_at_…` (1 h). Refresh `anc_rt_…`
  (30 days, rotated on use). Reuse of a rotated refresh token inside a **60 s
  grace window** returns the same new pair (several local processes may refresh
  at once); reuse after the window revokes the grant.
- Revocation: RFC 7009 `revoke` endpoint, used by `anchored remote logout`; the
  web "Connections" screen lists and revokes grants.
- Device code (RFC 8628): not in v0.7. Headless machines use
  `anchored remote add --api-key` with a personal key.
- Consent screen: login → pick the org (or create one, see the open item in
  section 9) → pick projects and scopes → confirm.

### Legacy credentials

- `anc_live_…` keys stay valid on `/v1/*`, `/api/v1/sync/*` and also on `/mcp`
  (as a static Bearer, for clients configured with a header).
- Personal keys (`/v1/me/api-keys`, scopes `sync` and `readonly`) are what the
  onboarding hands to developers; admin keys are never handed out by onboarding.

## 4. Web session

Cookie session for the dashboard (HttpOnly, Secure, SameSite=Lax), CSRF token
on state-changing requests, idle and absolute expiry, logout. The dashboard no
longer keeps a key in `localStorage`. Bearer keys keep working for API clients.

## 5. Remote MCP (`/mcp`)

- Streamable HTTP via the official go-sdk. Serves `2026-07-28` (stateless,
  `server/discover`) and the older `2025-11-25` / `2025-06-18` clients
  (`initialize`) on the same endpoint.
- Tools: the client's lean surface, same names and schemas from `protocol/`:
  `anchored_context`, `anchored_search` (empty query lists),
  `anchored_save` (`pin`, `scope`, `expires`), `anchored_update`,
  `anchored_forget`, `anchored_graph` (query + add), `anchored_task`, plus
  `anchored_skill` (`search` | `load`) as the skills fallback. A tool the
  server does not support is left out of `tools/list` and of
  `capabilities.features.mcp.tools`.
- Project: there is no `cwd` remotely. The `project` argument (slug, id,
  remote key or git URL) wins, then the grant's default project, else an error
  that lists the projects the grant allows.
- Skills extension `io.modelcontextprotocol/skills`: `skills/list`,
  `skills/get`, `resources/read` on `skill://<slug>/SKILL.md`. Digest =
  SHA-256 of the exact bytes served (no CRLF or whitespace normalization);
  the manifest lists every file; unknown URI → `-32602`. The declaration goes in
  both the `initialize` and the `server/discover` capabilities. Hosts without
  the extension (Claude, Cursor and Codex today; ChatGPT only imports a static
  snapshot) use `anchored_skill` and the `skill://` files, which are also
  listed in `resources/list`.
- Per-user results (`tools/list` aside) use `cacheScope: "private"`.
- Server instructions: short, generated from the same source as the client's
  skills.

## 6. Skills over REST (unchanged for the local client)

`GET /v1/projects/{id}/skills` and `GET /v1/projects/{id}/skills/{slug}` keep
returning the canonical `SKILL.md` as `content` and its `content_hash`
(= SHA-256 of `content`). Legacy rows keep their bytes and hash.

## 7. Memory model and sync v2

- Categories: all 7 accepted by `POST /v1/memories` and by sync.
- Scope: one field, `user | project | team | org`. `user` stays blocked by the
  client's safety filter by default; a remote may opt in with
  `sync_user_scope: true` (meant for a personal instance), and the server then
  keeps it private to the grant's account, never visible to team or org.
- Lifecycle columns: `kind`, `importance`, `pinned`, `expires_at`,
  `supersedes`.
- Sync v2 (enabled only when `features.sync.v2` is true):
  - LWW by the client's edit time (`updated_at`), clamped to the server clock
    plus a skew bound; the server never stamps `now()` over a client edit.
  - Tombstones both ways; a tombstoned memory is never resurrected by a
    re-push of an older version.
  - Incremental push by cursor; pull by watermark.
  - `POST /v1/import/batches` for `anchored migrate`.
- Search: BM25 (`tsvector` on Postgres, FTS5 on SQLite) + pgvector, fused with
  RRF (k = 60). Vectors may be sent by the client, tagged with
  `semantic_space_id`; the server compares only within a space.

## 8. Versions and rollout

Client v0.21 and OSS v0.7 ship together **[Jhol: D4]**, with a compatibility
matrix (new client × 0.5.7 / 0.6.1 / 0.7 servers; 0.19.2 / 0.20 clients × 0.7
server) and a migration guide.

## 9. Orgs

- An account may belong to several orgs (`org_members` already allows it). The
  web session and each grant carry the chosen org; the dashboard has an org
  switch.
- Creating an org from the consent screen is allowed only when the instance
  enables it; by default only the instance admin may.
