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

// This file is the frozen architecture guard contract for the agentic-sop core.
//
// The invariant it enforces: a core SOP or policy/governance package may depend on the
// canonical capability interface (internal/agent) and the routing vocabulary
// (internal/model), but must never acquire a direct dependency on a provider
// implementation or on any other declared forbidden direct dependency. Provider
// behavior belongs behind the canonical capability interface, so future providers can be
// introduced without leaking into policy and core reasoning.
//
// The guard is stdlib-only (go/parser import-graph scan), offline, and CI-safe: it needs
// no live provider, no network, and no credentials.

// corePackages is the union of the pre-existing Phase 8 core inventory and the
// policy/governance inventory. Every declared package must exist on disk (asserted by
// TestCorePackageDirectoriesExist) so the rule can never be vacuously satisfied.
var corePackages = append(append([]string{}, phase8CorePackages...), policyCorePackages...)

// policyCorePackages are the policy/governance packages that own allow/deny and gate
// decisions. They must stay free of direct dependencies on known specialized providers:
// policy is decided by declared capabilities and evidence, never by a provider name.
var policyCorePackages = []string{
	"internal/commandpolicy",
	"internal/approval",
	"internal/commitgate",
	"internal/mergegate",
	"internal/autonomy",
	"internal/decision",
	"internal/quality",
}

// providerImplementation is one known specialized provider adapter.
type providerImplementation struct {
	// Name is the human-readable provider name used in violation messages.
	Name string
	// Reason states why the package is a provider implementation and must not be a
	// direct dependency of a core SOP or policy package.
	Reason string
}

// providerImplementations is the registry of known specialized providers, keyed by
// import path. Introducing a new provider adapter is a one-line declaration here; the
// guard then protects every core package from depending on it. The consistency test
// TestProviderRegistryCoversProviderAdapters walks internal/provider/* and fails if a
// subdirectory is neither declared here nor explicitly allow-listed as provider-neutral.
var providerImplementations = map[string]providerImplementation{
	"github.com/imhttran/agentic-sop/internal/provider/ollama": {
		Name:   "ollama",
		Reason: "Ollama is a concrete local model runtime adapter; core packages must depend on the canonical capability interface instead.",
	},
	"github.com/imhttran/agentic-sop/internal/provider/openai": {
		Name:   "openai",
		Reason: "OpenAI is a concrete remote provider adapter; core packages must depend on the canonical capability interface instead.",
	},
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp": {
		Name:   "llamacpp",
		Reason: "llama.cpp is a concrete local inference adapter; core packages must remain provider-neutral.",
	},
	"github.com/imhttran/agentic-sop/internal/provider/mlx": {
		Name:   "mlx",
		Reason: "MLX is a concrete Apple-silicon inference adapter; core packages must remain provider-neutral.",
	},
	"github.com/imhttran/agentic-sop/internal/provider/httpx": {
		Name:   "httpx",
		Reason: "httpx is a concrete HTTP transport adapter for providers; core packages must remain transport-neutral.",
	},
	"github.com/imhttran/agentic-sop/internal/ollamaagent": {
		Name:   "ollamaagent",
		Reason: "ollamaagent wires the Ollama runtime into the agent boundary; policy and core packages must not depend on it directly.",
	},
}

// providerNeutralAllowList names subpackages under internal/provider that are shared,
// provider-neutral contract plumbing rather than provider implementations. A subdirectory
// here is deliberately excluded from providerImplementations by TestProviderRegistryCoversProviderAdapters.
// The top-level internal/provider package itself is neutral contract code (provider.go,
// capability.go, registry.go, errors.go, validate.go, availability.go, health.go).
var providerNeutralAllowList = map[string]string{
	"command": "provider command plumbing shared by all adapters, not a provider implementation",
}

// forbiddenCategory classifies why a dependency is forbidden.
type forbiddenCategory string

