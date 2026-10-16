package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
	anchoredskill "github.com/jholhewres/anchored_oss/internal/skill"
)

const sqliteSkillColumns = `s.id, s.org_id, s.name, s.slug, s.purpose, s.status, COALESCE(s.created_by, ''), s.created_at, s.updated_at,
	v.skill_id, v.version, v.purpose, v.body, v.frontmatter, v.content_hash, COALESCE(v.created_by, ''), v.created_at,
	(SELECT COUNT(*) FROM project_skills pc WHERE pc.skill_id = s.id)`

func scanSQLiteSkill(row interface{ Scan(...any) error }, skill *model.Skill) error {
	version := &model.SkillVersion{}
	var frontmatter []byte
	if err := row.Scan(&skill.ID, &skill.OrgID, &skill.Name, &skill.Slug, &skill.Purpose, &skill.Status, &skill.CreatedBy,
		scanTime(&skill.CreatedAt), scanTime(&skill.UpdatedAt), &version.SkillID, &version.Version, &version.Purpose, &version.Body, &frontmatter,
		&version.ContentHash, &version.CreatedBy, scanTime(&version.CreatedAt), &skill.ProjectCount); err != nil {
		return err
	}
	if frontmatter == nil {
		version.Content = version.Body
	} else {
		parsed, err := storedSkillHeader(frontmatter, skill.Slug, version.Purpose)
		if err != nil {
			return fmt.Errorf("parse skill frontmatter: %w", err)
		}
		content, err := anchoredskill.Render(skill.Slug, version.Purpose, parsed, version.Body)
		if err != nil {
			return fmt.Errorf("render skill content: %w", err)
		}
		version.Frontmatter, version.Content, version.ContentHash = parsed, content, skillHash(content)
	}
	skill.CurrentVersion = version
	return nil
}

func (s *SQLiteStore) CreateSkill(ctx context.Context, skill *model.Skill, content string) error {
	if !mutableSkillStatus(skill.Status) {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create skill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	body, frontmatter, content, err := skillVersionForCreate(skill, content)
	if err != nil {
		return fmt.Errorf("prepare skill version: %w", err)
	}
	id := newUUID()
	_, err = tx.ExecContext(ctx, `INSERT INTO skills (id, org_id, name, slug, purpose, status, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, skill.OrgID, skill.Name, skill.Slug, skill.Purpose, skill.Status, nullIfEmpty(skill.CreatedBy))
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO skill_versions (skill_id, version, purpose, body, frontmatter, content_hash, created_by) VALUES (?, 1, ?, ?, ?, ?, ?)`, id, skill.Purpose, body, frontmatter, skillHash(content), nullIfEmpty(skill.CreatedBy)); err != nil {
		return fmt.Errorf("create skill version: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create skill: %w", err)
	}
	created, err := s.GetSkill(ctx, skill.OrgID, id)
	if err != nil {
		return err
	}
	*skill = *created
	return nil
}

func (s *SQLiteStore) ListSkills(ctx context.Context, orgID string) ([]*model.Skill, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sqliteSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version WHERE s.org_id = ? AND s.deleted_at IS NULL ORDER BY s.name, s.slug`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	out := make([]*model.Skill, 0)
	for rows.Next() {
		skill := &model.Skill{}
		if err := scanSQLiteSkill(rows, skill); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, skill)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetSkill(ctx context.Context, orgID, id string) (*model.Skill, error) {
	skill := &model.Skill{}
	err := scanSQLiteSkill(s.db.QueryRowContext(ctx, `SELECT `+sqliteSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version WHERE s.org_id = ? AND s.id = ? AND s.deleted_at IS NULL`, orgID, id), skill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return skill, nil
}

