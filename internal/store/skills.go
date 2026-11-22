package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
	anchoredskill "github.com/jholhewres/anchored_oss/internal/skill"
)

const pgSkillColumns = `s.id, s.org_id, s.name, s.slug, s.purpose, s.status, COALESCE(s.created_by::text, ''), s.created_at, s.updated_at,
	v.skill_id, v.version, v.purpose, v.body, v.frontmatter, v.content_hash, COALESCE(v.created_by::text, ''), v.created_at,
	(SELECT COUNT(*) FROM project_skills pc WHERE pc.skill_id = s.id)`

func skillHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// storedSkillHeader tolerates draft rows produced before the SKILL.md header
// contract was corrected. Their generic JSON metadata is discarded in the
// runtime view; the canonical header is always derived from the catalog slug
// and purpose.
func storedSkillHeader(raw []byte, slug, purpose string) (map[string]any, error) {
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted == nil {
		return nil, fmt.Errorf("frontmatter must be a JSON object")
	}
	return anchoredskill.Header(slug, purpose), nil
}

func mutableSkillStatus(status string) bool {
	switch status {
	case model.SkillStatusDraft, model.SkillStatusActive, model.SkillStatusDeprecated:
		return true
	}
	return false
}

func scanPGSkill(row interface{ Scan(...any) error }, skill *model.Skill) error {
	version := &model.SkillVersion{}
	var frontmatter []byte
	if err := row.Scan(&skill.ID, &skill.OrgID, &skill.Name, &skill.Slug, &skill.Purpose, &skill.Status, &skill.CreatedBy,
		&skill.CreatedAt, &skill.UpdatedAt, &version.SkillID, &version.Version, &version.Purpose, &version.Body, &frontmatter,
		&version.ContentHash, &version.CreatedBy, &version.CreatedAt, &skill.ProjectCount); err != nil {
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

func skillVersionForCreate(skill *model.Skill, legacyContent string) (body string, frontmatter any, content string, err error) {
	if skill.CurrentVersion == nil || skill.CurrentVersion.Frontmatter == nil {
		if skill.CurrentVersion != nil && skill.CurrentVersion.Body != "" {
			legacyContent = skill.CurrentVersion.Body
		}
		return legacyContent, nil, legacyContent, nil
	}
	body = skill.CurrentVersion.Body
	header := anchoredskill.Header(skill.Slug, skill.Purpose)
	encoded, err := anchoredskill.JSON(header)
	if err != nil {
		return "", nil, "", err
	}
	content, err = anchoredskill.Render(skill.Slug, skill.Purpose, header, body)
	if err != nil {
		return "", nil, "", err
	}
	skill.CurrentVersion.Frontmatter = header
	return body, string(encoded), content, nil
}

func skillVersionForUpdate(current *model.Skill, upd model.SkillUpdate) (body string, frontmatter any, content string, err error) {
	// A legacy row's body is its raw SKILL.md, header included. Frontmatter
	// without a new body would render a second header on top of it.
	if upd.Frontmatter != nil && upd.Body == nil && current.CurrentVersion.Frontmatter == nil {
		return "", nil, "", ErrInvalid
	}
	body = current.CurrentVersion.Body
	fm := current.CurrentVersion.Frontmatter
	if upd.Content != nil {
		body = *upd.Content
	}
	if upd.Body != nil {
		body = *upd.Body
		if fm == nil {
			fm = map[string]any{}
		}
	}
	if upd.Frontmatter != nil && fm == nil {
		fm = map[string]any{}
	}
	if fm == nil {
		return body, nil, body, nil
	}
	fm = anchoredskill.Header(current.Slug, current.Purpose)
	encoded, err := anchoredskill.JSON(fm)
	if err != nil {
		return "", nil, "", err
	}
	content, err = anchoredskill.Render(current.Slug, current.Purpose, fm, body)
	if err != nil {
		return "", nil, "", err
	}
	return body, string(encoded), content, nil
}

func (s *PostgresStore) CreateSkill(ctx context.Context, skill *model.Skill, content string) error {
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
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO skills (org_id, name, slug, purpose, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, skill.OrgID, skill.Name, skill.Slug, skill.Purpose, skill.Status, nullIfEmpty(skill.CreatedBy)).Scan(&id)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO skill_versions (skill_id, version, purpose, body, frontmatter, content_hash, created_by)
		VALUES ($1, 1, $2, $3, $4, $5, $6)`, id, skill.Purpose, body, frontmatter, skillHash(content), nullIfEmpty(skill.CreatedBy)); err != nil {
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

func (s *PostgresStore) ListSkills(ctx context.Context, orgID string) ([]*model.Skill, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pgSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE s.org_id = $1 AND s.deleted_at IS NULL ORDER BY s.name, s.slug`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	out := make([]*model.Skill, 0)
	for rows.Next() {
		skill := &model.Skill{}
		if err := scanPGSkill(rows, skill); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, skill)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetSkill(ctx context.Context, orgID, id string) (*model.Skill, error) {
	skill := &model.Skill{}
	err := scanPGSkill(s.db.QueryRowContext(ctx, `SELECT `+pgSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE s.org_id = $1 AND s.id = $2 AND s.deleted_at IS NULL`, orgID, id), skill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return skill, nil
}

func (s *PostgresStore) UpdateSkill(ctx context.Context, orgID, id, actorID string, upd model.SkillUpdate) (*model.Skill, error) {
	if upd.Status != nil && !mutableSkillStatus(*upd.Status) {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update skill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cur model.Skill
	err = scanPGSkill(tx.QueryRowContext(ctx, `SELECT `+pgSkillColumns+` FROM skills s JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE s.org_id = $1 AND s.id = $2 AND s.deleted_at IS NULL FOR UPDATE`, orgID, id), &cur)
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
		if errors.Is(prepareErr, ErrInvalid) {
			return nil, ErrInvalid
		}
		if prepareErr != nil {
			return nil, fmt.Errorf("prepare skill version: %w", prepareErr)
		}
		next := cur.CurrentVersion.Version + 1
		if _, err = tx.ExecContext(ctx, `INSERT INTO skill_versions (skill_id, version, purpose, body, frontmatter, content_hash, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, next, cur.Purpose, body, frontmatter, skillHash(content), nullIfEmpty(actorID)); err != nil {
			return nil, fmt.Errorf("publish skill version: %w", err)
		}
		cur.CurrentVersion.Version = next
	}
	if _, err = tx.ExecContext(ctx, `UPDATE skills SET name = $1, purpose = $2, status = $3, current_version = $4, updated_at = now() WHERE id = $5 AND org_id = $6`, cur.Name, cur.Purpose, cur.Status, cur.CurrentVersion.Version, id, orgID); err != nil {
		return nil, fmt.Errorf("update skill: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update skill: %w", err)
	}
	return s.GetSkill(ctx, orgID, id)
}

func (s *PostgresStore) RevokeSkill(ctx context.Context, orgID, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE skills SET status = 'revoked', deleted_at = now(), updated_at = now() WHERE org_id = $1 AND id = $2 AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return fmt.Errorf("revoke skill: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListProjectSkills(ctx context.Context, projectID, query string, limit int) ([]*model.SkillDescriptor, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.slug, s.name, s.purpose, s.status, s.current_version, v.content_hash, s.created_at, s.updated_at, ps.created_at
		FROM project_skills ps JOIN projects p ON p.id = ps.project_id JOIN skills s ON s.id = ps.skill_id JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE ps.project_id = $1 AND p.deleted_at IS NULL AND s.status = 'active' AND s.deleted_at IS NULL AND ($2 = '%%' OR lower(s.name) LIKE $2 OR lower(s.slug) LIKE $2 OR lower(s.purpose) LIKE $2)
		ORDER BY CASE WHEN lower(s.name) LIKE $2 OR lower(s.slug) LIKE $2 THEN 0 ELSE 1 END, s.name LIMIT $3`, projectID, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("list project skills: %w", err)
	}
	defer rows.Close()
	out := make([]*model.SkillDescriptor, 0)
	for rows.Next() {
		d := &model.SkillDescriptor{}
		var attached sql.NullTime
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Purpose, &d.Status, &d.Version, &d.ContentHash, &d.CreatedAt, &d.UpdatedAt, &attached); err != nil {
			return nil, fmt.Errorf("scan project skill: %w", err)
		}
		if attached.Valid {
			d.AttachedAt = &attached.Time
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListProjectSkillAttachments(ctx context.Context, orgID, projectID string) ([]*model.SkillDescriptor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.slug, s.name, s.purpose, s.status, s.current_version, v.content_hash, s.created_at, s.updated_at, ps.created_at
		FROM project_skills ps
		JOIN projects p ON p.id = ps.project_id
		JOIN skills s ON s.id = ps.skill_id
		JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE ps.project_id = $1 AND p.org_id = $2 AND p.deleted_at IS NULL
		ORDER BY s.name, s.slug`, projectID, orgID)
	if err != nil {
		return nil, fmt.Errorf("list project skill attachments: %w", err)
	}
	defer rows.Close()
	out := make([]*model.SkillDescriptor, 0)
	for rows.Next() {
		d := &model.SkillDescriptor{}
		var attached sql.NullTime
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Purpose, &d.Status, &d.Version, &d.ContentHash, &d.CreatedAt, &d.UpdatedAt, &attached); err != nil {
			return nil, fmt.Errorf("scan project skill attachment: %w", err)
		}
		if attached.Valid {
			d.AttachedAt = &attached.Time
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetProjectSkill(ctx context.Context, projectID, slug string) (*model.Skill, error) {
	skill := &model.Skill{}
	err := scanPGSkill(s.db.QueryRowContext(ctx, `SELECT `+pgSkillColumns+` FROM project_skills ps JOIN projects p ON p.id = ps.project_id JOIN skills s ON s.id = ps.skill_id JOIN skill_versions v ON v.skill_id = s.id AND v.version = s.current_version
		WHERE ps.project_id = $1 AND s.slug = $2 AND p.deleted_at IS NULL AND s.status = 'active' AND s.deleted_at IS NULL`, projectID, slug), skill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project skill: %w", err)
	}
	return skill, nil
}

func (s *PostgresStore) ReplaceProjectSkills(ctx context.Context, orgID, projectID string, skillIDs []string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replace project skills: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize every replacement for this project. The returned pre-state then
	// describes the same attachment set the transaction mutates and audits.
	var lockedProjectID string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM projects WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL FOR UPDATE`, projectID, orgID).Scan(&lockedProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock project for skill replacement: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT skill_id::text FROM project_skills WHERE project_id = $1 ORDER BY skill_id`, projectID)
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
	// Lock every referenced skill in lexical UUID order. This makes concurrent
	// replacements deterministic and prevents RevokeSkill/UpdateSkill from
	// changing a status between validation and attachment mutation.
	lockSet := make(map[string]struct{}, len(oldSet)+len(unique))
	for skillID := range oldSet {
		lockSet[skillID] = struct{}{}
	}
	for skillID := range unique {
		lockSet[skillID] = struct{}{}
	}
	lockIDs := make([]string, 0, len(lockSet))
	for skillID := range lockSet {
		lockIDs = append(lockIDs, skillID)
	}
	sort.Strings(lockIDs)
	type skillLockState struct {
		status string
		live   bool
	}
	states := make(map[string]skillLockState, len(lockIDs))
	for _, skillID := range lockIDs {
		var state skillLockState
		err = tx.QueryRowContext(ctx, `SELECT status, deleted_at IS NULL FROM skills WHERE id = $1 AND org_id = $2 FOR UPDATE`, skillID, orgID).Scan(&state.status, &state.live)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock skill for replacement: %w", err)
		}
		states[skillID] = state
	}
	for skillID := range unique {
		state, exists := states[skillID]
		_, alreadyAttached := oldSet[skillID]
		if !exists || (!alreadyAttached && (!state.live || state.status == model.SkillStatusRevoked)) {
			return nil, ErrNotFound
		}
	}
	for skillID := range unique {
		if _, exists := oldSet[skillID]; !exists {
			if _, err = tx.ExecContext(ctx, `INSERT INTO project_skills (project_id, skill_id) VALUES ($1, $2)`, projectID, skillID); err != nil {
				return nil, fmt.Errorf("attach skill: %w", err)
			}
		}
	}
	for skillID := range oldSet {
		if _, retained := unique[skillID]; !retained {
			if _, err = tx.ExecContext(ctx, `DELETE FROM project_skills WHERE project_id = $1 AND skill_id = $2`, projectID, skillID); err != nil {
				return nil, fmt.Errorf("detach skill: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit project skills: %w", err)
	}
	return oldIDs, nil
}