const (
	// categoryProviderImplementation marks concrete specialized provider adapters.
	categoryProviderImplementation forbiddenCategory = "provider-implementation"
	// categoryForbiddenDirectDependency marks any other declared forbidden direct
	// dependency (concrete transports, adapters, or vendor SDKs) that must not leak into
	// core SOP or policy packages.
	categoryForbiddenDirectDependency forbiddenCategory = "forbidden-direct-dependency"
)

// forbiddenRule is the frozen rule shape the guard enforces over corePackages. Every
// rule names its Category and a human-readable Reason so violation messages explain why
// the dependency is forbidden, not merely that it is.
type forbiddenRule struct {
	Prefix   string
	Category forbiddenCategory
	Reason   string
}

// forbiddenRules is derived from providerImplementations plus any additional declared
// forbidden direct dependencies. Sourcing provider rules from the registry means adding
// one entry to providerImplementations protects against a new provider with no other
// code change.
func forbiddenRules() []forbiddenRule {
	rules := make([]forbiddenRule, 0, len(providerImplementations)+4)
	for path, impl := range providerImplementations {
		rules = append(rules, forbiddenRule{
			Prefix:   path,
			Category: categoryProviderImplementation,
			Reason:   impl.Reason,
		})
	}
	rules = append(rules, []forbiddenRule{
		{
			Prefix:   "github.com/imhttran/agentic-sop/internal/provider/",
			Category: categoryProviderImplementation,
			Reason:   "any package under internal/provider/ is a provider implementation and must not be a direct dependency of a core SOP or policy package",
		},
		{
			Prefix:   "github.com/imhttran/agentic-sop/internal/provider",
			Category: categoryForbiddenDirectDependency,
			Reason:   "core SOP and policy packages are provider-neutral and must not depend on the provider contract/registry package directly",
		},
	}...)
	return rules
}

// ruleMatches reports whether an import path is forbidden, returning the matching rule.
// A rule matches on exact path equality or on a subpackage prefix.
func ruleMatches(importPath string) (forbiddenRule, bool) {
	for _, rule := range forbiddenRules() {
		if importPath == rule.Prefix || strings.HasPrefix(importPath, rule.Prefix+"/") {
			return rule, true
		}
	}
	return forbiddenRule{}, false
}

// scanFileImports parses a single Go source file and returns its import paths. It reads
// only the import block (parser.ImportsOnly), so it does not require a type-checkable or
// syntactically complete file to detect a forbidden dependency.
func scanFileImports(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var imports []string
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		imports = append(imports, p)
	}
	return imports, nil
}

// scanSourceImports returns the import paths declared in a Go source string, letting the
// guard's detection logic be exercised without a real file on disk.
func scanSourceImports(src string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var imports []string
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		imports = append(imports, p)
	}
	return imports, nil
}

// forbiddenViolations reports the rules violated by a single import path.
func forbiddenViolations(importPath string) []forbiddenRule {
	if rule, ok := ruleMatches(importPath); ok {
		return []forbiddenRule{rule}
	}
	return nil
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

// TestArchitectureGuard fails if any core SOP or policy/governance package acquires a
// forbidden direct dependency or a provider-specific import. The message names the
// importing file, the offending import, and the rule's Reason.
func TestArchitectureGuard(t *testing.T) {
	root := moduleRoot(t)
	for _, pkg := range corePackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			imports, err := scanFileImports(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("parse %s/%s: %v", pkg, entry.Name(), err)
			}
			for _, path := range imports {
				for _, rule := range forbiddenViolations(path) {
					t.Errorf("%s/%s imports %q (%s); %s",
						pkg, entry.Name(), path, rule.Category, rule.Reason)
				}
			}
		}
	}
}

// TestCorePackageDirectoriesExist asserts every declared core and policy package
// directory exists, so the guard cannot silently protect nothing.
func TestCorePackageDirectoriesExist(t *testing.T) {
	root := moduleRoot(t)
	for _, pkg := range corePackages {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(pkg)))
		if err != nil || !info.IsDir() {
			t.Errorf("declared core package %s does not exist as a directory (err=%v)", pkg, err)
		}
	}
}

