//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/project"
)

// TestFreshInstallFlow is the wave-1 master criterion: configure → doctor
// clean → save → sync from inside the repo → memories land in the matching
// project. Mirrors a brand-new user following the Connect tab.
func TestFreshInstallFlow(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)
	ctx := context.Background()

	// Project pre-created in the dashboard with the repository URL.
	proj, err := e.store.CreateProject(ctx, e.orgID, "API Service", "api-service",
		project.DeriveRemoteKey(testOrigin), project.DeriveLegacyRemoteKey(testOrigin),
		testOrigin, e.acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := e.store.EnsureCreatorProjectAccess(ctx, e.orgID, e.acctID, proj.ID); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	e.configureRemote(t, e.syncKey)

	// Doctor must pass (exit 0) and report the remote + matched identity.
	out, code := e.run(t, repo, "doctor", "--json", "--cwd", repo)
	if code != 0 {
		t.Fatalf("doctor exit %d:\n%s", code, out)
	}
	var doc struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &doc); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, out)
	}
	var remoteOK, identityOK bool
	for _, c := range doc.Checks {
		if strings.HasPrefix(c.Name, `remote "e2e"`) && c.Status == "ok" {
			remoteOK = true
		}
		if c.Name == "project identity" && c.Status == "ok" && strings.Contains(c.Detail, proj.ID) {
			identityOK = true
		}
	}
	if !remoteOK {
		t.Fatalf("doctor did not report remote ok:\n%s", out)
	}
	if !identityOK {
		t.Fatalf("doctor did not match project identity:\n%s", out)
	}

	// Save a memory from inside the repo, then sync.
	out, code = e.run(t, repo, "save",
		"decision: the api-service deployment pipeline standardizes on the sync-scope API key for pushes because admin keys bypass team access checks and would mask authorization regressions in end-to-end coverage",
		"--category", "decision")
	if code != 0 {
		t.Fatalf("save exit %d:\n%s", code, out)
	}
	out, code = e.run(t, repo, "remote", "sync")
	if code != 0 {
		t.Fatalf("sync exit %d:\n%s", code, out)
	}
	mustContain(t, out, "accepted")

	if n := countProjectMemories(t, e.store, proj.ID); n < 1 {
		t.Fatalf("project has %d memories after sync, want >= 1", n)
	}
}

// TestIdentityMismatch locks invariant (1): a push explicitly targeted at a
// project registered to a DIFFERENT repository is refused without --force.
func TestIdentityMismatch(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)
	ctx := context.Background()

	otherOrigin := "git@github.example.com:org/unrelated.git"
	other, err := e.store.CreateProject(ctx, e.orgID, "Unrelated", "unrelated",
		project.DeriveRemoteKey(otherOrigin), project.DeriveLegacyRemoteKey(otherOrigin),
		otherOrigin, e.acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := e.store.EnsureCreatorProjectAccess(ctx, e.orgID, e.acctID, other.ID); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	e.configureRemote(t, e.syncKey)
	if out, code := e.run(t, repo, "save",
		"learning: the repository identity guard caught a push aimed at a project registered to another repository, which is exactly the wrong-project pollution the sync contract is designed to prevent",
		"--category", "learning"); code != 0 {
		t.Fatalf("save: %d\n%s", code, out)
	}

	out, code := e.run(t, repo, "remote", "sync", "--project-id", other.ID)
	if code == 0 {
		t.Fatalf("mismatched push must fail, got exit 0:\n%s", out)
	}
	mustContain(t, out, "different repository")
	if n := countProjectMemories(t, e.store, other.ID); n != 0 {
		t.Fatalf("refused push must write nothing, project has %d memories", n)
	}

	// --force is the documented escape hatch.
	out, code = e.run(t, repo, "remote", "sync", "--project-id", other.ID, "--force")
	if code != 0 {
		t.Fatalf("forced push failed (%d):\n%s", code, out)
	}
	mustContain(t, out, "accepted")
	if n := countProjectMemories(t, e.store, other.ID); n < 1 {
		t.Fatal("forced push accepted nothing")
	}
}

// TestDoctorOffline: with the server down, doctor reports the connectivity
// failure class and exits non-zero (critical check).
func TestDoctorOffline(t *testing.T) {
	e := newEnv(t)
	e.configureRemote(t, e.syncKey)

	// Point the configured remote at a dead port by replacing the config URL.
	// (Simplest reliable "offline": nothing listens on port 1.)
	cfgPath := e.home + "/.anchored/config.yaml"
	data, err := readFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := writeFile(cfgPath, strings.ReplaceAll(data, e.baseURL, "http://127.0.0.1:1")); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, code := e.run(t, "", "doctor", "--json")
	if code == 0 {
		t.Fatalf("doctor must exit non-zero with remote down:\n%s", out)
	}
	var doc struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &doc); err != nil {
		t.Fatalf("doctor json: %v\n%s", err, out)
	}
	var failedConn bool
	for _, c := range doc.Checks {
		if strings.HasPrefix(c.Name, `remote "e2e"`) && c.Status == "failed" &&
			strings.Contains(c.Detail, "connectivity failed") {
			failedConn = true
		}
	}
	if !failedConn {
		t.Fatalf("missing failed connectivity check:\n%s", out)
	}
}

// TestNoFallback locks invariant (2) end to end: a repo whose key matches no
// project errors out actionably and writes NOTHING anywhere on the server.
func TestNoFallback(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, "git@github.example.com:org/never-registered.git")
	ctx := context.Background()

	// A different project exists and is even linked-accessible — it must not
	// become a fallback target.
	decoyOrigin := "git@github.example.com:org/decoy.git"
	decoy, err := e.store.CreateProject(ctx, e.orgID, "Decoy", "decoy",
		project.DeriveRemoteKey(decoyOrigin), project.DeriveLegacyRemoteKey(decoyOrigin),
		decoyOrigin, e.acctID, "service")
	if err != nil {
		t.Fatalf("create decoy: %v", err)
	}
	if err := e.store.EnsureCreatorProjectAccess(ctx, e.orgID, e.acctID, decoy.ID); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	e.configureRemote(t, e.syncKey)
	if out, code := e.run(t, repo, "save",
		"learning: a repository whose remote key matches no configured project must fail the sync with an actionable error instead of silently falling back to whichever project happens to be linked",
		"--category", "learning"); code != 0 {
		t.Fatalf("save: %d\n%s", code, out)
	}

	out, code := e.run(t, repo, "remote", "sync")
	if code == 0 {
		t.Fatalf("sync with unknown key must fail, got 0:\n%s", out)
	}
	mustContain(t, out, "No configured remote has a project", "Repository URL")

	if n := allMemoryCount(t, e); n != 0 {
		t.Fatalf("server must have zero memories after refused sync, has %d", n)
	}
}
