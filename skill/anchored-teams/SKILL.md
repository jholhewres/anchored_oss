---
name: anchored-teams
description: >-
  Access a remote, self-hosted Anchored OSS (Anchored Teams) server over its
  HTTP API using a personal API token — search and save team memories, list
  projects and their memories, and optionally ask the RAG chat endpoint. Use
  this whenever the anchored MCP memory server is NOT available but you have an
  Anchored OSS server URL and a personal token (env vars ANCHORED_OSS_URL and
  ANCHORED_OSS_TOKEN). Triggers on: "anchored oss", "anchored teams", accessing
  or searching team memory on a remote/self-hosted anchored server, saving a
  memory to the team server over the API/token, listing a project's team
  memories via curl. This is complementary to the local anchored MCP memory and
  targets a shared Teams server instead.
---

# Anchored Teams — API access with a personal token

Talk to a self-hosted **Anchored OSS** server directly over HTTP with `curl`,
authenticated by the user's own personal API token. Use this when the local
`anchored` MCP tools are not available but a Teams server and a token are.

All endpoints are under `/v1`. Auth is a bearer token:
`Authorization: Bearer anc_live_<hex>`.

## Configuration (read these env vars — never hardcode)

- `ANCHORED_OSS_URL` — base URL, e.g. `http://localhost:8771` or
  `https://memory.your-domain.com` (no trailing slash needed).
- `ANCHORED_OSS_TOKEN` — the personal API token, starting with `anc_live_`.

**Never print, echo, or log the token.** Always pass it through the environment
variable (`$ANCHORED_OSS_TOKEN`), never inline the literal value into a command
you show the user.

### Preflight

Before any call, confirm both vars are set. Do NOT reveal the token value:

```bash
[ -n "$ANCHORED_OSS_URL" ] && echo "URL set: $ANCHORED_OSS_URL" || echo "ANCHORED_OSS_URL missing"
[ -n "$ANCHORED_OSS_TOKEN" ] && echo "TOKEN set (hidden)" || echo "ANCHORED_OSS_TOKEN missing"
```

If either is missing, tell the user to set them and how to get a token:

1. Open the server dashboard in a browser (`$ANCHORED_OSS_URL`), sign in.
2. Go to **API Keys**, create a key, copy the `anc_live_...` value (shown once).
3. Export the vars in the shell (or add to a profile / `.env`):
   ```bash
   export ANCHORED_OSS_URL="http://your-server:8771"
   export ANCHORED_OSS_TOKEN="anc_live_..."   # paste your key
   ```

Minting tokens via the API is admin-only, so always direct the user to the
dashboard to create their personal token.

A convenience wrapper used by the snippets below (define once per shell):

```bash
anc() { curl -sS -H "Authorization: Bearer $ANCHORED_OSS_TOKEN" "$@"; }
```

`jq` is used to parse responses. If `jq` is unavailable, drop the `| jq ...`
pipe and read the raw JSON.

## Scopes

A token has one scope:

- `admin` — full access, including writes and admin-only endpoints.
- `sync` — read + write memories on accessible projects.
- `readonly` — read only; any write (`POST /v1/memories`) returns **403**.

Call `/v1/me` first to learn the scope, then respect it — do not attempt writes
on a `readonly` token.

## Playbook

### 1. Whoami (always call first)

Discovers `account_id`, `org_id`, `org_slug`, `scope`, `email`.

```bash
anc "$ANCHORED_OSS_URL/v1/me" | jq '{account_id, org_id, org_slug, scope, email}'
```

- `401` → token is missing/invalid/expired/revoked; stop and ask the user to
  re-check `ANCHORED_OSS_TOKEN`.

### 2. List accessible projects

Returns a JSON **array** of projects the token can see.

```bash
anc "$ANCHORED_OSS_URL/v1/projects" | jq '[.[] | {id, name, slug, remote_key}]'
```

### 3. Resolve a project by name → id

Project-scoped endpoints need the project **UUID** (`id`), not the name/slug.

```bash
PROJECT_NAME="my-service"
PROJECT_ID=$(anc "$ANCHORED_OSS_URL/v1/projects" \
  | jq -r --arg n "$PROJECT_NAME" \
      '.[] | select(.name == $n or .slug == $n) | .id' | head -n1)
echo "resolved: $PROJECT_ID"
```

If empty, list projects (step 2) and pick the right `id`. A wrong/unknown id
yields **404** on the project endpoints.

### 4. Search memories (semantic, with text fallback)

`GET /v1/memories/search?project_id=&q=&limit=&mode=semantic|text`. Prefer
`semantic`; if the server has no embedder or its index is rebuilding it returns
**422** with `{"error":{"code":"semantic_unavailable", ...}}` — fall back to
`mode=text`. Results are an array; each item carries the full memory plus
`rank` and `effective_mode`.

```bash
search() {
  local q="$1" limit="${2:-10}"
  local url="$ANCHORED_OSS_URL/v1/memories/search"
  # Try semantic first.
  local body
  body=$(anc --get "$url" \
    --data-urlencode "project_id=$PROJECT_ID" \
    --data-urlencode "q=$q" \
    --data-urlencode "limit=$limit" \
    --data-urlencode "mode=semantic")
  if echo "$body" | jq -e '.error.code == "semantic_unavailable"' >/dev/null 2>&1; then
    # Fall back to text search.
    body=$(anc --get "$url" \
      --data-urlencode "project_id=$PROJECT_ID" \
      --data-urlencode "q=$q" \
      --data-urlencode "limit=$limit" \
      --data-urlencode "mode=text")
  fi
  echo "$body"
}

search "how do we handle auth" 10 \
  | jq '[.[] | {rank, effective_mode, category, content}]'
```

