package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// phase8CorePackages are the Phase 8 subsystems that must stay provider/model neutral:
// the deterministic harness owns policy and reason, and provider/model behavior belongs
// behind the canonical capability interface.
var phase8CorePackages = []string{
	"internal/context",
	"internal/repoindex",
	"internal/retrieval",
	"internal/retrievalgate",
	"internal/prompt",
	"internal/normalize",
	"internal/verifcache",
	"internal/promptcache",
	"internal/vectoreval",
	"internal/decisionmemory",
	"internal/adaptiveroute",
	"internal/prompttuning",
}

// providerImplementationPrefixes name the packages that host provider-specific
// implementations. A Phase 8 core package must not import them: it may depend on the
// canonical capability interface (internal/agent) and the routing vocabulary
// (internal/model), but not on a concrete provider.
var providerImplementationPrefixes = []string{
	"github.com/imhttran/agentic-sop/internal/provider/",
	"github.com/imhttran/agentic-sop/internal/ollamaagent",
}

// moduleRoot is the repository root, derived from this test file's location.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source file")
	}
	// <root>/internal/archtest/<file> -> <root>
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// TestPhase8CoreDoesNotImportProviders is a Go import-graph check: it parses every
// Phase 8 core source file and fails if any imports a provider implementation package.
func TestPhase8CoreDoesNotImportProviders(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	for _, pkg := range phase8CorePackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s/%s: %v", pkg, entry.Name(), err)
			}
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					continue
				}
				for _, bad := range providerImplementationPrefixes {
					if path == bad || strings.HasPrefix(path, bad) {
						t.Errorf("%s/%s imports provider implementation %q; a Phase 8 core package must depend only on the canonical capability interface",
							pkg, entry.Name(), path)
					}
				}
			}
		}
	}
}

// TestProviderImplementationsExist proves the check is meaningful: the forbidden
// packages must actually exist, so the rule is not vacuously satisfied.
func TestProviderImplementationsExist(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range []string{"internal/provider/ollama", "internal/ollamaagent"} {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil || !info.IsDir() {
			t.Errorf("expected provider package %s to exist (err=%v)", rel, err)
		}
	}
}
