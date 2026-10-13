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

// allowedImporters is the closed set of files outside internal/runmetrics that may
// reference runmetrics. It names exactly the approved, read-only `sop report
// --decisions` entry point and its test. Every other file must not import or
// reference runmetrics, so no production execution path (run, execute, routing,
// approval) can reach the aggregator.
var allowedImporters = map[string]bool{
	"internal/cli/report_decisions.go":      true,
	"internal/cli/report_decisions_test.go": true,
}

// TestNoProductionImportOfRunmetrics asserts that runmetrics is imported only by
// its own package and the approved read-only CLI entry point. Any other importer is
// an offender: the aggregator must stay off the production execution path.
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
		rel = filepath.ToSlash(rel)
		// The runmetrics package itself (including its tests) is allowed, as is the
		// approved read-only CLI entry point (and its test).
		if strings.HasPrefix(rel, "internal/runmetrics/") || allowedImporters[rel] {
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

// TestNoCLIModification asserts that the only files under internal/cli that
// reference runmetrics are the approved read-only report entry point and its test.
// Every other CLI file — in particular the run/execute/routing/approval path — must
// not reference it, so the aggregator cannot become a production dependency.
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
		if allowedImporters["internal/cli/"+e.Name()] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cliDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "runmetrics") {
			t.Errorf("internal/cli/%s references runmetrics; only the approved report entry point may", e.Name())
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
