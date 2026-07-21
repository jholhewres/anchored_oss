package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/lib/pq"
)

// memoryUpsertCols counts the columns inserted per row by UpsertMemories.
const memoryUpsertCols = 12

// memoryBatchSize splits an UpsertMemories call into chunks that stay
// below Postgres' 65535-parameter limit (12 cols * 5000 rows = 60000).
const memoryBatchSize = 1000

// qualityFilterSQL hides low-signal / mis-categorized rows from read paths.
// Threshold mirrors policy.RemoteQualityThreshold so write and read stay in
// sync. The `pinned=true` escape hatch lets clients keep a memory visible
// even if its score is low.
var qualityFilterSQL = fmt.Sprintf(`
		   AND (
		     metadata IS NULL
		     OR jsonb_typeof(metadata->'quality_score') IS DISTINCT FROM 'number'
		     OR (metadata->>'quality_score')::double precision >= %f
		     OR (metadata->>'pinned')::boolean IS TRUE
		   )
		   AND (metadata IS NULL OR metadata->>'scope' IS DISTINCT FROM 'user')
		   AND (
		     metadata IS NULL
		     OR metadata->>'memory_type' IS DISTINCT FROM 'operational'
		     OR metadata->>'kind' = 'handoff'
		   )
		   AND (metadata IS NULL OR metadata->>'origin' IS DISTINCT FROM 'precompact')
		   AND (metadata IS NULL OR metadata->>'origin' IS DISTINCT FROM 'handoff')`,
	policy.RemoteQualityThreshold,
)

