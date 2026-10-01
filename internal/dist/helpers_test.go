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
	"path/filepath"
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
