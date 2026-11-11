package archtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestCommandContextBindsProcessGroup guards OWASP LLM10 process-group termination:
// a file that starts a subprocess with exec.CommandContext must bind a process group
// (procgroup.Bind in that file), or sit in a package with its own Setpgid helper, so
// a timeout kills the whole tree and not only the direct child.
func TestCommandContextBindsProcessGroup(t *testing.T) {
	root := moduleRoot(t)
	type file struct {
		path, src string
	}
	var starts []file
	helper := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		dir := filepath.Dir(path)
		if strings.Contains(src, "Setpgid") {
			helper[dir] = true
		}
		if strings.Contains(src, "exec.CommandContext(") {
			starts = append(starts, file{path, src})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Known gaps, each with its reason. Remove an entry when the file binds a group.
	// internal/decision/command is a SEAM-005 leaf (see decision_seam_test.go), which
	// allows no import beyond internal/decision; binding it needs an operator decision.
	exempt := map[string]bool{"internal/decision/command/command.go": true}

	var missing []string
	for _, f := range starts {
		rel, _ := filepath.Rel(root, f.path)
		rel = filepath.ToSlash(rel)
		if exempt[rel] || strings.Contains(f.src, "procgroup.Bind(") || helper[filepath.Dir(f.path)] {
			continue
		}
		missing = append(missing, rel)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("subprocesses started with exec.CommandContext without a process group "+
			"(call procgroup.Bind on the command): %s", strings.Join(missing, ", "))
	}
}
