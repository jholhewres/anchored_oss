// Package web embeds the admin dashboard build output and serves it from
// the same binary that exposes the JSON API. The Vite production build
// lands in `dist/` (configured via vite.config.ts outDir).
package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"strings"
)

// DistFS holds the compiled SPA. A stub index.html is tracked in git so
// `go build` works without a Node.js toolchain; `make web-build` overwrites
// it with the real Vite output.
//
//go:embed all:dist
var DistFS embed.FS

// NewSPAHandler returns an http.Handler that:
//   - rejects /v1/* and /api/* with a JSON 404 (the server's API routes
//     win first, but if a path slipped through the mux this guard avoids
//     handing back index.html and confusing API clients);
//   - serves any embedded file directly with cache headers tuned to its
//     name (hashed `assets/*` are immutable, everything else is no-cache);
//   - falls back to index.html for SPA deep links.
//
// CSP is set on the index.html response so React lives inside a tight
// `default-src 'self'; script-src 'self'` envelope.
func NewSPAHandler() (http.Handler, error) {
	root, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, err
	}
	indexBytes, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API guard: never let the SPA mask a typo in API paths.
		if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND"}`))
			return
		}

		clean := strings.TrimPrefix(path, "/")
		if clean == "" {
			serveIndex(w, indexBytes)
			return
		}

		f, err := root.Open(clean)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				serveIndex(w, indexBytes)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			serveIndex(w, indexBytes)
			return
		}

		if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), readSeeker(f))
	}), nil
}

func serveIndex(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// readSeeker adapts an fs.File to io.ReadSeeker when the underlying file
// supports seeking. Embedded files always do.
func readSeeker(f fs.File) interface {
	Read(p []byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
} {
	type rs interface {
		Read(p []byte) (int, error)
		Seek(offset int64, whence int) (int64, error)
	}
	if seeker, ok := f.(rs); ok {
		return seeker
	}
	return nil
}
