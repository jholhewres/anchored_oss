package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpsertTriple stores a (subject, predicate, object) edge for a project.
//
// Behavior:
//   - Subject and object entities are auto-created on first use; alias lookup
//     (lowercase form) lets clients reference the same entity under different
//     casing without spawning duplicates.
//   - Predicates are project-agnostic and shared across the org.
//   - If the predicate is marked is_functional (e.g. "deployed_on", "owns"),
//     all existing live triples for (subject, predicate) are tombstoned by
//     setting valid_to=now() before inserting the new one. This is how the
//     graph evolves over time without losing history.
//   - The same logical triple (subject, predicate, object, project) is never
//     duplicated thanks to idx_kg_triples_logical; conflicting inserts return
//     the existing row's id.
//   - Confidence defaults to 1.0 when callers pass 0.
func (s *PostgresStore) UpsertTriple(ctx context.Context, t *model.Triple) error {
	if t.ProjectID == "" {
		return errors.New("UpsertTriple: project_id is required")
	}
	subj := strings.TrimSpace(t.Subject)
	pred := strings.TrimSpace(t.Predicate)
	obj := strings.TrimSpace(t.Object)
	if subj == "" || pred == "" || obj == "" {
		return errors.New("UpsertTriple: subject/predicate/object cannot be empty")
	}
	if t.Confidence == 0 {
		t.Confidence = 1.0
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	subjectID, err := pgEnsureEntity(ctx, tx, subj, t.ProjectID)
	if err != nil {
		return fmt.Errorf("ensure subject: %w", err)
	}
	predicateID, isFunctional, err := pgEnsurePredicate(ctx, tx, pred)
	if err != nil {
		return fmt.Errorf("ensure predicate: %w", err)
	}
	objectID, err := pgEnsureEntity(ctx, tx, obj, t.ProjectID)
	if err != nil {
		return fmt.Errorf("ensure object: %w", err)
	}

	if isFunctional {
		// Tombstone any prior live triple with same subject+predicate but a
		// different object. Same-object inserts are deduped via the unique
		// index below, so we explicitly preserve those.
		if _, err := tx.ExecContext(ctx,
			`UPDATE kg_triples SET valid_to = now()
			 WHERE subject_id = $1 AND predicate_id = $2 AND project_id = $3
			       AND object_id <> $4 AND valid_to IS NULL`,
			subjectID, predicateID, t.ProjectID, objectID,
		); err != nil {
			return fmt.Errorf("supersede functional triple: %w", err)
		}
	}

	id := newHexID()
	var insertedID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO kg_triples (id, subject_id, predicate_id, object_id, confidence, project_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (subject_id, predicate_id, object_id, project_id) WHERE valid_to IS NULL
		 DO NOTHING
		 RETURNING id`,
		id, subjectID, predicateID, objectID, t.Confidence, t.ProjectID,
	).Scan(&insertedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("insert triple: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		// Conflict — fetch the existing live triple's id.
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM kg_triples
			 WHERE subject_id = $1 AND predicate_id = $2 AND object_id = $3 AND project_id = $4
			   AND valid_to IS NULL`,
			subjectID, predicateID, objectID, t.ProjectID,
		).Scan(&insertedID); err != nil {
			return fmt.Errorf("fetch existing triple: %w", err)
		}
	}
	t.ID = insertedID

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListTriplesByProject(ctx context.Context, projectID string, limit, offset int) ([]*model.Triple, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, s.name, p.name, o.name, t.confidence, t.project_id, t.created_at,
		        COUNT(*) OVER() AS total
		 FROM kg_triples t
		 JOIN kg_entities s ON t.subject_id = s.id
		 JOIN kg_predicates p ON t.predicate_id = p.id
		 JOIN kg_entities o ON t.object_id = o.id
		 WHERE t.project_id = $1 AND t.valid_to IS NULL
		 ORDER BY t.created_at DESC
		 LIMIT $2 OFFSET $3`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list triples: %w", err)
	}
	defer rows.Close()

	triples := make([]*model.Triple, 0, limit)
	total := 0
	for rows.Next() {
		var t model.Triple
		if err := rows.Scan(&t.ID, &t.Subject, &t.Predicate, &t.Object, &t.Confidence, &t.ProjectID, &t.CreatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("scan triple: %w", err)
		}
		triples = append(triples, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate triples: %w", err)
	}
	return triples, total, nil
}

func (s *PostgresStore) CountTriplesByProject(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kg_triples WHERE project_id = $1 AND valid_to IS NULL`,
		projectID,
	).Scan(&count)
	return count, err
}

func pgEnsureEntity(ctx context.Context, tx *sql.Tx, name, projectID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM kg_entities WHERE name = $1 AND project_id = $2`,
		name, projectID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	// Try alias match (case-insensitive convenience).
	alias := strings.ToLower(name)
	err = tx.QueryRowContext(ctx,
		`SELECT e.id FROM kg_entity_aliases a
		 JOIN kg_entities e ON a.entity_id = e.id
		 WHERE a.alias = $1 AND e.project_id = $2
		 LIMIT 1`,
		alias, projectID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	id = newHexID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_entities (id, name, project_id) VALUES ($1, $2, $3)
		 ON CONFLICT (name, project_id) DO NOTHING`,
		id, name, projectID,
	); err != nil {
		return "", fmt.Errorf("insert entity: %w", err)
	}
	// Concurrent inserter may have won the race — re-select to grab the
	// authoritative id.
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM kg_entities WHERE name = $1 AND project_id = $2`,
		name, projectID,
	).Scan(&id); err != nil {
		return "", fmt.Errorf("fetch entity after insert: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_entity_aliases (entity_id, alias) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		id, alias,
	); err != nil {
		return "", fmt.Errorf("insert alias: %w", err)
	}
	return id, nil
}

func pgEnsurePredicate(ctx context.Context, tx *sql.Tx, name string) (string, bool, error) {
	var id string
	var isFunctional bool
	err := tx.QueryRowContext(ctx,
		`SELECT id, is_functional FROM kg_predicates WHERE name = $1`, name,
	).Scan(&id, &isFunctional)
	if err == nil {
		return id, isFunctional, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}

	id = newHexID()
	isFunctional = isBuiltinFunctional(name)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_predicates (id, name, is_functional) VALUES ($1, $2, $3)
		 ON CONFLICT (name) DO NOTHING`,
		id, name, isFunctional,
	); err != nil {
		return "", false, fmt.Errorf("insert predicate: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT id, is_functional FROM kg_predicates WHERE name = $1`, name,
	).Scan(&id, &isFunctional); err != nil {
		return "", false, fmt.Errorf("fetch predicate after insert: %w", err)
	}
	return id, isFunctional, nil
}

// isBuiltinFunctional returns true for predicates where the subject usually
// has a single live object at a time. New predicates can be flipped later via
// admin tooling; this is just a sane default for the well-known ones used by
// Anchored clients.
func isBuiltinFunctional(name string) bool {
	switch strings.ToLower(name) {
	case "deployed_on", "owns", "owned_by", "located_at", "managed_by", "current_version":
		return true
	}
	return false
}

func newHexID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
