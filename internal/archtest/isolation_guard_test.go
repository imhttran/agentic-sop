package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This guard protects the W8 scanner baseline entry for `dangerous-exec-command`
// at internal/toolharness/harness.go (execCaptured). That entry is justified only
// while the mandatory isolation boundary (M1 + G3 + G4) actually holds for every
// model-reachable execution. A regression that adds a second execution path, or
// that lets a model-facing tool reach the sink without building the sandbox,
// would silently fall under the same baseline entry, so this fails the build
// instead.
//
// The check is stdlib-only (go/parser) and offline.

// sandboxExecAllowlist names every function in package toolharness that may start a
// process, with the reason each is safe. Adding a new name is a deliberate review
// decision, not an incidental edit.
var sandboxExecAllowlist = map[string]string{
	"execCaptured":              "the single model-reachable sink; every caller must be execIsolated",
	"gitProbe":                  "fixed read-only git query (check-attr/config) with hardened args; the model cannot choose argv and these subcommands run no repository-configured program",
	"SandboxRunnable":           "backend availability probe with a literal argv; executes only the platform sandbox tool",
	"probeProductionInvocation": "production-argv probe with a literal backend name; executes only the platform sandbox tool",
}

// modelReachableExecFunctions are the functions that implement the tools a model can
// call. Each must obtain its execution through execIsolated.
var modelReachableExecFunctions = []string{
	"runCommandWithStatus",
	"git",
	"execFingerprintCmd",
	"WorkingTreeChanged",
}

// sandboxedSinkCallers are the only functions allowed to call the sink directly:
// execIsolated builds the sandbox first, and execWithStatus is the test-only helper
// whose own production callers are checked separately.
var sandboxedSinkCallers = map[string]bool{"execIsolated": true, "execWithStatus": true}

type funcInfo struct {
	execs       []string
	calls       map[string]bool
	isolateLine int
	sinkLine    int
}

// isolationViolations analyses a set of Go source files (filename -> source) and
// returns one message per violation. It is pure so the guard can be tested against
// synthetic packages that each introduce exactly one regression.
func isolationViolations(sources map[string]string) ([]string, error) {
	fset := token.NewFileSet()
	funcs := map[string]*funcInfo{}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		file, err := parser.ParseFile(fset, name, sources[name], 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := funcs[fn.Name.Name]
			if info == nil {
				info = &funcInfo{calls: map[string]bool{}}
				funcs[fn.Name.Name] = info
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				pos := fset.Position(call.Pos()).Line
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					if ident, ok := fun.X.(*ast.Ident); ok && ident.Name == "exec" {
						if fun.Sel.Name == "Command" || fun.Sel.Name == "CommandContext" {
							info.execs = append(info.execs, "exec."+fun.Sel.Name)
							return true
						}
					}
					info.calls[fun.Sel.Name] = true
					if fun.Sel.Name == "execCaptured" && (info.sinkLine == 0 || pos < info.sinkLine) {
						info.sinkLine = pos
					}
				case *ast.Ident:
					info.calls[fun.Name] = true
					if fun.Name == "isolate" && (info.isolateLine == 0 || pos < info.isolateLine) {
						info.isolateLine = pos
					}
				}
				return true
			})
		}
	}

	var violations []string
	for _, name := range sortedKeys(funcs) {
		info := funcs[name]
		for _, site := range info.execs {
			if _, ok := sandboxExecAllowlist[name]; !ok {
				violations = append(violations, fmt.Sprintf("%s calls %s but is not in the sandbox exec allowlist", name, site))
			}
			if site == "exec.CommandContext" && name != "execCaptured" && name != "gitProbe" {
				violations = append(violations, fmt.Sprintf("%s calls exec.CommandContext; only execCaptured and the fixed-argv git probe may", name))
			}
		}
	}

	// Who calls the sink, and who calls the test-only helpers.
	for _, name := range sortedKeys(funcs) {
		info := funcs[name]
		if info.calls["execCaptured"] && !sandboxedSinkCallers[name] {
			violations = append(violations, fmt.Sprintf("%s calls execCaptured without going through execIsolated", name))
		}
		if info.calls["execWithStatus"] && name != "exec" {
			violations = append(violations, fmt.Sprintf("%s calls execWithStatus, which does not build the sandbox", name))
		}
		if info.calls["exec"] && name != "" {
			// `exec` itself is the test-only wrapper; any production caller reopens
			// the unsandboxed path.
			violations = append(violations, fmt.Sprintf("%s calls exec, which does not build the sandbox", name))
		}
	}

	isolated, ok := funcs["execIsolated"]
	if !ok {
		violations = append(violations, "execIsolated is missing; the single isolated execution entry point must exist")
	} else {
		if isolated.isolateLine == 0 {
			violations = append(violations, "execIsolated does not call isolate")
		}
		if isolated.sinkLine == 0 {
			violations = append(violations, "execIsolated does not call execCaptured")
		}
		if isolated.isolateLine != 0 && isolated.sinkLine != 0 && isolated.isolateLine > isolated.sinkLine {
			violations = append(violations, "execIsolated reaches the sink before calling isolate")
		}
	}

	for _, name := range modelReachableExecFunctions {
		info, ok := funcs[name]
		if !ok {
			violations = append(violations, fmt.Sprintf("model-facing function %s is missing", name))
			continue
		}
		if !info.calls["execIsolated"] {
			violations = append(violations, fmt.Sprintf("model-facing function %s does not execute through execIsolated", name))
		}
		if len(info.execs) > 0 {
			violations = append(violations, fmt.Sprintf("model-facing function %s starts a process directly (%s)", name, info.execs[0]))
		}
	}
	return violations, nil
}

