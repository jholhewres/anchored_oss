# anchored-teams (Claude Code Skill)

A distributable [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
Skill that lets an agent access a **self-hosted Anchored OSS (Anchored Teams)**
server directly over its HTTP API, using your **personal API token** — no MCP
server required. It can search and save team memories, list projects and their
memories, and (when enabled) query the RAG chat endpoint.

This is **complementary** to the local `anchored` MCP memory: that one is your
personal cross-tool memory; this skill targets a shared Teams server over HTTP.

## Install

Copy the `anchored-teams/` folder into a skills directory:

- **User-wide** (all projects):
  ```bash
  cp -r skill/anchored-teams ~/.claude/skills/anchored-teams
  ```
- **Project-local** (this repo only):
  ```bash
  mkdir -p .claude/skills
  cp -r skill/anchored-teams .claude/skills/anchored-teams
  ```

The folder must contain `SKILL.md`. Claude Code auto-discovers it on next start.

## Configure

The skill reads two environment variables:

| Variable | Meaning | Example |
| --- | --- | --- |
| `ANCHORED_OSS_URL` | Base URL of your Anchored OSS server | `https://memory.your-domain.com` |
| `ANCHORED_OSS_TOKEN` | Your personal API token (`anc_live_...`) | `anc_live_ab12...` |

Set them in your shell (or a profile / `.env`):

```bash
export ANCHORED_OSS_URL="http://your-server:8771"
export ANCHORED_OSS_TOKEN="anc_live_..."
```

### Getting a token

1. Open the server dashboard (`$ANCHORED_OSS_URL`) in a browser and sign in.
2. Go to **API Keys** and create a key.
3. Copy the `anc_live_...` value (shown once) into `ANCHORED_OSS_TOKEN`.

Tokens carry a scope: `admin`, `sync` (read + write), or `readonly` (read
only). Minting tokens via the API is admin-only, so use the dashboard.

## Use

Once installed and configured, just ask the agent to work with your team
memory, e.g.:

- "Search the anchored teams server for how we handle auth."
- "Save this decision to the billing project on the anchored oss server."
- "List projects on the anchored teams server."

The agent follows `SKILL.md`: it checks `/v1/me`, resolves the project id,
prefers semantic search (falling back to text), and respects your token scope.
It never prints your token.

## Requirements

- `curl` (required)
- `jq` (recommended, for parsing responses)
