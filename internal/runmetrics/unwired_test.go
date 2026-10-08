package runmetrics

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const importPath = "github.com/imhttran/agentic-sop/internal/runmetrics"

// TestNoProductionImportOfRunmetrics asserts that no package outside
// internal/runmetrics imports it, and that the testdata and test helper files
// are the only references. It walks the repository from the package root.
func TestNoProductionImportOfRunmetrics(t *testing.T) {
	root := repoRoot(t)

	fset := token.NewFileSet()
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".agent-sdlc", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		// The runmetrics package itself (including its tests) is allowed.
		if strings.HasPrefix(filepath.ToSlash(rel), "internal/runmetrics/") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, "\"") == importPath {
				offenders = append(offenders, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("production packages import %s: %v", importPath, offenders)
	}
}

// TestNoCLIModification checks that no file under internal/cli references the
// runmetrics package, so the library is wired to no CLI command.
func TestNoCLIModification(t *testing.T) {
	root := repoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	entries, err := os.ReadDir(cliDir)
	if err != nil {
		t.Fatalf("read internal/cli: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cliDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "runmetrics") {
			t.Errorf("internal/cli/%s references runmetrics", e.Name())
		}
	}
}

// repoRoot walks up from the test working directory to the module root (the
// directory holding go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate module root (go.mod)")
		}
		dir = parent
	}
}
