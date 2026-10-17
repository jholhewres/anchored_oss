# Glossary

Definitions only. Use these words in code, cards, tests and docs.

**Instance** — one running Anchored OSS server, with its own database, public
base URL and `instance_id`. It is both the OAuth issuer and the protected
resource.

**Org** — a tenant inside an instance (the UI may call it a *workspace*). It owns
projects, teams, skills, keys and grants. An account may belong to several orgs.

**Account** — a person who logs in on an instance.

**Project** — a repository inside an org, identified by its remote key.

**Remote key** — the identifier derived from a project's git origin. The same
origin on different machines resolves to the same project.

**Client** — `anchored`, the local-first CLI and MCP server on a developer
machine.

**Remote** — a client's configured connection to one instance.

**Memory** — a unit of saved knowledge, with a category and a memory scope.

**Category** — the kind of memory: fact, preference, decision, event, learning,
plan or summary.

**Memory scope** — who may see a memory: `user` (its owner only), `project`,
`team` or `org`. Not the same as a key scope or an OAuth scope.

**Tombstone** — the record that a memory was deleted. It travels in sync so the
delete wins over older copies.

**Skill** — an org-owned, versioned set of instructions served as a `SKILL.md`.

**Attachment** — the link that makes a skill available to a project.

**Capabilities** — the public document at `GET /v1/capabilities` that tells a
client what an instance supports.

**Remote MCP** — the instance's MCP endpoint, `/mcp`.

**Legacy key** — an `anc_live_` API key. Its key scope is `admin`, `sync` or
`readonly`.

**Personal key** — a legacy key an account creates for itself, with key scope
`sync` or `readonly` and an expiry.

**Web session** — a dashboard login, held in a cookie.

**Grant** — the authorization an account gives an OAuth client for one org, a
set of projects and a set of OAuth scopes. The *Connections* screen lists and
revokes grants.

**OAuth scope** — what a grant allows: `memories:read`, `memories:write`,
`skills:read` or `admin`.
