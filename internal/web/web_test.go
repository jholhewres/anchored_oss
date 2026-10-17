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
	scriptTag    = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	srcAttr      = regexp.MustCompile(`(?i)\bsrc\s*=`)
	deferredAttr = regexp.MustCompile(`(?i)\b(defer|async)\b|type\s*=\s*"module"`)
)

// assertThemeBootstrapRunsFirst checks that the theme bootstrap is a classic,
// blocking script placed before the first module script (the bundle), so the
// saved theme is applied before the app renders.
func assertThemeBootstrapRunsFirst(t *testing.T, html string) {
	t.Helper()
	tags := scriptTag.FindAllString(html, -1)
	for i, tag := range tags {
		if strings.Contains(tag, `src="/theme-init.js"`) {
			if deferredAttr.MatchString(tag) {
				t.Fatalf("theme bootstrap must block (no defer, async or type=module): %s", tag)
			}
			return
		}
		if strings.Contains(tag, `type="module"`) {
			t.Fatalf("script %d (%s) loads before the theme bootstrap", i, tag)
		}
	}
	t.Fatal("index.html must load the theme bootstrap from /theme-init.js")
}

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
	assertThemeBootstrapRunsFirst(t, string(src))
	script, err := os.ReadFile("../../web/public/theme-init.js")
	if err != nil {
		t.Fatalf("theme bootstrap file missing: %v", err)
	}
	for _, want := range []string{`localStorage.getItem("anchored_theme")`, "dataset.theme"} {
		if !strings.Contains(string(script), want) {
			t.Fatalf("theme-init.js does not apply the saved theme (missing %q)", want)
		}
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
	assertThemeBootstrapRunsFirst(t, rec.Body.String())

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
