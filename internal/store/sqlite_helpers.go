package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func jsonMarshalKeywords(keywords []string) string {
	if keywords == nil {
		return "[]"
	}
	b, _ := json.Marshal(keywords)
	return string(b)
}

type sqliteTime time.Time

var sqliteTimeFormats = []string{
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02T15:04:05.999999999 -0700 MST",
	"2006-01-02T15:04:05 -0700 MST",
	"2006-01-02T15:04:05.999999999Z",
	"2006-01-02T15:04:05Z",
}

func (t *sqliteTime) Scan(value interface{}) error {
	if value == nil {
		*t = sqliteTime(time.Time{})
		return nil
	}
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("sqliteTime: expected string, got %T", value)
	}
	for _, f := range sqliteTimeFormats {
		if parsed, err := time.Parse(f, s); err == nil {
			*t = sqliteTime(parsed)
			return nil
		}
	}
	return fmt.Errorf("sqliteTime: cannot parse %q", s)
}

func (t sqliteTime) Time() time.Time { return time.Time(t) }

func scanTime(dest *time.Time) *sqliteTime {
	return (*sqliteTime)(dest)
}

type nullTimeScanner struct {
	dest **time.Time
}

func (s *nullTimeScanner) Scan(value interface{}) error {
	if value == nil {
		*s.dest = nil
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("nullTimeScanner: expected string, got %T", value)
	}
	for _, f := range sqliteTimeFormats {
		if parsed, err := time.Parse(f, str); err == nil {
			t := parsed
			*s.dest = &t
			return nil
		}
	}
	return fmt.Errorf("nullTimeScanner: cannot parse %q", str)
}

func scanNullTime(dest **time.Time) *nullTimeScanner {
	return &nullTimeScanner{dest: dest}
}

type nullStringScanner struct {
	dest *string
}

func (s *nullStringScanner) Scan(value any) error {
	var nullable sql.NullString
	if err := nullable.Scan(value); err != nil {
		return err
	}
	if nullable.Valid {
		*s.dest = nullable.String
	} else {
		*s.dest = ""
	}
	return nil
}

func scanNullString(dest *string) *nullStringScanner {
	return &nullStringScanner{dest: dest}
}

// nullIfEmpty maps "" to a SQL NULL so nullable TEXT columns stay NULL instead
// of storing an empty string. Used for repo_url / remote_key_v1 on projects.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// deletedRemoteKey is the per-row sentinel remote_key parked on a soft-deleted
// project. remote_key is NOT NULL and carries UNIQUE(org_id, remote_key), so a
// shared placeholder (e.g. "") would collide on the second delete in an org.
// The full project id (a UUID) guarantees uniqueness, and the "deleted-" prefix
// can never match a sha256-derived 16-hex repo key, so a freed repo can be
// re-linked without resolving to this dead row.
func deletedRemoteKey(id string) string { return "deleted-" + id }

// noRepoRemoteKey is the per-row sentinel remote_key parked on a project whose
// repo_url was cleared. Same rationale as deletedRemoteKey: keep the NOT NULL,
// org-unique column collision-free while leaving the project unmatchable by any
// real repo key.
func noRepoRemoteKey(id string) string { return "norepo-" + id }

// mangleDeletedSlug derives the parked slug a soft-deleted project is renamed
// to so its original slug is freed for reuse: slug + "-deleted-" + id[:8].
// Computed in Go so the soft-delete UPDATE is identical across backends.
func mangleDeletedSlug(slug, id string) string {
	suffix := id
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return slug + "-deleted-" + suffix
}
