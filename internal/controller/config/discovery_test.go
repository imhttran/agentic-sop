package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkProject creates an initialized SOP project at dir: a .agent-sdlc/config.yaml
// declaring the given name, plus a placeholder .agent-sdlc/state.db. Discovery
// only checks that these exist, so no database engine is needed.
func mkProject(t *testing.T, dir, name string) {
	t.Helper()
	sdlc := filepath.Join(dir, ".agent-sdlc")
	if err := os.MkdirAll(sdlc, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "project:\n  name: \"" + name + "\"\n  integration_branch: main\n"
	if err := os.WriteFile(filepath.Join(sdlc, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdlc, "state.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkConfig writes a raw .agent-sdlc/config.yaml (no state.db), for invalid-config
// cases.
func mkConfig(t *testing.T, dir, content string) {
	t.Helper()
	sdlc := filepath.Join(dir, ".agent-sdlc")
	if err := os.MkdirAll(sdlc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdlc, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustCanon(t *testing.T, p string) string {
	t.Helper()
	c, err := canonicalPath(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func idsOf(ps []Project) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

// A. Explicit project only: existing behavior is unchanged. The project is
// exposed under its declared name, and no workspace is scanned.
func TestResolveExplicitProjectOnly(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "some-dir")
	mkProject(t, proj, "explicit")

	got, diags, err := ResolveProjects([]string{proj}, nil, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].ID != "explicit" || got[0].Root != mustCanon(t, proj) {
		t.Fatalf("got = %+v, want one project named explicit at %s", got, proj)
	}
}

// An explicit project with no config.yaml keeps its backward-compatible
// directory-name identity and is still accepted (the store validates state.db).
func TestResolveExplicitProjectWithoutConfigKeepsBasename(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "bare-project")
	if err := os.MkdirAll(filepath.Join(proj, ".agent-sdlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, _, err := ResolveProjects([]string{proj}, nil, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "bare-project" {
		t.Fatalf("got = %+v, want basename identity", got)
	}
}

// B. Workspace discovery: workspace/project-a is found.
func TestDiscoverWorkspaceProject(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "project-a"), "project-a")

	got, diags := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].ID != "project-a" {
		t.Fatalf("got = %+v, want project-a", got)
	}
}

// C. Nested workspace discovery: workspace/projects/project-b is found.
func TestDiscoverNestedWorkspaceProject(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "projects", "project-b"), "project-b")

	got, diags := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].ID != "project-b" {
		t.Fatalf("got = %+v, want project-b", got)
	}
}

// D. Non-SOP directories are ignored without producing diagnostics.
func TestDiscoverIgnoresNonProjects(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "random-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "git-repo-without-agent-sdlc", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, diags := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(got) != 0 {
		t.Fatalf("discovered non-projects: %v", idsOf(got))
	}
	if len(diags) != 0 {
		t.Fatalf("non-projects should be silent: %v", diags)
	}
}

// E. Explicit + discovered reach the same root: one project, not two.
func TestResolveDeduplicatesByRoot(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "project-a")
	mkProject(t, proj, "project-a")

	got, diags, err := ResolveProjects([]string{proj}, []string{ws}, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].ID != "project-a" {
		t.Fatalf("got = %+v, want exactly one project-a", got)
	}
}

// F. Multiple projects are all exposed, explicit first then discovered.
func TestResolveMultipleProjects(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "alpha"), "alpha")
	mkProject(t, filepath.Join(ws, "group", "beta"), "beta")
	explicit := filepath.Join(t.TempDir(), "gamma")
	mkProject(t, explicit, "gamma")

	got, _, err := ResolveProjects([]string{explicit}, []string{ws}, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d projects (%v), want 3", len(got), idsOf(got))
	}
	want := map[string]bool{"alpha": true, "beta": true, "gamma": true}
	for _, id := range idsOf(got) {
		if !want[id] {
			t.Errorf("unexpected project id %q", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Errorf("missing projects: %v", want)
	}
	if got[0].ID != "gamma" {
		t.Errorf("explicit project should come first, got %q", got[0].ID)
	}
}

// G. One invalid config must not hide a valid project; it yields a diagnostic.
func TestDiscoverInvalidConfigYieldsDiagnostic(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "good"), "good")
	mkConfig(t, filepath.Join(ws, "bad"), "version: 1\n# no project section\n")

	got, diags, err := ResolveProjects(nil, []string{ws}, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("valid project lost: %v", idsOf(got))
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Reason, "invalid SOP config") {
		t.Fatalf("expected one invalid-config diagnostic, got %v", diags)
	}
}