func (s *SQLiteStore) UpdateSkill(ctx context.Context, orgID, id, actorID string, upd model.SkillUpdate) (*model.Skill, error) {
	if upd.Status != nil && !mutableSkillStatus(*upd.Status) {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update skill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cur model.Skill
	err = scanSQLiteSkill(tx.QueryRowContext(ctx, `SELECT `+sqliteSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version WHERE s.org_id = ? AND s.id = ? AND s.deleted_at IS NULL`, orgID, id), &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load skill for update: %w", err)
	}
	if upd.Name != nil {
		cur.Name = *upd.Name
	}
	if upd.Status != nil {
		cur.Status = *upd.Status
	}
	versioned := upd.Purpose != nil || upd.Content != nil || upd.Body != nil || upd.Frontmatter != nil
	if upd.Purpose != nil {
		cur.Purpose = *upd.Purpose
	}
	if versioned {
		body, frontmatter, content, prepareErr := skillVersionForUpdate(&cur, upd)
		if prepareErr != nil {
			return nil, fmt.Errorf("prepare skill version: %w", prepareErr)
		}
		next := cur.CurrentVersion.Version + 1
		if _, err = tx.ExecContext(ctx, `INSERT INTO skill_versions (skill_id, version, purpose, body, frontmatter, content_hash, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, next, cur.Purpose, body, frontmatter, skillHash(content), nullIfEmpty(actorID)); err != nil {
			return nil, fmt.Errorf("publish skill version: %w", err)
		}
		cur.CurrentVersion.Version = next
	}
	if _, err = tx.ExecContext(ctx, `UPDATE skills SET name = ?, purpose = ?, status = ?, current_version = ?, updated_at = datetime('now') WHERE id = ? AND org_id = ?`, cur.Name, cur.Purpose, cur.Status, cur.CurrentVersion.Version, id, orgID); err != nil {
		return nil, fmt.Errorf("update skill: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update skill: %w", err)
	}
	return s.GetSkill(ctx, orgID, id)
}

func (s *SQLiteStore) RevokeSkill(ctx context.Context, orgID, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE skills SET status = 'revoked', deleted_at = datetime('now'), updated_at = datetime('now') WHERE org_id = ? AND id = ? AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return fmt.Errorf("revoke skill: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListProjectSkills(ctx context.Context, projectID, query string, limit int) ([]*model.SkillDescriptor, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.slug, s.name, s.purpose, s.status, s.current_version, v.content_hash, s.created_at, s.updated_at, ps.created_at
		FROM project_skills ps JOIN projects p ON p.id = ps.project_id JOIN skills s ON s.id = ps.skill_id JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE ps.project_id = ? AND p.deleted_at IS NULL AND s.status = 'active' AND s.deleted_at IS NULL AND (? = '%%' OR lower(s.name) LIKE ? OR lower(s.slug) LIKE ? OR lower(s.purpose) LIKE ?)
		ORDER BY CASE WHEN lower(s.name) LIKE ? OR lower(s.slug) LIKE ? THEN 0 ELSE 1 END, s.name LIMIT ?`, projectID, pattern, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("list project skills: %w", err)
	}
	defer rows.Close()
	out := make([]*model.SkillDescriptor, 0)
	for rows.Next() {
		d := &model.SkillDescriptor{}
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Purpose, &d.Status, &d.Version, &d.ContentHash, scanTime(&d.CreatedAt), scanTime(&d.UpdatedAt), scanNullTime(&d.AttachedAt)); err != nil {
			return nil, fmt.Errorf("scan project skill: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListProjectSkillAttachments(ctx context.Context, orgID, projectID string) ([]*model.SkillDescriptor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.slug, s.name, s.purpose, s.status, s.current_version, v.content_hash, s.created_at, s.updated_at, ps.created_at
		FROM project_skills ps
		JOIN projects p ON p.id = ps.project_id
		JOIN skills s ON s.id = ps.skill_id
		JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE ps.project_id = ? AND p.org_id = ? AND p.deleted_at IS NULL
		ORDER BY s.name, s.slug`, projectID, orgID)
	if err != nil {
		return nil, fmt.Errorf("list project skill attachments: %w", err)
	}
	defer rows.Close()
	out := make([]*model.SkillDescriptor, 0)
	for rows.Next() {
		d := &model.SkillDescriptor{}
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Purpose, &d.Status, &d.Version, &d.ContentHash, scanTime(&d.CreatedAt), scanTime(&d.UpdatedAt), scanNullTime(&d.AttachedAt)); err != nil {
			return nil, fmt.Errorf("scan project skill attachment: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetProjectSkill(ctx context.Context, projectID, slug string) (*model.Skill, error) {
	skill := &model.Skill{}
	err := scanSQLiteSkill(s.db.QueryRowContext(ctx, `SELECT `+sqliteSkillColumns+` FROM project_skills ps JOIN projects p ON p.id = ps.project_id JOIN skills s ON s.id = ps.skill_id JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version WHERE ps.project_id = ? AND s.slug = ? AND p.deleted_at IS NULL AND s.status = 'active' AND s.deleted_at IS NULL`, projectID, slug), skill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project skill: %w", err)
	}
	return skill, nil
}

func (s *SQLiteStore) ReplaceProjectSkills(ctx context.Context, orgID, projectID string, skillIDs []string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replace project skills: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id = ? AND org_id = ? AND deleted_at IS NULL)`, projectID, orgID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT skill_id FROM project_skills WHERE project_id = ? ORDER BY skill_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list existing project skills: %w", err)
	}
	oldIDs := make([]string, 0)
	oldSet := make(map[string]struct{})
	for rows.Next() {
		var skillID string
		if err := rows.Scan(&skillID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan existing project skill: %w", err)
		}
		oldIDs = append(oldIDs, skillID)
		oldSet[skillID] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close existing project skills: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing project skills: %w", err)
	}
	unique := make(map[string]struct{}, len(skillIDs))
	for _, skillID := range skillIDs {
		if skillID == "" {
			return nil, ErrNotFound
		}
		unique[skillID] = struct{}{}
	}
	for skillID := range unique {
		var ok bool
		_, alreadyAttached := oldSet[skillID]
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM skills WHERE id = ? AND org_id = ? AND (? OR (deleted_at IS NULL AND status <> 'revoked')))`, skillID, orgID, alreadyAttached).Scan(&ok); err != nil {
			return nil, fmt.Errorf("check skill attachment: %w", err)
		}
		if !ok {
			return nil, ErrNotFound
		}
	}
	for skillID := range unique {
		if _, exists := oldSet[skillID]; !exists {
			if _, err = tx.ExecContext(ctx, `INSERT INTO project_skills (project_id, skill_id) VALUES (?, ?)`, projectID, skillID); err != nil {
				return nil, fmt.Errorf("attach skill: %w", err)
			}
		}
	}
	for skillID := range oldSet {
		if _, retained := unique[skillID]; !retained {
			if _, err = tx.ExecContext(ctx, `DELETE FROM project_skills WHERE project_id = ? AND skill_id = ?`, projectID, skillID); err != nil {
				return nil, fmt.Errorf("detach skill: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit project skills: %w", err)
	}
	return oldIDs, nil
}
