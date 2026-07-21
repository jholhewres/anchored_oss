package store

import (
	"database/sql"
	"fmt"
)

const sqliteSchemaVersion = 21

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

// sqliteMigration014 mirrors Postgres 014: repo_url + remote_key_v1 on
// projects. The ADD COLUMNs and the backfill UPDATE run via columnExists guards
// in MigrateSQLite (modernc SQLite has no "ADD COLUMN IF NOT EXISTS"), so this
// SQL block is intentionally empty — see the v == 14 branch below.
const sqliteMigration014 = ``

// sqliteMigration015 mirrors Postgres 015: retroactively park slug/remote_key
// of rows soft-deleted before v0.4.7 so their identity is freed for reuse.
// SQLite ids are already TEXT, so no cast is needed.
const sqliteMigration015 = `
UPDATE projects SET
    slug = slug || '-deleted-' || substr(id, 1, 8),
    remote_key = 'deleted-' || id,
    remote_key_v1 = NULL,
    repo_url = NULL
WHERE deleted_at IS NOT NULL
  AND remote_key NOT LIKE 'deleted-%';
`

// sqliteMigration016 mirrors Postgres 016: per-day sync rejection counters for
// the memory health dashboard. SQLite ids are TEXT; same shape otherwise.
const sqliteMigration016 = `
CREATE TABLE IF NOT EXISTS sync_rejection_stats (
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    rule TEXT NOT NULL,
    day TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, project_id, rule, day)
);
CREATE INDEX IF NOT EXISTS idx_sync_rejection_org_day ON sync_rejection_stats(org_id, day);
`

// sqliteMigration017 mirrors Postgres 017: per-org push batch cap. modernc
// SQLite has no ADD COLUMN IF NOT EXISTS, so the ALTER runs via a columnExists
// guard in the v == 17 branch below; this SQL block is intentionally empty.
const sqliteMigration017 = ``

// sqliteMigration018 mirrors Postgres 018 (per-account task threads).
const sqliteMigration018 = `
CREATE TABLE IF NOT EXISTS account_task_threads (
    account_id TEXT NOT NULL,
    task_key TEXT NOT NULL,
    external_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    projects TEXT NOT NULL DEFAULT '[]',
    journal TEXT NOT NULL DEFAULT '[]',
    details TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, task_key)
);
CREATE INDEX IF NOT EXISTS idx_account_task_threads_status ON account_task_threads(account_id, status);
`

// sqliteMigration019 mirrors Postgres 019: caller-scoped memory write
// idempotency with the original successful response preserved for replay.
const sqliteMigration019 = `
CREATE TABLE IF NOT EXISTS memory_write_idempotency (
    org_scope TEXT NOT NULL,
    actor_scope TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    response_json TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (org_scope, actor_scope, operation_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_write_idempotency_created_at
    ON memory_write_idempotency(created_at);
CREATE INDEX IF NOT EXISTS idx_memory_write_idempotency_memory_id
    ON memory_write_idempotency(memory_id);
`

// sqliteMigration020 adds complete semantic-space identity. The column is
// added through the guarded migration branch below for legacy databases.
const sqliteMigration020 = ``

// sqliteMigration021 mirrors Postgres 021: curation-queue lease/owner columns.
// The columns are added through the guarded migration branch below (SQLite has
// no ADD COLUMN IF NOT EXISTS); the index and the one-time orphan reset run here.
const sqliteMigration021 = ``

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
	14: sqliteMigration014,
	15: sqliteMigration015,
	16: sqliteMigration016,
	17: sqliteMigration017,
	18: sqliteMigration018,
	19: sqliteMigration019,
	20: sqliteMigration020,
	21: sqliteMigration021,
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

		// Handle ALTER TABLE ADD COLUMN for migration 14 (repo_url + remote_key_v1).
		// Columns must exist before the backfill UPDATE references remote_key_v1.
		if v == 14 {
			if !columnExists(db, "projects", "repo_url") {
				if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN repo_url TEXT`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add repo_url: %w", v, err)
				}
			}
			if !columnExists(db, "projects", "remote_key_v1") {
				if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN remote_key_v1 TEXT`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add remote_key_v1: %w", v, err)
				}
			}
			if _, err := tx.Exec(`UPDATE projects SET remote_key_v1 = remote_key
				WHERE remote_key_v1 IS NULL AND remote_key IS NOT NULL AND remote_key != ''`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d backfill remote_key_v1: %w", v, err)
			}
		}

		// Handle ALTER TABLE ADD COLUMN for migration 17 (org push batch cap).
		if v == 17 && !columnExists(db, "org_policies", "max_memories_per_sync") {
			if _, err := tx.Exec(`ALTER TABLE org_policies ADD COLUMN max_memories_per_sync INTEGER NOT NULL DEFAULT 500`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d add max_memories_per_sync: %w", v, err)
			}
		}

		if v == 20 {
			if !columnExists(db, "memories", "semantic_space_id") {
				if _, err := tx.Exec(`ALTER TABLE memories ADD COLUMN semantic_space_id TEXT`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add semantic_space_id: %w", v, err)
				}
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_semantic_space
				ON memories(project_id, semantic_space_id)
				WHERE embedding IS NOT NULL AND deleted_at IS NULL`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d index semantic_space_id: %w", v, err)
			}
			// Databases that already ran the development version of migration
			// 019 still need the deletion-lookup index. Keep migration 020's
			// SQL text empty because its ALTER TABLE must remain guarded.
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_write_idempotency_memory_id
				ON memory_write_idempotency(memory_id)`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d index memory idempotency memory_id: %w", v, err)
			}
		}

		// Handle ADD COLUMN + orphan reset for migration 21 (curation lease/owner).
		if v == 21 {
			if !columnExists(db, "curation_queue", "owner_id") {
				if _, err := tx.Exec(`ALTER TABLE curation_queue ADD COLUMN owner_id TEXT`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add owner_id: %w", v, err)
				}
			}
			if !columnExists(db, "curation_queue", "lease_expires_at") {
				if _, err := tx.Exec(`ALTER TABLE curation_queue ADD COLUMN lease_expires_at TEXT`); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d add lease_expires_at: %w", v, err)
				}
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_curation_queue_lease
				ON curation_queue(lease_expires_at)
				WHERE status IN ('processing', 'processing_dirty')`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d index lease: %w", v, err)
			}
			// One-time recovery of rows stranded in processing before leasing
			// existed. Safe on a fresh process: migrations run single-writer.
			if _, err := tx.Exec(`UPDATE curation_queue
				SET status = 'pending', owner_id = NULL, lease_expires_at = NULL
				WHERE status IN ('processing', 'processing_dirty')`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d reset orphans: %w", v, err)
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
