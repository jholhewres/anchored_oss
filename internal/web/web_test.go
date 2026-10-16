package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	scriptTag = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	srcAttr   = regexp.MustCompile(`(?i)\bsrc\s*=`)
)

// inlineScripts returns the <script> tags without a src attribute, which the
// dashboard CSP (script-src 'self') refuses to run.
func inlineScripts(html string) []string {
	var found []string
	for _, tag := range scriptTag.FindAllString(html, -1) {
		if !srcAttr.MatchString(tag) {
			found = append(found, tag)
		}
	}
	return found
}

func TestIndexHasNoInlineScript(t *testing.T) {
	src, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatalf("read web/index.html: %v", err)
	}
	if tags := inlineScripts(string(src)); len(tags) > 0 {
		t.Fatalf("web/index.html has inline scripts the CSP blocks: %q", tags)
	}
	if !strings.Contains(string(src), `src="/theme-init.js"`) {
		t.Fatal("web/index.html must load the theme bootstrap from /theme-init.js")
	}
	if _, err := os.Stat("../../web/public/theme-init.js"); err != nil {
		t.Fatalf("theme bootstrap file missing: %v", err)
	}
}

func TestServedIndexLoadsThemeBootstrapUnderCSP(t *testing.T) {
	root, err := fs.Sub(DistFS, "dist")
	if err != nil {
		t.Fatal(err)
	}
	built, err := fs.ReadFile(root, "index.html")
	if err != nil || !strings.Contains(string(built), "/assets/") {
		t.Skip("no production dashboard build in internal/web/dist; run `cd web && npm run build`")
	}

	h, err := NewSPAHandler()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("index CSP = %q, want script-src 'self'", csp)
	}
	if tags := inlineScripts(rec.Body.String()); len(tags) > 0 {
		t.Fatalf("served index.html has inline scripts the CSP blocks: %q", tags)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/theme-init.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /theme-init.js = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("GET /theme-init.js Content-Type = %q, want JavaScript", ct)
	}
	if !strings.Contains(rec.Body.String(), "anchored_theme") {
		t.Fatal("served /theme-init.js does not apply the saved theme")
	}
}
