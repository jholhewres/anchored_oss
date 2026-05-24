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

func scanNullString(dest *string) *sql.NullString {
	return &sql.NullString{}
}

func fromNullString(ns *sql.NullString, dest *string) {
	if ns.Valid {
		*dest = ns.String
	}
}