func (s *PostgresStore) SearchMemories(ctx context.Context, projectID string, query string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Term-wise matching with relevance ranking — see the SQLite twin for the
	// rationale (whole-query substrings return nothing for natural queries).
	terms := searchTerms(query)
	escaped := strings.ReplaceAll(query, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "%", `\%`)
	escaped = strings.ReplaceAll(escaped, "_", `\_`)
	if len(terms) == 0 {
		terms = []string{escaped}
	}

	args := []any{projectID}
	ph := func(v string) string {
		args = append(args, "%"+v+"%")
		return fmt.Sprintf("$%d", len(args))
	}
	var where, rank []string
	for _, t := range terms {
		p := ph(t)
		where = append(where, fmt.Sprintf("(content ILIKE %s OR keywords::text ILIKE %s)", p, p))
	}
	phrasePH := ph(escaped)
	rank = append(rank, fmt.Sprintf("(CASE WHEN content ILIKE %s OR keywords::text ILIKE %s THEN 100 ELSE 0 END)", phrasePH, phrasePH))
	for _, t := range terms {
		p := ph(t)
		rank = append(rank, fmt.Sprintf("(CASE WHEN content ILIKE %s OR keywords::text ILIKE %s THEN 1 ELSE 0 END)", p, p))
	}
	args = append(args, limit)
	limitPH := fmt.Sprintf("$%d", len(args))

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL`+qualityFilterSQL+`
		   AND (`+strings.Join(where, " OR ")+`)
		 ORDER BY `+strings.Join(rank, " + ")+` DESC, updated_at DESC
		 LIMIT `+limitPH,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

func (s *PostgresStore) UpsertMemory(ctx context.Context, m *model.Memory) error {
	if _, err := postgresUpsertMemoryResult(ctx, s.db, m); err != nil {
		return err
	}
	if err := s.EnqueueCuration(ctx, []string{m.ID}); err != nil {
		slog.Default().Warn("enqueue curation failed", "memory_id", m.ID, "error", err)
	}
	return nil
}

func (s *PostgresStore) UpsertMemoryWithStatus(ctx context.Context, m *model.Memory) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin memory upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	created, err := postgresUpsertMemoryStatusResult(ctx, tx, m)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit memory upsert: %w", err)
	}
	if err := s.EnqueueCuration(ctx, []string{m.ID}); err != nil {
		slog.Default().Warn("enqueue curation failed", "memory_id", m.ID, "error", err)
	}
	return created, nil
}

func postgresUpsertMemoryStatusResult(ctx context.Context, execer memoryQuerierExecer, m *model.Memory) (bool, error) {
	var metadataBytes []byte
	if m.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(m.Metadata)
		if err != nil {
			return false, fmt.Errorf("marshal memory metadata: %w", err)
		}
	}
	result, err := execer.ExecContext(ctx,
		`INSERT INTO memories
		   (id, project_id, category, content, content_hash, keywords, source,
		    author_id, author_name, created_at, updated_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (id) DO NOTHING`,
		m.ID,
		m.ProjectID,
		m.Category,
		m.Content,
		m.ContentHash,
		pq.Array(m.Keywords),
		m.Source,
		nilIfEmpty(m.AuthorID),
		m.AuthorName,
		m.CreatedAt,
		m.UpdatedAt,
		metadataBytes,
	)
	if err != nil {
		return false, fmt.Errorf("insert memory: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect memory insert result: %w", err)
	}
	if inserted == 1 {
		return true, nil
	}
	if inserted != 0 {
		return false, fmt.Errorf("insert memory affected %d rows, want 0 or 1", inserted)
	}
	if _, err := postgresUpsertMemoryResult(ctx, execer, m); err != nil {
		return false, err
	}
	return false, nil
}

func postgresUpsertMemoryResult(ctx context.Context, execer memoryQuerierExecer, m *model.Memory) (int64, error) {
	var metadataBytes []byte
	if m.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(m.Metadata)
		if err != nil {
			return 0, fmt.Errorf("marshal memory metadata: %w", err)
		}
	}

	// Last-write-wins by (id): editing a memory keeps the same id but may
	// change content/hash. migration003 dropped the UNIQUE constraint on
	// (content_hash, project_id) and replaced it with a plain index kept for
	// lookups only — content-level dupes under different ids are legal; dedup
	// and quality filtering are handled asynchronously by the curation worker.
	result, err := execer.ExecContext(ctx,
		`INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (id) DO UPDATE SET
		   project_id = EXCLUDED.project_id,
		   category = EXCLUDED.category,
		   content = EXCLUDED.content,
		   content_hash = EXCLUDED.content_hash,
		   keywords = EXCLUDED.keywords,
		   source = EXCLUDED.source,
		   author_id = EXCLUDED.author_id,
		   author_name = EXCLUDED.author_name,
		   updated_at = EXCLUDED.updated_at,
		   metadata = EXCLUDED.metadata,
		   embedding = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embedding END,
		   embed_model = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embed_model END,
		   embed_dims = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embed_dims END,
		   semantic_space_id = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.semantic_space_id END,
		   deleted_at = NULL
		 WHERE memories.project_id IS NOT DISTINCT FROM EXCLUDED.project_id
		   AND memories.updated_at <= EXCLUDED.updated_at`,
		m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
		pq.Array(m.Keywords), m.Source, m.AuthorID, m.AuthorName,
		m.CreatedAt, m.UpdatedAt, metadataBytes,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert memory: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect upsert memory result: %w", err)
	}
	if affected == 0 {
		var existingProjectID string
		err := execer.QueryRowContext(ctx,
			`SELECT project_id FROM memories WHERE id = $1`,
			m.ID,
		).Scan(&existingProjectID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("inspect existing memory project: %w", err)
		}
		if err == nil && existingProjectID != m.ProjectID {
			return 0, fmt.Errorf(
				"%w: memory ID %s belongs to another project",
				ErrConflict,
				m.ID,
			)
		}
	}
	return affected, nil
}

// UpsertMemories runs a single batched INSERT ... ON CONFLICT (id) DO UPDATE
// for every memory in ms. Internally chunks by memoryBatchSize so we stay
// under the Postgres parameter limit.
func (s *PostgresStore) UpsertMemories(ctx context.Context, ms []*model.Memory) error {
	if len(ms) == 0 {
		return nil
	}
	for start := 0; start < len(ms); start += memoryBatchSize {
		end := start + memoryBatchSize
		if end > len(ms) {
			end = len(ms)
		}
		if err := s.upsertMemoriesChunk(ctx, ms[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) upsertMemoriesChunk(ctx context.Context, ms []*model.Memory) error {
	if err := validateRequestedMemoryProjects(ms); err != nil {
		return err
	}
	args := make([]any, 0, memoryUpsertCols*len(ms))
	placeholders := make([]string, 0, len(ms))
	for i, m := range ms {
		var metadataBytes []byte
		if m.Metadata != nil {
			b, err := json.Marshal(m.Metadata)
			if err != nil {
				return fmt.Errorf("marshal memory metadata: %w", err)
			}
			metadataBytes = b
		}
		base := i * memoryUpsertCols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6,
			base+7, base+8, base+9, base+10, base+11, base+12,
		))
		args = append(args,
			m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
			pq.Array(m.Keywords), m.Source, nilIfEmpty(m.AuthorID), m.AuthorName,
			m.CreatedAt, m.UpdatedAt, metadataBytes,
		)
	}

	query := `INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (id) DO UPDATE SET
		   project_id = EXCLUDED.project_id,
		   category = EXCLUDED.category,
		   content = EXCLUDED.content,
		   content_hash = EXCLUDED.content_hash,
		   keywords = EXCLUDED.keywords,
		   source = EXCLUDED.source,
		   author_id = EXCLUDED.author_id,
		   author_name = EXCLUDED.author_name,
		   updated_at = EXCLUDED.updated_at,
		   metadata = EXCLUDED.metadata,
		   embedding = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embedding END,
		   embed_model = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embed_model END,
		   embed_dims = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.embed_dims END,
		   semantic_space_id = CASE
		     WHEN memories.content IS DISTINCT FROM EXCLUDED.content
		       OR memories.content_hash IS DISTINCT FROM EXCLUDED.content_hash
		     THEN NULL ELSE memories.semantic_space_id END,
		   deleted_at = NULL
		 WHERE memories.project_id IS NOT DISTINCT FROM EXCLUDED.project_id
		   AND memories.updated_at <= EXCLUDED.updated_at`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert memories batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("upsert memories batch: %w", err)
	}
	if err := validatePostgresStoredMemoryProjects(ctx, tx, ms); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert memories batch: %w", err)
	}
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	if err := s.EnqueueCuration(ctx, ids); err != nil {
		slog.Default().Warn("enqueue curation failed", "memory_count", len(ids), "error", err)
	}
	return nil
}

func validatePostgresStoredMemoryProjects(
	ctx context.Context,
	tx *sql.Tx,
	memories []*model.Memory,
) error {
	expected := make(map[string]string, len(memories))
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		expected[memory.ID] = memory.ProjectID
		ids = append(ids, memory.ID)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, project_id FROM memories WHERE id = ANY($1)`,
		pq.Array(ids),
	)
	if err != nil {
		return fmt.Errorf("validate memory batch projects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID string
		if err := rows.Scan(&id, &projectID); err != nil {
			return fmt.Errorf("scan memory batch project: %w", err)
		}
		if projectID != expected[id] {
			return fmt.Errorf(
				"%w: memory ID %s belongs to another project",
				ErrConflict,
				id,
			)
		}
	}
	return rows.Err()
}

func (s *PostgresStore) GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND updated_at > $2 AND deleted_at IS NULL`+qualityFilterSQL+`
		 ORDER BY updated_at`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get memories updated since: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

const (
	memoryDefaultLimit = 20
	memoryMaxLimit     = 200
)

// ListMemoriesPaginated returns a page of memories for a non-deleted project.
// Returns ErrNotFound when the project is missing or soft-deleted so handlers
// can map directly to 404.
func (s *PostgresStore) ListMemoriesPaginated(ctx context.Context, projectID string, limit, offset int, category string) ([]*model.Memory, int, error) {
	// Guard: project must be live. We piggyback on GetActiveProjectByID so the
	// caller cannot peek at a soft-deleted project's memories.
	if _, err := s.GetActiveProjectByID(ctx, projectID); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = memoryDefaultLimit
	}
	if limit > memoryMaxLimit {
		limit = memoryMaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	catFilter := ""
	args := []any{projectID}
	if category != "" {
		catFilter = ` AND category = $2`
		args = append(args, category)
	}
	args = append(args, limit, offset)

	limitIdx := len(args) - 1
	offsetIdx := len(args)

	query := fmt.Sprintf(
		`SELECT id, project_id, category, content, content_hash, keywords, source,
		        author_id, author_name, created_at, updated_at, deleted_at, metadata,
		        COUNT(*) OVER() AS total
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL%s`+qualityFilterSQL+`
		 ORDER BY updated_at DESC
		 LIMIT $%d OFFSET $%d`,
		catFilter, limitIdx, offsetIdx,
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list memories paginated: %w", err)
	}
	defer rows.Close()

	memories := make([]*model.Memory, 0, limit)
	total := 0
	for rows.Next() {
		var m model.Memory
		var metadataBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			pq.Array(&m.Keywords), &m.Source, scanNullString(&m.AuthorID), &m.AuthorName,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &metadataBytes, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan memory: %w", err)
		}
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &m.Metadata); err != nil {
				return nil, 0, fmt.Errorf("unmarshal memory metadata: %w", err)
			}
		}
		memories = append(memories, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate memories: %w", err)
	}
	return memories, total, nil
}

func (s *PostgresStore) SoftDeleteMemory(ctx context.Context, id, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin soft delete memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE memories SET deleted_at = now(), updated_at = now() WHERE id = $1 AND project_id = $2 AND deleted_at IS NULL`,
		id, projectID,
	)
	if err != nil {
		return fmt.Errorf("soft delete memory: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE memory_write_idempotency
		 SET response_json = jsonb_build_object(
		   'memory', jsonb_build_object('id', memory_id),
		   'created', false
		 )
		 WHERE memory_id = $1`,
		id,
	); err != nil {
		return fmt.Errorf("redact memory idempotency on soft delete: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit soft delete memory: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM memories WHERE project_id = $1 AND deleted_at IS NOT NULL AND updated_at > $2`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get tombstones since: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tombstone: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tombstones: %w", err)
	}
	return ids, nil
}

