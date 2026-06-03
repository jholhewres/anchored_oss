package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Error("marshal response failed", "error", err)
		return
	}
	w.Write(sanitizeJSONControlChars(raw))
	w.Write([]byte("\n"))
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	raw, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		slog.Error("marshal error response failed", "error", err)
		return
	}
	w.Write(sanitizeJSONControlChars(raw))
	w.Write([]byte("\n"))
}

// sanitizeJSONControlChars replaces raw control characters (0x00-0x1F)
// inside JSON string values with their properly escaped equivalents.
// Characters outside strings (structural JSON) are left untouched.
func sanitizeJSONControlChars(b []byte) []byte {
	// Quick check: if no control chars, return as-is
	found := false
	for _, c := range b {
		if c < 0x20 {
			found = true
			break
		}
	}
	if !found {
		return b
	}

	var buf bytes.Buffer
	buf.Grow(len(b) + len(b)/10)
	inString := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			if c == '\\' && i+1 < len(b) {
				// Already an escape sequence, pass through.
				buf.WriteByte(c)
				buf.WriteByte(b[i+1])
				i++
				continue
			}
			if c == '"' {
				inString = false
				buf.WriteByte(c)
				continue
			}
			if c < 0x20 {
				switch c {
				case '\n':
					buf.WriteString("\\n")
				case '\r':
					buf.WriteString("\\r")
				case '\t':
					buf.WriteString("\\t")
				default:
					fmt.Fprintf(&buf, "\\u%04x", c)
				}
				continue
			}
			buf.WriteByte(c)
		} else {
			if c == '"' {
				inString = true
			}
			buf.WriteByte(c)
		}
	}
	return buf.Bytes()
}