// A config that is valid but has no initialized state.db is reported, not bound.
func TestDiscoverMissingStateIsDiagnostic(t *testing.T) {
	ws := t.TempDir()
	mkConfig(t, filepath.Join(ws, "configured"), "project:\n  name: \"configured\"\n")

	got, diags := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(got) != 0 {
		t.Fatalf("uninitialized project was bound: %v", idsOf(got))
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Reason, "state.db") {
		t.Fatalf("expected a missing-state diagnostic, got %v", diags)
	}
}

// H. Two distinct roots claiming the same identity is a deterministic conflict:
// neither is bound, and the error names both roots.
func TestResolveDuplicateIdentityConflicts(t *testing.T) {
	ws1, ws2 := t.TempDir(), t.TempDir()
	mkProject(t, filepath.Join(ws1, "one"), "dup")
	mkProject(t, filepath.Join(ws2, "two"), "dup")

	got, _, err := ResolveProjects(nil, []string{ws1, ws2}, DefaultMaxDepth)
	if err == nil {
		t.Fatalf("expected a duplicate-identity conflict, got projects %v", idsOf(got))
	}
	if got != nil {
		t.Errorf("no project should be bound on conflict, got %v", idsOf(got))
	}
	if !strings.Contains(err.Error(), "dup") ||
		!strings.Contains(err.Error(), mustCanon(t, filepath.Join(ws1, "one"))) ||
		!strings.Contains(err.Error(), mustCanon(t, filepath.Join(ws2, "two"))) {
		t.Errorf("conflict error should name both roots: %v", err)
	}
}

// I. Symlinks are never followed: a symlink loop does not recurse, and a symlink
// escaping the workspace root exposes no project.
func TestDiscoverDoesNotFollowSymlinks(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "real"), "real")

	// A self-referential loop inside the workspace.
	if err := os.Symlink(ws, filepath.Join(ws, "loop")); err != nil {
		t.Fatal(err)
	}
	// A symlink pointing to a valid project outside the workspace root.
	outside := t.TempDir()
	mkProject(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(ws, "escape")); err != nil {
		t.Fatal(err)
	}

	got, diags := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].ID != "real" {
		t.Fatalf("symlinks must not add projects, got %v", idsOf(got))
	}
}

// J. Ignored directories are never traversed, even when they contain a project.
func TestDiscoverSkipsIgnoredDirectories(t *testing.T) {
	ws := t.TempDir()
	for _, dir := range []string{"node_modules", "vendor", ".git", "dist", "build", "target", ".cache"} {
		mkProject(t, filepath.Join(ws, dir, "hidden-project"), "hidden-"+dir)
	}
	mkProject(t, filepath.Join(ws, "visible"), "visible")

	got, _ := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(got) != 1 || got[0].ID != "visible" {
		t.Fatalf("ignored directories were traversed: %v", idsOf(got))
	}
}

// Discovery is depth-bounded: a project nested deeper than the limit is not found.
func TestDiscoverRespectsMaxDepth(t *testing.T) {
	ws := t.TempDir()
	// depth 4 (workspace/a/b/c/project) is within the limit.
	mkProject(t, filepath.Join(ws, "a", "b", "c", "shallow"), "shallow")
	// depth 6 (workspace/a/b/c/d/e/deep) is beyond it.
	mkProject(t, filepath.Join(ws, "a", "b", "c", "d", "e", "deep"), "deep")

	got, _ := DiscoverProjects([]string{ws}, DefaultMaxDepth)
	if len(got) != 1 || got[0].ID != "shallow" {
		t.Fatalf("depth bound not applied: %v", idsOf(got))
	}
}

// A missing workspace root is a diagnostic, never a crash or an error.
func TestDiscoverMissingWorkspaceRootIsDiagnostic(t *testing.T) {
	got, diags := DiscoverProjects([]string{filepath.Join(t.TempDir(), "nope")}, DefaultMaxDepth)
	if len(got) != 0 {
		t.Fatalf("got projects from a missing root: %v", idsOf(got))
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic for a missing root, got %v", diags)
	}
}

// Identity prefers the declared name and falls back to the directory name.
func TestProjectIDPrefersNameThenBasename(t *testing.T) {
	named := filepath.Join(t.TempDir(), "directory-name")
	mkProject(t, named, "declared-name")
	if id := ProjectID(named); id != "declared-name" {
		t.Errorf("ProjectID = %q, want declared-name", id)
	}

	unnamed := filepath.Join(t.TempDir(), "dir-only")
	mkConfig(t, unnamed, "project:\n  integration_branch: main\n")
	if id := ProjectID(unnamed); id != "dir-only" {
		t.Errorf("ProjectID = %q, want dir-only", id)
	}
}