// TestPhase8CoveragePreserved asserts the expanded core inventory still contains every
// entry the original Phase 8 check protected, so the guard never regresses.
func TestPhase8CoveragePreserved(t *testing.T) {
	membership := make(map[string]bool, len(corePackages))
	for _, pkg := range corePackages {
		membership[pkg] = true
	}
	for _, pkg := range phase8CorePackages {
		if !membership[pkg] {
			t.Errorf("core inventory regressed: %s was protected before and must remain protected", pkg)
		}
	}
}

// TestProviderImplementationsExist proves the check is meaningful: the registered
// forbidden packages must actually exist, so the rule is not vacuously satisfied.
func TestProviderImplementationsExist(t *testing.T) {
	root := moduleRoot(t)
	for path := range providerImplementations {
		rel := strings.TrimPrefix(path, "github.com/imhttran/agentic-sop/")
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil || !info.IsDir() {
			t.Errorf("expected provider package %s to exist (err=%v)", rel, err)
		}
	}
}

// TestGuardDetectsRepresentativeForbiddenDependency proves the guard's detection logic
// actually fires: a synthetic source importing a known provider implementation (and a
// second provider package) must produce one violation per forbidden import naming the
// offending path.
func TestGuardDetectsRepresentativeForbiddenDependency(t *testing.T) {
	src := `package probe

import (
	_ "github.com/imhttran/agentic-sop/internal/provider/ollama"
	_ "github.com/imhttran/agentic-sop/internal/ollamaagent"
)
`
	imports, err := scanSourceImports(src)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}

	var violations []string
	for _, path := range imports {
		for range forbiddenViolations(path) {
			violations = append(violations, path)
		}
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations for the representative forbidden imports, got %d (%v)", len(violations), violations)
	}
	for _, want := range []string{
		"github.com/imhttran/agentic-sop/internal/provider/ollama",
		"github.com/imhttran/agentic-sop/internal/ollamaagent",
	} {
		found := false
		for _, got := range violations {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a violation identifying %q, got %v", want, violations)
		}
	}
}

// TestGuardAcceptsNeutralImports is the positive control: the canonical capability
// interface and the routing vocabulary must never be flagged.
func TestGuardAcceptsNeutralImports(t *testing.T) {
	for _, path := range []string{
		"github.com/imhttran/agentic-sop/internal/agent",
		"github.com/imhttran/agentic-sop/internal/model",
	} {
		if v := forbiddenViolations(path); len(v) != 0 {
			t.Errorf("neutral import %q must not be flagged, got %+v", path, v)
		}
	}
}

// TestProviderRegistryCoversProviderAdapters walks internal/provider/* subdirectories and
// fails if a subdirectory is neither declared in providerImplementations nor explicitly
// allow-listed as provider-neutral. Adding a provider adapter therefore forces a
// deliberate one-line registry declaration instead of silently escaping the guard.
func TestProviderRegistryCoversProviderAdapters(t *testing.T) {
	root := moduleRoot(t)
	providerDir := filepath.Join(root, "internal", "provider")
	entries, err := os.ReadDir(providerDir)
	if err != nil {
		t.Fatalf("read %s: %v", providerDir, err)
	}

	const modulePrefix = "github.com/imhttran/agentic-sop/internal/provider/"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := modulePrefix + entry.Name()
		_, declared := providerImplementations[path]
		_, allowed := providerNeutralAllowList[entry.Name()]
		if !declared && !allowed {
			t.Errorf("internal/provider/%s is neither a declared provider implementation nor allow-listed as provider-neutral; add it to providerImplementations or providerNeutralAllowList", entry.Name())
		}
	}

	// Unit-level negative case: classification is list-driven, so a hypothetical
	// provider directory not in either list would be rejected.
	if isClassified("hypothetical-new-provider") {
		t.Error("a provider directory absent from both declarations must be flagged as unclassified")
	}
}

// isClassified reports whether a provider subdirectory name is declared or allow-listed.
func isClassified(name string) bool {
	path := "github.com/imhttran/agentic-sop/internal/provider/" + name
	if _, ok := providerImplementations[path]; ok {
		return true
	}
	_, ok := providerNeutralAllowList[name]
	return ok
}
