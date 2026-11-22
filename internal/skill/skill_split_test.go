package skill

import (
	"runtime"
	"strings"
	"testing"
)

func TestSplitHeader(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		hasHeader     bool
		wantErr       bool
		wantKeys      int
		wantBody      string
	}{
		{"no header", "# Deploy\n\nSteps.", false, false, 0, "# Deploy\n\nSteps."},
		{"canonical", "---\nname: deploy\ndescription: Deploy safely\n---\n# Deploy\n", true, false, 2, "# Deploy\n"},
		{"extra key", "---\nname: deploy\ndescription: x\nallowed-tools: Bash\n---\nbody", true, false, 3, "body"},
		{"crlf and blank lines first", "\r\n\r\n---\r\nname: a\r\ndescription: b\r\n---\r\nbody", true, false, 2, "body"},
		{"unterminated", "---\nname: a\ndescription: b\nbody", true, true, 0, ""},
		{"not a mapping", "---\n- a\n- b\n---\nbody", true, true, 0, ""},
		{"opening with a non-breaking space", "---\u00a0\nname: a\ndescription: b\nallowed-tools: Bash\n---\nbody", true, false, 3, "body"},
		{"opening with text after it", "---yaml\nname: a\n---\nbody", true, true, 0, ""},
		{"BOM first", "\ufeff---\nname: a\ndescription: b\n---\nbody", true, false, 2, "body"},
		{"anchors and aliases", "---\nname: &n a\ndescription: *n\n---\nbody", true, true, 0, ""},
		{"merge key", "---\nbase: &b {x: 1}\n<<: *b\n---\nbody", true, true, 0, ""},
		{"nested value", "---\nname: a\ndescription: [b]\n---\nbody", true, true, 0, ""},
		{"empty header", "---\n---\nbody", true, false, 0, "body"},
		{"trailing spaces on the opening line", "--- \t\nname: a\ndescription: b\nallowed-tools: Bash\n---\nbody", true, false, 3, "body"},
		{"indented opening line", "  ---\nname: a\ndescription: b\n---\nbody", true, false, 2, "body"},
		{"a longer rule is refused", "---- \nbody", true, true, 0, ""},
		{"no dashes", "# Title\n---\n", false, false, 0, "# Title\n---\n"},
		{"document end inside", "---\nname: a\ndescription: b\n...\nallowed-tools: x\n---\nbody", true, true, 0, ""},
		{"directive inside", "---\n%YAML 1.2\nname: a\ndescription: b\n---\nbody", true, true, 0, ""},
		{"closing with trailing spaces", "---\nname: a\ndescription: b\n---  \nbody", true, false, 2, "body"},
		{"header past 32 KiB", "---\nname: a\ndescription: " + strings.Repeat("b", 40<<10) + "\n---\nbody", true, true, 0, ""},
	} {
		header, body, has, err := SplitHeader(tc.content)
		if has != tc.hasHeader || (err != nil) != tc.wantErr {
			t.Errorf("%s: hasHeader=%t err=%v, want %t err=%t", tc.name, has, err, tc.hasHeader, tc.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if len(header) != tc.wantKeys || body != tc.wantBody {
			t.Errorf("%s: header=%v body=%q, want %d keys and %q", tc.name, header, body, tc.wantKeys, tc.wantBody)
		}
	}
}

// TestSplitHeaderDoesNotExpandAliases (attack h1, run 20261122T223317Z): a
// small header built from nested aliases is refused without being expanded.
func TestSplitHeaderDoesNotExpandAliases(t *testing.T) {
	var b strings.Builder
	b.WriteString("---\nname: deploy\ndescription: Deploy safely\nl0: &l0 \"" + strings.Repeat("x", 2000) + "\"\n")
	b.WriteString("l1: &l1 [" + strings.TrimSuffix(strings.Repeat("*l0,", 60), ",") + "]\n")
	b.WriteString("l2: [" + strings.TrimSuffix(strings.Repeat("*l1,", 60), ",") + "]\n---\nbody")
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	start := stats.TotalAlloc
	if _, _, _, err := SplitHeader(b.String()); err == nil {
		t.Fatal("a header of aliases was accepted")
	}
	runtime.ReadMemStats(&stats)
	if grew := stats.TotalAlloc - start; grew > 4<<20 {
		t.Fatalf("refusing a %d-byte alias header allocated %d bytes", b.Len(), grew)
	}
}
