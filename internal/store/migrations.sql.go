package store

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 12

// advisoryLockKey is a constant 64-bit key used to serialize migrations
// across concurrent server instances on the same database.
const advisoryLockKey int64 = 0x416E63686F726564 // "Anchored" as bytes

const migration001 = `
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, slug)
);

CREATE TABLE IF NOT EXISTS org_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, account_id)
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (team_id, account_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    remote_key TEXT NOT NULL,
    created_by UUID REFERENCES accounts(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, slug),
    UNIQUE(org_id, remote_key)
);

CREATE TABLE IF NOT EXISTS team_project_access (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'writer',
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (team_id, project_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'sync',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    keywords TEXT[],
    source TEXT,
    author_id UUID REFERENCES accounts(id),
    author_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_memories_project_updated ON memories(project_id, updated_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_content_hash_project ON memories(content_hash, project_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES accounts(id),
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT now()
);
`

const migration002 = `
CREATE INDEX IF NOT EXISTS idx_audit_org_created ON audit_log(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_project_created ON audit_log(project_id, created_at DESC) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_actor_created ON audit_log(actor_id, created_at DESC) WHERE actor_id IS NOT NULL;
`

// migration003 relaxes the dedupe constraint on memories. Memories with the
// same content_hash but different ids are legal (clients may import similar
// content under multiple ids); we keep the index for lookups but drop the
// UNIQUE so batched upserts cannot fail on partial-index conflicts.
const migration003 = `
DROP INDEX IF EXISTS idx_memories_content_hash_project;
CREATE INDEX IF NOT EXISTS idx_memories_content_hash_project ON memories(content_hash, project_id) WHERE deleted_at IS NULL;
`

// migration004 introduces soft-delete on projects. The partial index keeps
// "active projects per org" lookups cheap and excludes archived rows from
// the dashboard list views.
const migration004 = `
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_projects_org_active ON projects(org_id) WHERE deleted_at IS NULL;
`

// migration005 adds password_hash to accounts for email/password login.
const migration005 = `
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS password_hash TEXT;
`

// migration006 introduces knowledge graph tables for entity-relationship storage.
const migration006 = `
CREATE TABLE IF NOT EXISTS kg_entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_entities_name_project ON kg_entities(name, project_id);

CREATE TABLE IF NOT EXISTS kg_predicates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS kg_triples (
    id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    predicate_id TEXT NOT NULL REFERENCES kg_predicates(id) ON DELETE CASCADE,
    object_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT now(),
    valid_to TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_kg_triples_project ON kg_triples(project_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_kg_triples_subject ON kg_triples(subject_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_kg_triples_object ON kg_triples(object_id) WHERE valid_to IS NULL;
`

// migration007 hardens the knowledge graph: aliases for fuzzy entity matching,
// is_functional predicates for temporal supersession, and a logical unique
// constraint so the same (subject, predicate, object) is not duplicated per
// project.
const migration007 = `
CREATE TABLE IF NOT EXISTS kg_entity_aliases (
    entity_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    PRIMARY KEY (entity_id, alias)
);
CREATE INDEX IF NOT EXISTS idx_kg_entity_aliases_alias ON kg_entity_aliases(alias);

ALTER TABLE kg_predicates ADD COLUMN IF NOT EXISTS is_functional BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE kg_triples ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_triples_logical
    ON kg_triples(subject_id, predicate_id, object_id, project_id)
    WHERE valid_to IS NULL;
`

// migration008 adds a project.category column for organizing the dashboard.
// Categories are validated at the application layer (service/library/app/
// infra/experiment/other). 'other' is a safe default for legacy rows.
const migration008 = `
ALTER TABLE projects ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';
CREATE INDEX IF NOT EXISTS idx_projects_org_category ON projects(org_id, category) WHERE deleted_at IS NULL;
`

// migration009 introduces invite tokens for inviting new developers. Tokens
// are stored hashed so the raw value never sits in the database; only the
// /invite/:token URL carries it. Expiry is enforced at the handler level.
const migration009 = `
CREATE TABLE IF NOT EXISTS invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'sync',
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_by UUID REFERENCES accounts(id),
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invites_org_pending ON invites(org_id) WHERE accepted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_invites_email ON invites(email);
`

// migration010 introduces the curation queue used by the async worker. Each
// new memory inserts one queue row that the worker picks up to compute
// quality_score, detect near-duplicates within the project, and update
// metadata. Status transitions: pending -> processing -> done|failed.
const migration010 = `
CREATE TABLE IF NOT EXISTS curation_queue (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_curation_queue_pending
    ON curation_queue(scheduled_at)
    WHERE status = 'pending';
`

// migration011 adds semantic-search embeddings. It enables the pgvector
// extension and stores one 384-dim vector per memory alongside the model name
// (so a provider/model change can be detected and reindexed). The HNSW index
// uses cosine ops to match the L2-normalized vectors the embedders emit.
//
// NOTE: CREATE EXTENSION requires the database role to have the vector
// extension available (run once by a superuser on managed Postgres). Self-host
// installs should provision pgvector before first start.
const migration011 = `
CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS embedding vector(384);
ALTER TABLE memories ADD COLUMN IF NOT EXISTS embed_model TEXT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS embed_dims INT;
CREATE INDEX IF NOT EXISTS idx_memories_embedding
    ON memories USING hnsw (embedding vector_cosine_ops);
`

// migration012 adds per-org guardrail overrides. A row is optional: absent
// means "use defaults". blocked_categories overrides the default event/preference
// set; thresholds override the curation/quality defaults. The secret and
// local-path guardrails are never configurable and live in code.
const migration012 = `
CREATE TABLE IF NOT EXISTS org_policies (
    org_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    blocked_categories TEXT[] NOT NULL DEFAULT '{}',
    quality_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.55,
    near_dup_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

var migrations = map[int]string{
	1:  migration001,
	2:  migration002,
	3:  migration003,
	4:  migration004,
	5:  migration005,
	6:  migration006,
	7:  migration007,
	8:  migration008,
	9:  migration009,
	10: migration010,
	11: migration011,
	12: migration012,
}

// Migrate brings the schema up to schemaVersion. Safe to call from
// multiple instances concurrently: a session-level advisory lock
// serializes execution per database.
func Migrate(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn for migration: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var current int
	row := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for v := current + 1; v <= schemaVersion; v++ {
		sqlText, ok := migrations[v]
		if !ok {
			return fmt.Errorf("migration %d not found", v)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", v, err)
		}

		if _, err := tx.Exec(sqlText); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec migration %d: %w", v, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES ($1)`, v); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", v, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", v, err)
		}
	}

	return nil
}
