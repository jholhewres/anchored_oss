package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpsertTriple mirrors PostgresStore.UpsertTriple semantics for SQLite.
// SQLite supports partial unique indexes since 3.8, so the same logical
// dedup works: idx_kg_triples_logical (subject, predicate, object, project)
// WHERE valid_to IS NULL.
func (s *SQLiteStore) UpsertTriple(ctx context.Context, t *model.Triple) error {
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

	subjectID, err := sqliteEnsureEntity(ctx, tx, subj, t.ProjectID)
	if err != nil {
		return fmt.Errorf("ensure subject: %w", err)
	}
	predicateID, isFunctional, err := sqliteEnsurePredicate(ctx, tx, pred)
	if err != nil {
		return fmt.Errorf("ensure predicate: %w", err)
	}
	objectID, err := sqliteEnsureEntity(ctx, tx, obj, t.ProjectID)
	if err != nil {
		return fmt.Errorf("ensure object: %w", err)
	}

	if isFunctional {
		if _, err := tx.ExecContext(ctx,
			`UPDATE kg_triples SET valid_to = datetime('now')
			 WHERE subject_id = ? AND predicate_id = ? AND project_id = ?
			       AND object_id <> ? AND valid_to IS NULL`,
			subjectID, predicateID, t.ProjectID, objectID,
		); err != nil {
			return fmt.Errorf("supersede functional triple: %w", err)
		}
	}

	id := newHexID()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO kg_triples (id, subject_id, predicate_id, object_id, confidence, project_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (subject_id, predicate_id, object_id, project_id) WHERE valid_to IS NULL
		 DO NOTHING`,
		id, subjectID, predicateID, objectID, t.Confidence, t.ProjectID,
	)
	if err != nil {
		return fmt.Errorf("insert triple: %w", err)
	}

	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM kg_triples
		 WHERE subject_id = ? AND predicate_id = ? AND object_id = ? AND project_id = ?
		   AND valid_to IS NULL`,
		subjectID, predicateID, objectID, t.ProjectID,
	).Scan(&id); err != nil {
		return fmt.Errorf("fetch triple id: %w", err)
	}
	t.ID = id

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListTriplesByProject(ctx context.Context, projectID string, limit, offset int) ([]*model.Triple, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kg_triples WHERE project_id = ? AND valid_to IS NULL`,
		projectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count triples: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, s.name, p.name, o.name, t.confidence, t.project_id, t.created_at
		 FROM kg_triples t
		 JOIN kg_entities s ON t.subject_id = s.id
		 JOIN kg_predicates p ON t.predicate_id = p.id
		 JOIN kg_entities o ON t.object_id = o.id
		 WHERE t.project_id = ? AND t.valid_to IS NULL
		 ORDER BY t.created_at DESC
		 LIMIT ? OFFSET ?`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list triples: %w", err)
	}
	defer rows.Close()

	triples := make([]*model.Triple, 0, limit)
	for rows.Next() {
		var t model.Triple
		if err := rows.Scan(&t.ID, &t.Subject, &t.Predicate, &t.Object, &t.Confidence, &t.ProjectID, scanTime(&t.CreatedAt)); err != nil {
			return nil, 0, fmt.Errorf("scan triple: %w", err)
		}
		triples = append(triples, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate triples: %w", err)
	}
	return triples, total, nil
}

func (s *SQLiteStore) CountTriplesByProject(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kg_triples WHERE project_id = ? AND valid_to IS NULL`,
		projectID,
	).Scan(&count)
	return count, err
}

func sqliteEnsureEntity(ctx context.Context, tx *sql.Tx, name, projectID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM kg_entities WHERE name = ? AND project_id = ?`,
		name, projectID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	alias := strings.ToLower(name)
	err = tx.QueryRowContext(ctx,
		`SELECT e.id FROM kg_entity_aliases a
		 JOIN kg_entities e ON a.entity_id = e.id
		 WHERE a.alias = ? AND e.project_id = ?
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
		`INSERT INTO kg_entities (id, name, project_id) VALUES (?, ?, ?)
		 ON CONFLICT (name, project_id) DO NOTHING`,
		id, name, projectID,
	); err != nil {
		return "", fmt.Errorf("insert entity: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM kg_entities WHERE name = ? AND project_id = ?`,
		name, projectID,
	).Scan(&id); err != nil {
		return "", fmt.Errorf("fetch entity after insert: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_entity_aliases (entity_id, alias) VALUES (?, ?)
		 ON CONFLICT DO NOTHING`,
		id, alias,
	); err != nil {
		return "", fmt.Errorf("insert alias: %w", err)
	}
	return id, nil
}

func sqliteEnsurePredicate(ctx context.Context, tx *sql.Tx, name string) (string, bool, error) {
	var id string
	var isFunctional bool
	err := tx.QueryRowContext(ctx,
		`SELECT id, is_functional FROM kg_predicates WHERE name = ?`, name,
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
		`INSERT INTO kg_predicates (id, name, is_functional) VALUES (?, ?, ?)
		 ON CONFLICT (name) DO NOTHING`,
		id, name, isFunctional,
	); err != nil {
		return "", false, fmt.Errorf("insert predicate: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT id, is_functional FROM kg_predicates WHERE name = ?`, name,
	).Scan(&id, &isFunctional); err != nil {
		return "", false, fmt.Errorf("fetch predicate after insert: %w", err)
	}
	return id, isFunctional, nil
}