### 5. List a project's memories (paginated)

`GET /v1/projects/{id}/memories?limit=&offset=&category=`. Response is
`{memories, total, limit, offset}`. Optional `category` filter ∈
{fact, decision, plan, summary, learning}.

```bash
anc --get "$ANCHORED_OSS_URL/v1/projects/$PROJECT_ID/memories" \
  --data-urlencode "limit=20" \
  --data-urlencode "offset=0" \
  | jq '{total, count: (.memories | length),
         items: [.memories[] | {category, content, author_name, created_at}]}'
```

### 6. Save a memory (respect scope; use idempotency)

`POST /v1/memories` with `{project_id, category, content, keywords?, source?}`.
`category` must be one of **fact, decision, plan, summary, learning**
(note: `event` and `preference` are rejected by the server). A `readonly` token
gets **403** — do not retry, tell the user their token can't write.

Pass an `Idempotency-Key` header so a retry can't create a duplicate.

```bash
IDEM="save-$(date +%s)-$RANDOM"   # any stable, unique string ≤128 safe chars
anc -X POST "$ANCHORED_OSS_URL/v1/memories" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM" \
  -d @- <<JSON | jq '{created, id: .id, category, content}'
{
  "project_id": "$PROJECT_ID",
  "category": "decision",
  "content": "We standardized on Postgres for the billing service.",
  "keywords": ["postgres", "billing"],
  "source": "claude-code"
}
JSON
```

Note: the heredoc above does not expand `$PROJECT_ID`. Build the JSON with a
tool that interpolates safely, e.g. `jq`:

```bash
jq -n --arg pid "$PROJECT_ID" \
      --arg cat "decision" \
      --arg content "We standardized on Postgres for the billing service." \
      '{project_id:$pid, category:$cat, content:$content,
        keywords:["postgres","billing"], source:"claude-code"}' \
  | anc -X POST "$ANCHORED_OSS_URL/v1/memories" \
        -H "Content-Type: application/json" \
        -H "Idempotency-Key: $IDEM" \
        -d @- \
  | jq '{created, id, category}'
```

A replayed idempotent write returns the original memory with header
`Idempotency-Replayed: true` and `"created": false`.

### 7. Optional: RAG chat over team memories

Only when the server has chat enabled and the project index is ready. Check
status first: `index_state` must be `ready`.

```bash
anc --get "$ANCHORED_OSS_URL/v1/chat/status" \
  --data-urlencode "project_id=$PROJECT_ID" \
  | jq '{enabled, model, index_state}'
```

If `enabled` is true and `index_state == "ready"`, ask a question. Response is
`{answer, sources}`; sources cite the memories used (`[n]`).

```bash
jq -n --arg pid "$PROJECT_ID" --arg q "What database does billing use?" \
      '{project_id:$pid, query:$q}' \
  | anc -X POST "$ANCHORED_OSS_URL/v1/chat" \
        -H "Content-Type: application/json" -d @- \
  | jq '{answer, sources: [.sources[] | {category, snippet}]}'
```

- `503` → chat or embeddings not enabled on this server; skip chat, use search.
- `422 semantic_unavailable` → index still rebuilding; retry later or use search.

## Notes and gotchas

- There is **no** `GET /v1/memories/{id}`. Read a memory only via list (step 5)
  or search (step 4).
- Project endpoints require the project **UUID**; a slug/name won't match.
- Two error shapes exist: flat `{"error":"CODE"}` (e.g. `UNAUTHORIZED`,
  `FORBIDDEN`) and nested `{"error":{"code":"...","message":"..."}}` (e.g.
  `semantic_unavailable`, `idempotency_conflict`). Handle both.
- `search`/`list` limits are capped server-side (search max 100).
- Prefer semantic search; always handle the 422 → text fallback.
- Never write on a `readonly` token; never expose the token value.

## Troubleshooting

| Symptom | Meaning | What to do |
| --- | --- | --- |
| `401` `{"error":"UNAUTHORIZED"}` | Token missing, invalid, expired, or revoked | Verify `ANCHORED_OSS_TOKEN`; mint a fresh key in the dashboard → API Keys |
| `403` `{"error":"FORBIDDEN"}` on save | `readonly` scope, or no team access to the project | Don't retry; use a `sync`/`admin` token or request access |
| `422` `{"error":{"code":"semantic_unavailable"}}` | No embedder configured or index rebuilding | Retry with `mode=text`; for chat, retry later |
| `404` `{"error":"project not found"}` | Wrong project id, or not accessible to this token | Re-resolve id via `/v1/projects` |
| `400` `invalid category ...` | Category not in {fact, decision, plan, summary, learning} | Use an allowed category (no `event`/`preference`) |
| `503` on chat | Chat/embeddings disabled on server | Skip chat; use `/v1/memories/search` |
| `curl: (7) Connection refused` | Server down or wrong `ANCHORED_OSS_URL` | Check URL/port; confirm the server is reachable |