func sortedKeys(m map[string]*funcInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// loadToolharnessProductionSources reads the non-test sources of package toolharness.
func loadToolharnessProductionSources(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), "internal", "toolharness")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || isTestFile(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sources[entry.Name()] = string(data)
	}
	if len(sources) == 0 {
		t.Fatalf("no production sources found in %s; the guard must not pass vacuously", dir)
	}
	return sources
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go")
}

// TestModelReachableExecIsIsolated is the guard the W8 baseline entry depends on.
func TestModelReachableExecIsIsolated(t *testing.T) {
	violations, err := isolationViolations(loadToolharnessProductionSources(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("the mandatory isolation boundary can be bypassed:\n  %s", joinLines(violations))
	}
}

func joinLines(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += "\n  "
		}
		out += item
	}
	return out
}

// The synthetic-package tests below are the guard's own positive and negative
// controls: each source introduces exactly one regression and must be reported.

const guardBaseline = `package toolharness

type Harness struct{}

func (h *Harness) execCaptured(argv []string) { _ = exec.CommandContext(nil, argv[0]) }
func (h *Harness) execWithStatus(argv []string) { h.execCaptured(argv) }
func (h *Harness) exec(argv []string) { h.execWithStatus(argv) }
func isolate(argv []string) {}
func (h *Harness) execIsolated(argv []string) {
	isolate(argv)
	h.execCaptured(argv)
}
func (h *Harness) runCommandWithStatus(argv []string) { h.execIsolated(argv) }
func (h *Harness) git(argv []string) { h.execIsolated(argv) }
func (h *Harness) execFingerprintCmd(argv []string) { h.execIsolated(argv) }
func (h *Harness) WorkingTreeChanged() { h.execIsolated(nil) }
func (h *Harness) gitProbe(argv []string) { _ = exec.CommandContext(nil, argv[0]) }
func SandboxRunnable() { _ = exec.Command("bwrap") }
func probeProductionInvocation() { _ = exec.Command("bwrap") }
`

func TestIsolationGuardAcceptsTheCompliantShape(t *testing.T) {
	violations, err := isolationViolations(map[string]string{"baseline.go": guardBaseline})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("the compliant shape was rejected:\n  %s", joinLines(violations))
	}
}

func TestIsolationGuardDetectsBypasses(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a new exec sink outside the allowlist",
			src:  strings.Replace(guardBaseline, "func (h *Harness) gitProbe", "func (h *Harness) sneaky(argv []string) { _ = exec.CommandContext(nil, argv[0]) }\n\nfunc (h *Harness) gitProbe", 1),
			want: "sneaky calls exec.CommandContext",
		},
		{
			name: "a model-facing tool calling the sink directly",
			src:  strings.Replace(guardBaseline, "func (h *Harness) git(argv []string) { h.execIsolated(argv) }", "func (h *Harness) git(argv []string) { h.execCaptured(argv) }", 1),
			want: "git calls execCaptured without going through execIsolated",
		},
		{
			name: "execIsolated skipping the sandbox",
			src:  strings.Replace(guardBaseline, "\tisolate(argv)\n\th.execCaptured(argv)", "\th.execCaptured(argv)", 1),
			want: "execIsolated does not call isolate",
		},
		{
			name: "execIsolated reaching the sink first",
			src: strings.Replace(guardBaseline, "\tisolate(argv)\n\th.execCaptured(argv)",
				"\th.execCaptured(argv)\n\tisolate(argv)", 1),
			want: "execIsolated reaches the sink before calling isolate",
		},
		{
			name: "a production caller of the test-only wrapper",
			src:  strings.Replace(guardBaseline, "func (h *Harness) git(argv []string) { h.execIsolated(argv) }", "func (h *Harness) git(argv []string) { h.exec(argv) }", 1),
			want: "git calls exec, which does not build the sandbox",
		},
		{
			name: "a production caller of execWithStatus",
			src:  strings.Replace(guardBaseline, "func (h *Harness) git(argv []string) { h.execIsolated(argv) }", "func (h *Harness) git(argv []string) { h.execWithStatus(argv) }", 1),
			want: "git calls execWithStatus, which does not build the sandbox",
		},
		{
			name: "a model-facing tool that stops using execIsolated",
			src:  strings.Replace(guardBaseline, "func (h *Harness) git(argv []string) { h.execIsolated(argv) }", "func (h *Harness) git(argv []string) {}", 1),
			want: "git does not execute through execIsolated",
		},
		{
			name: "the isolated entry point removed",
			src:  strings.Replace(guardBaseline, "func (h *Harness) execIsolated(argv []string) {\n\tisolate(argv)\n\th.execCaptured(argv)\n}", "", 1),
			want: "execIsolated is missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations, err := isolationViolations(map[string]string{"baseline.go": tc.src})
			if err != nil {
				t.Fatal(err)
			}
			joined := joinLines(violations)
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("want a violation containing %q, got:\n  %s", tc.want, joined)
			}
		})
	}
}
