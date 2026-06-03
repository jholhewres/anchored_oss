package store

import (
	"database/sql"
	"fmt"
)

const sqliteSchemaVersion = 13

const sqliteMigration001 = `
CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    UNIQUE(org_id, slug)
);

CREATE TABLE IF NOT EXISTS org_members (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TEXT DEFAULT (datetime('now')),
    UNIQUE(org_id, account_id)
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (team_id, account_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    remote_key TEXT NOT NULL,
    created_by TEXT REFERENCES accounts(id),
    created_at TEXT DEFAULT (datetime('now')),
    UNIQUE(org_id, slug),
    UNIQUE(org_id, remote_key)
);

CREATE TABLE IF NOT EXISTS team_project_access (
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'writer',
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (team_id, project_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'sync',
    expires_at TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    keywords TEXT,
    source TEXT,
    author_id TEXT REFERENCES accounts(id),
    author_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    metadata TEXT
);

CREATE INDEX IF NOT EXISTS idx_memories_project_updated ON memories(project_id, updated_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_content_hash_project ON memories(content_hash, project_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    actor_id TEXT REFERENCES accounts(id),
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    metadata TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);
`

const sqliteMigration002 = `
CREATE INDEX IF NOT EXISTS idx_audit_org_created ON audit_log(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_project_created ON audit_log(project_id, created_at DESC) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_actor_created ON audit_log(actor_id, created_at DESC) WHERE actor_id IS NOT NULL;
`

const sqliteMigration003 = `
DROP INDEX IF EXISTS idx_memories_content_hash_project;
CREATE INDEX IF NOT EXISTS idx_memories_content_hash_project ON memories(content_hash, project_id) WHERE deleted_at IS NULL;
`

const sqliteMigration004 = `
ALTER TABLE projects ADD COLUMN deleted_at TEXT;
CREATE INDEX IF NOT EXISTS idx_projects_org_active ON projects(org_id) WHERE deleted_at IS NULL;
`

const sqliteMigration005 = ``

// sqliteMigration006 mirrors Postgres migration006 (basic KG tables).
const sqliteMigration006 = `
CREATE TABLE IF NOT EXISTS kg_entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_entities_name_project ON kg_entities(name, project_id);

CREATE TABLE IF NOT EXISTS kg_predicates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    is_functional INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS kg_triples (
    id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    predicate_id TEXT NOT NULL REFERENCES kg_predicates(id) ON DELETE CASCADE,
    object_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    confidence REAL NOT NULL DEFAULT 1.0,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT (datetime('now')),
    valid_from TEXT NOT NULL DEFAULT (datetime('now')),
    valid_to TEXT
);
CREATE INDEX IF NOT EXISTS idx_kg_triples_project ON kg_triples(project_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_kg_triples_subject ON kg_triples(subject_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_kg_triples_object ON kg_triples(object_id) WHERE valid_to IS NULL;
`

// sqliteMigration007 adds aliases + logical unique constraint.
const sqliteMigration007 = `
CREATE TABLE IF NOT EXISTS kg_entity_aliases (
    entity_id TEXT NOT NULL REFERENCES kg_entities(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    PRIMARY KEY (entity_id, alias)
);
CREATE INDEX IF NOT EXISTS idx_kg_entity_aliases_alias ON kg_entity_aliases(alias);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kg_triples_logical
    ON kg_triples(subject_id, predicate_id, object_id, project_id)
    WHERE valid_to IS NULL;
`

// sqliteMigration008 mirrors Postgres 008: project.category.
const sqliteMigration008 = ``

// sqliteMigration009 mirrors Postgres 009: invites table.
const sqliteMigration009 = `
CREATE TABLE IF NOT EXISTS invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'sync',
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    accepted_at TEXT,
    created_by TEXT REFERENCES accounts(id),
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_invites_org_pending ON invites(org_id) WHERE accepted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_invites_email ON invites(email);
`

// sqliteMigration010 mirrors Postgres 010: curation_queue.
const sqliteMigration010 = `
CREATE TABLE IF NOT EXISTS curation_queue (
    memory_id TEXT PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    scheduled_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_curation_queue_pending
    ON curation_queue(scheduled_at)
    WHERE status = 'pending';
`

// sqliteMigration011 mirrors Postgres 011 for the embeddings columns. SQLite
// has no pgvector, so the vector is stored as a JSON-encoded float array and
// similarity search is brute-forced in Go (acceptable for single-node dev).
const sqliteMigration011 = `
ALTER TABLE memories ADD COLUMN embedding TEXT;
ALTER TABLE memories ADD COLUMN embed_model TEXT;
ALTER TABLE memories ADD COLUMN embed_dims INTEGER;
`

// sqliteMigration012 mirrors Postgres 012: per-org guardrail overrides.
// blocked_categories is a JSON array (no array type in SQLite).
const sqliteMigration012 = `
CREATE TABLE IF NOT EXISTS org_policies (
    org_id TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    blocked_categories TEXT NOT NULL DEFAULT '[]',
    quality_threshold REAL NOT NULL DEFAULT 0.55,
    near_dup_threshold REAL NOT NULL DEFAULT 0.85,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`

// sqliteMigration013 mirrors Postgres 013: the per-org guardrail manager.
const sqliteMigration013 = `
CREATE TABLE IF NOT EXISTS org_guardrails (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    builtin INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_org_guardrails_org ON org_guardrails(org_id);
`

var sqliteMigrations = map[int]string{
	1:  sqliteMigration001,
	2:  sqliteMigration002,
	3:  sqliteMigration003,
	4:  sqliteMigration004,
	5:  sqliteMigration005,
	6:  sqliteMigration006,
	7:  sqliteMigration007,
	8:  sqliteMigration008,
	9:  sqliteMigration009,
	10: sqliteMigration010,
	11: sqliteMigration011,
	12: sqliteMigration012,
	13: sqliteMigration013,
}

func columnExists(db *sql.DB, table, column string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column).Scan(&count)
	return count > 0
}

func MigrateSQLite(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for v := current + 1; v <= sqliteSchemaVersion; v++ {
		sqlText, ok := sqliteMigrations[v]
		if !ok {
			return fmt.Errorf("migration %d not found", v)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", v, err)
		}

		if sqlText != "" {
			if _, err := tx.Exec(sqlText); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("exec migration %d: %w", v, err)
			}
		}

		// Handle ALTER TABLE ADD COLUMN for migration 5.
		if v == 5 && !columnExists(db, "accounts", "password_hash") {
			if _, err := tx.Exec(`ALTER TABLE accounts ADD COLUMN password_hash TEXT`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d add password_hash: %w", v, err)
			}
		}

		// Handle ALTER TABLE ADD COLUMN for migration 8 (project.category).
		if v == 8 && !columnExists(db, "projects", "category") {
			if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN category TEXT NOT NULL DEFAULT 'other'`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d add category: %w", v, err)
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_projects_org_category ON projects(org_id, category) WHERE deleted_at IS NULL`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d index category: %w", v, err)
			}
		}

		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, v); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", v, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", v, err)
		}
	}

	return nil
}