func scanMemories(rows *sql.Rows) ([]*model.Memory, error) {
	var memories []*model.Memory
	for rows.Next() {
		var m model.Memory
		var metadataBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			pq.Array(&m.Keywords), &m.Source, scanNullString(&m.AuthorID), &m.AuthorName,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &metadataBytes,
		); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &m.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal memory metadata: %w", err)
			}
		}
		memories = append(memories, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories: %w", err)
	}
	return memories, nil
}

// SoftDeleteMemoriesByWindow soft-deletes every live memory of a project whose
// created_at falls inside [since, until). Built for admin moderation: undoing
// a sync batch that landed in the wrong project. Returns the number of
// memories tombstoned (clients pick the deletions up via GetTombstonesSince).
func (s *PostgresStore) SoftDeleteMemoriesByWindow(ctx context.Context, projectID string, since, until *time.Time) (int64, error) {
	q := `UPDATE memories SET deleted_at = now(), updated_at = now()
	      WHERE project_id = $1 AND deleted_at IS NULL`
	args := []any{projectID}
	if since != nil {
		args = append(args, since.UTC())
		q += fmt.Sprintf(` AND created_at >= $%d`, len(args))
	}
	if until != nil {
		args = append(args, until.UTC())
		q += fmt.Sprintf(` AND created_at < $%d`, len(args))
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("soft delete memories by window: %w", err)
	}
	return res.RowsAffected()
}
