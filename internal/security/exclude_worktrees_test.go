package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// Registered worktrees created by other agent harnesses live under
// .claude/worktrees. They are separate checkouts, exactly like .worktrees, so
// their fixtures must not become this repository's dependency obligations. The
// exclusion has to stay narrow: the rest of .claude is ordinary repository content.

// repoExcludes loads the committed configuration's effective exclusion list.
func repoExcludes(t *testing.T) []string {
	t.Helper()
	cfg, err := config.Load(filepath.Join("..", "..", ".agent-sdlc", "config.yaml"))
	if err != nil {
		t.Fatalf("load repo config: %v", err)
	}
	excludes := cfg.SecurityExcludes()
	if len(excludes) == 0 {
		t.Fatal("repo config declares no security exclusions; the test must not pass vacuously")
	}
	return excludes
}

func TestRepoConfigExcludesClaudeWorktreesOnly(t *testing.T) {
	excludes := repoExcludes(t)
	have := map[string]bool{}
	for _, pattern := range excludes {
		have[pattern] = true
	}
	if !have[".claude/worktrees"] {
		t.Errorf("exclusions = %v, want .claude/worktrees", excludes)
	}
	if have[".claude"] {
		t.Errorf("exclusions = %v, want .claude itself still in scope", excludes)
	}
	if !have[".worktrees"] {
		t.Errorf("exclusions = %v, want the existing .worktrees convention kept", excludes)
	}
	if !have[".agent-sdlc"] || !have[".git"] {
		t.Errorf("exclusions = %v, want the default boundaries kept", excludes)
	}
}

func TestExcludeMatcherClaudeWorktreesIsNarrow(t *testing.T) {
	matcher := NewExcludeMatcher(repoExcludes(t))
	cases := []struct {
		path string
		want bool
	}{
		{".claude/worktrees/pr499/Cargo.toml", true},
		{".claude/worktrees/cargo-audit-neg/.run/reference/tool/Cargo.toml", true},
		{".claude/worktrees", true},
		// Not excluded: the rest of .claude is repository content, and a scanner
		// finding there must stay visible.
		{".claude", false},
		{".claude/skills/example/SKILL.md", false},
		{".claude/settings.json", false},
		// Ordinary source is untouched.
		{"internal/toolharness/harness.go", false},
		{"Cargo.toml", false},
	}
	for _, tc := range cases {
		if got := matcher.Excluded(tc.path); got != tc.want {
			t.Errorf("Excluded(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}

// writeTree materialises a repository tree for detection tests.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestNestedWorktreeFixtureDoesNotBecomeAnObligation proves the parent checkout's
// obligations ignore a registered worktree's fixture, while real Rust content in
// the repository stays detectable.
func TestNestedWorktreeFixtureDoesNotBecomeAnObligation(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":  "module example.com/parent\n\ngo 1.22\n",
		"main.go": "package main\n",
		// In-scope Rust content with its lockfile: a real obligation, satisfied.
		"Cargo.toml": "[package]\nname = \"parent\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "version = 3\n",
		// A registered worktree's negative-test fixture: manifest, no lockfile.
		".claude/worktrees/neg/.run/reference/tool/Cargo.toml": "[package]\nname = \"tool\"\nversion = \"0.1.0\"\n",
	})

	detection, err := DetectRepositoryScoped(root, repoExcludes(t))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, manifest := range detection.Manifests {
		if strings.HasPrefix(manifest.Path, ".claude/worktrees/") {
			t.Errorf("excluded worktree manifest was detected: %s", manifest.Path)
		}
	}
	for _, lockfile := range detection.Lockfiles {
		if strings.HasPrefix(lockfile.Path, ".claude/worktrees/") {
			t.Errorf("excluded worktree lockfile was recorded: %s", lockfile.Path)
		}
	}
	if !detection.HasLanguage(LanguageGo) || !detection.HasLanguage(LanguageRust) {
		t.Fatalf("languages = %v, want the in-scope Go and Rust content", detection.Languages)
	}
	for _, issue := range detection.Issues {
		if strings.Contains(issue, ".claude/worktrees") {
			t.Errorf("excluded worktree produced an issue: %s", issue)
		}
	}
}

// TestInScopeRustWithoutLockfileStillFailsClosed is the negative control: the
// exclusion must not be able to hide a real obligation in the repository itself.
func TestInScopeRustWithoutLockfileStillFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"crates/app/Cargo.toml":            "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		".claude/worktrees/neg/Cargo.toml": "[package]\nname = \"neg\"\nversion = \"0.1.0\"\n",
		".claude/worktrees/neg/Cargo.lock": "version = 3\n",
	})

	detection, err := DetectRepositoryScoped(root, repoExcludes(t))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	missing := false
	for _, lockfile := range detection.Lockfiles {
		if lockfile.Path == "crates/app/Cargo.lock" && !lockfile.Present {
			missing = true
		}
		if strings.HasPrefix(lockfile.Path, ".claude/worktrees/") {
			t.Errorf("excluded worktree lockfile was recorded: %s", lockfile.Path)
		}
	}
	if !missing {
		t.Errorf("in-scope Cargo.toml without a lockfile was not recorded as missing; lockfiles = %+v", detection.Lockfiles)
	}
}

// TestExcludedPathDoesNotSatisfyInScopeObligation proves an excluded manifest
// cannot stand in for missing in-scope content: the obligation is still owned by
// the repository's own manifest.
func TestExcludedPathDoesNotSatisfyInScopeObligation(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"crates/app/Cargo.toml":            "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		".claude/worktrees/neg/Cargo.toml": "[package]\nname = \"neg\"\nversion = \"0.1.0\"\n",
		".claude/worktrees/neg/Cargo.lock": "version = 3\n",
	})

	detection, err := DetectRepositoryScoped(root, repoExcludes(t))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, obligation := range detection.Obligations {
		if strings.Contains(obligation.Input, ".claude/worktrees") ||
			strings.Contains(obligation.Manifest, ".claude/worktrees") ||
			strings.Contains(obligation.Root, ".claude/worktrees") {
			t.Errorf("excluded path produced an obligation: %+v", obligation)
		}
	}
	owned := false
	for _, obligation := range detection.Obligations {
		if strings.HasPrefix(obligation.Input, "crates/app/") {
			owned = true
		}
	}
	if !owned {
		t.Errorf("the in-scope manifest owns no obligation; obligations = %+v", detection.Obligations)
	}
}
