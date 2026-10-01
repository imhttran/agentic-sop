// Package dist validates SOP's distribution layer as an artifact: the installers
// (install.sh on macOS/Linux, install.ps1 on Windows), the per-agent skill installation
// they delegate to or perform, and the Claude Code plugin package under
// integrations/claude/.
//
// Every test runs against temporary directories — a scratch HOME, a scratch bin
// directory, and (for the plugin) the packaged tree itself. Nothing here touches the
// developer's real ~/.agents, ~/.claude, PATH, or installed sop binary, and nothing
// edits a shell startup file or a PowerShell profile.
package dist

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the checkout under test (internal/dist -> ../..).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// read returns a file's contents.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func isDirEmpty(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}

// goEnv reads one value from the Go environment. The tests pin the Go caches with it: the
// toolchain derives GOPATH and the build/module caches from the home directory, so an
// isolated HOME would otherwise send every build inside an installer to an empty cache —
// slow, and it leaves a read-only tree in the scratch home.
func goEnv(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// scratchHome returns a temporary home directory for an installer run. It is deliberately
// not t.TempDir: a Go build under a fresh HOME can leave a read-only module cache behind,
// which makes the framework's own TempDir cleanup fail the test. A tolerant cleanup keeps
// that out of the result, and the callers pin the Go caches elsewhere anyway.
func scratchHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("", "sop-dist-home-")
	if err != nil {
		t.Fatalf("scratch home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return home
}