// The config parser accepts a project section and rejects one without it.
func TestReadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	mkProject(t, dir, "parsed")
	cfg, err := ReadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "parsed" || cfg.Branch != "main" {
		t.Fatalf("config = %+v, want parsed/main", cfg)
	}

	bad := filepath.Join(t.TempDir(), "bad")
	mkConfig(t, bad, "version: 1\n")
	if _, err := ReadProjectConfig(bad); err == nil {
		t.Error("a config with no project section should be rejected")
	}
}

// Workspace roots are sufficient on their own: configuring only
// SOP_CONTROLLER_WORKSPACES does not force the current directory as a project.
func TestLoadWorkspacesWithoutProjects(t *testing.T) {
	t.Setenv("SOP_CONTROLLER_PROJECTS", "")
	t.Setenv("SOP_CONTROLLER_WORKSPACES", "/one,/two")
	cfg := Load()
	if len(cfg.ProjectRoots) != 0 {
		t.Errorf("ProjectRoots = %v, want empty when only workspaces are set", cfg.ProjectRoots)
	}
	if len(cfg.Workspaces) != 2 || cfg.Workspaces[0] != "/one" || cfg.Workspaces[1] != "/two" {
		t.Errorf("Workspaces = %v, want [/one /two]", cfg.Workspaces)
	}
}

// With neither variable set, the controller still observes the current
// directory as before.
func TestLoadDefaultsToCurrentDirectory(t *testing.T) {
	t.Setenv("SOP_CONTROLLER_PROJECTS", "")
	t.Setenv("SOP_CONTROLLER_WORKSPACES", "")
	cfg := Load()
	if len(cfg.ProjectRoots) != 1 || cfg.ProjectRoots[0] != "." {
		t.Errorf("ProjectRoots = %v, want [.]", cfg.ProjectRoots)
	}
	if len(cfg.Workspaces) != 0 {
		t.Errorf("Workspaces = %v, want empty", cfg.Workspaces)
	}
}

// Explicit projects and workspace roots merge.
func TestLoadMergesProjectsAndWorkspaces(t *testing.T) {
	t.Setenv("SOP_CONTROLLER_PROJECTS", "/p1,/p2")
	t.Setenv("SOP_CONTROLLER_WORKSPACES", "/w1")
	cfg := Load()
	if len(cfg.ProjectRoots) != 2 || len(cfg.Workspaces) != 1 {
		t.Errorf("merged config = roots %v workspaces %v", cfg.ProjectRoots, cfg.Workspaces)
	}
}

// The discovery depth is configurable; an unset, non-numeric, or non-positive
// value falls back to the default.
func TestLoadDiscoveryDepth(t *testing.T) {
	t.Setenv("SOP_CONTROLLER_DISCOVERY_DEPTH", "7")
	if cfg := Load(); cfg.DiscoveryDepth != 7 {
		t.Errorf("DiscoveryDepth = %d, want 7", cfg.DiscoveryDepth)
	}
	for _, bad := range []string{"", "0", "-3", "deep"} {
		t.Setenv("SOP_CONTROLLER_DISCOVERY_DEPTH", bad)
		if cfg := Load(); cfg.DiscoveryDepth != DefaultMaxDepth {
			t.Errorf("DiscoveryDepth(%q) = %d, want default %d", bad, cfg.DiscoveryDepth, DefaultMaxDepth)
		}
	}
}

// A configured depth is honored: raising it finds deeper projects, lowering it
// hides them.
func TestResolveProjectsHonorsDepth(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "a", "b", "deep"), "deep")

	if got, _, err := ResolveProjects(nil, []string{ws}, 2); err != nil || len(got) != 0 {
		t.Fatalf("depth 2: got %v err %v, want no projects", idsOf(got), err)
	}
	if got, _, err := ResolveProjects(nil, []string{ws}, 3); err != nil || len(got) != 1 || got[0].ID != "deep" {
		t.Fatalf("depth 3: got %v err %v, want deep", idsOf(got), err)
	}
}

// ResolveReport carries the configuration and outcome for the /discovery view.
func TestResolveReport(t *testing.T) {
	ws := t.TempDir()
	mkProject(t, filepath.Join(ws, "ok"), "ok")
	mkConfig(t, filepath.Join(ws, "bad"), "version: 1\n")
	explicit := filepath.Join(t.TempDir(), "exp")
	mkProject(t, explicit, "exp")

	rep, err := ResolveReport([]string{explicit}, []string{ws}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.MaxDepth != 5 || len(rep.Workspaces) != 1 || len(rep.Explicit) != 1 {
		t.Fatalf("report config not carried: %+v", rep)
	}
	ids := idsOf(rep.Projects)
	if len(ids) != 2 || ids[0] != "exp" || ids[1] != "ok" {
		t.Fatalf("report projects = %v, want [exp ok]", ids)
	}
	if len(rep.Diagnostics) != 1 {
		t.Fatalf("report diagnostics = %v, want one", rep.Diagnostics)
	}
}
