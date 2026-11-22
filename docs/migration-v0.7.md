# Upgrading to Anchored OSS v0.7

This guide is for operators upgrading a v0.6.x server and for authors of
clients that call its API. It lists what changes on upgrade and what you may
need to do. Released clients (0.19.x and 0.20.x) keep working against v0.7;
the exceptions below say so.

## Before you upgrade

- **Back up the database.** Migrations 022 to 025 run on first start, on both
  Postgres and SQLite.
- **Postgres needs pgvector.** The Compose file now runs
  `pgvector/pgvector:pg16`. A plain `postgres:16` image fails migration 011.
- **Behind a reverse proxy?** Set `rate_limit.trusted_proxies` (or
  `RATE_LIMIT_TRUSTED_PROXIES`) to the address your proxy connects from. See
  [Rate limiting](#rate-limiting-and-x-forwarded-for).

## What every dashboard user notices

- **Everyone signs in once more.** Migration 025 revokes every key named
  `session`: the old login credential was an admin-scope key with no
  expiry. The next sign-in issues a session scoped to the account's role
  (`admin` → `admin`, `readonly` → `readonly`, any other role → `sync`).
- **Role changes take effect at once.** Demoting or removing an account
  revokes its login session and every key above its new role. Renaming an
  account, or saving it with the role it already has, revokes nothing.
- **A key never outranks its account.** A key whose scope is above its
  account's current role, or whose account is no longer a member of the
  key's org, is refused (`401`) and revoked on first use. Such keys can only
  exist on a v0.6 database (keys minted before a demotion that did not
  revoke them). Admins can no longer mint them: `POST /v1/api-keys` answers
  `400` for a scope above the target account's role.

## API changes

Unless marked otherwise, each change is additive or narrows access that
should never have been granted.

### Projects and org isolation

- **Another org's project is `404`, never `403`.** Every route that names a
  project (`/v1/projects/{id}/…`, `POST /v1/memories`,
  `GET /v1/memories/search`, `/v1/chat*`) answers `404 project not found`,
  and sync answers `404 PROJECT_NOT_FOUND`, the same as for a project that
  does not exist. Before, an admin key of any org reached another org's
  project by id. Clients that branched on `403 project belongs to a
  different organization` now get `404`.
- **Deleted projects are gone for every scope**, admin included, on the HTTP
  routes and on sync (`PROJECT_NOT_FOUND`).
- **`POST /v1/memories` needs team access** for non-admin keys, as sync
  already did: `403 no team access to this project`. A `project_claim` that
  names an existing project takes the same check.
- **Creating a project by claim.** A non-admin key creates a project from a
  `project_claim` only when its account is on the org's default team;
  otherwise `403`. The new project is granted to the default team.
- **`POST /v1/projects` with a `repo_url` the org already has answers `200`
  with the existing project**, as in v0.6.x. A `repo_url` must be a valid
  HTTPS or SSH git remote (`400` otherwise); `remote_key` alone is still
  accepted for released clients.
- **Teams and invites stay in their org.** `GET /v1/teams/{id}`,
  `POST /v1/teams/{id}/members`, `DELETE /v1/teams/{id}/members/{account_id}`
  and `DELETE /v1/invites/{id}` answer `404` for another org's team or
  invite. A team only takes members of its own org. `GET /v1/teams/{id}` is
  now admin-only (`403` for other keys): it lists members' emails and every
  project the team grants.
- **Account administration stays in the org.** `PATCH`, `DELETE`,
  `GET …/projects` and `PUT …/projects` on `/v1/accounts/{id}` answer `404`
  for an account outside the caller's org; an unknown role is `400`.

### Keys and sessions

- **Personal API keys:** `GET`, `POST` and `DELETE /v1/me/api-keys`, usable
  only with a login session. Scopes `sync` and `readonly`, never above the
  session's own; every one expires (`expires_in` from `1d` to `365d`). Login sessions and admin keys
  never appear in, nor can be revoked through, these routes.
- The name `session` is reserved: no API path creates a key with it.
- **Still to change:** onboarding hands the CLI an admin key with no expiry,
  because released CLIs create projects with it through the admin-only
  `POST /v1/projects`. Handing out expiring personal keys instead is part of
  the v0.7 auth work.

### Onboarding

- `POST /v1/onboarding/complete` answers `409` as soon as an organization
  exists, before it reads the body.
- The body is capped at 16 KiB (`413`), a NUL character in any stored field
  is `400`, and every project needs a valid git remote.
- The route shares the stricter auth rate limit with login and register.

### Skills

New: the org skills catalog (`/v1/skills`, `/v1/skill-templates`, and
`/v1/projects/{id}/skills` for attachments and runtime loading), with
versions, frontmatter and built-in templates (migrations 022 to 024).

- A SKILL.md header may hold only `name` and `description`, matching the
  skill's slug and purpose. This holds for the `frontmatter` field and for a
  header inside the legacy `content` field: such content is stored
  canonically (body plus frontmatter); a bad header, an unclosed one, YAML
  anchors or aliases, or an opening line that is not exactly `---` is `400`.
- Header-less `content` still makes a raw legacy row, kept byte for byte.
- A legacy row whose stored header breaks these rules is withheld from
  `GET /v1/projects/{id}/skills/{slug}` (`404`, logged) until an admin saves
  it again.
- `PUT /v1/projects/{id}/skills` needs UUIDs in `skill_ids` (`400`).

## Rate limiting and X-Forwarded-For

v0.6 keyed the rate limiter on the first `X-Forwarded-For` hop, which any
client writes for itself, so a client could take a fresh bucket per request.
v0.7 reads `X-Forwarded-For` only from connections whose address is in
`rate_limit.trusted_proxies`, walking the header from the right past trusted
hops; every other client is limited by its connection's IP (an IPv6 client by
its /64).

**No proxy is trusted by default**, because the installer and the Compose
file expose the server directly. If a reverse proxy sits in front of the
server, list the address it connects from, as narrowly as you can:

```yaml
rate_limit:
  trusted_proxies: ["127.0.0.1"]   # a proxy on the same host
```

```bash
RATE_LIMIT_TRUSTED_PROXIES=172.20.0.10   # a proxy container
```

Without it, every client behind the proxy shares one bucket, and the auth
bucket (10 requests a minute, burst 5) is shared by every sign-in. The server
logs one warning when it sees `X-Forwarded-For` from a private address it does
not trust.

## Migrations

| Migration | What it does |
|---|---|
| 022 to 024 | Skills catalog: skills, versions, frontmatter and project attachments. 023 and 024 correct tables 022 creates; they stay separate so databases from pre-release builds upgrade. |
| 025 | Revokes every key named `session` (the old admin-scope login credential). Every dashboard user signs in once more. |

Migration 025 is broad on purpose: it revokes every `session` key, not only
those whose scope is above the account's role, because an older admin
session left in place would keep full admin rights.

## Optional cleanup

Before v0.7, an admin of one org could put an account of another org on its
own teams. v0.7 can no longer write such rows and never lists them, and they
grant nothing, but they stay in the database. To remove them:

```sql
DELETE FROM team_members tm
USING teams t
WHERE t.id = tm.team_id
  AND NOT EXISTS (
    SELECT 1 FROM org_members om
    WHERE om.org_id = t.org_id AND om.account_id = tm.account_id
  );
```

The same statement works on SQLite written as
`DELETE FROM team_members WHERE NOT EXISTS (SELECT 1 FROM teams t JOIN org_members om ON om.org_id = t.org_id WHERE t.id = team_members.team_id AND om.account_id = team_members.account_id);`.
