package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SEAM-005 architecture guard for the provider-neutral decision seam.
//
// The decision contract (internal/decision) and its provider-neutral process
// adapter (internal/decision/command) must stay independent of execution-model
// routing, of any provider implementation, and of the lifecycle/approval/domain
// packages. The seam is a leaf: it can depend on nothing in this module except the
// contract itself. This is the structural half of "providers evaluate; SOP governs":
// the decision surface cannot reach model routing or governance state because it
// has no edge to them.

const moduleImportPrefix = "github.com/imhttran/agentic-sop/"

// internalImportPrefix is the module's own package prefix.
const internalImportPrefix = moduleImportPrefix + "internal/"

// decisionSeamPackages maps each seam package to the internal packages it MAY
// import. internal/decision is a leaf (no internal import); the process adapter may
// import only the contract it implements.
var decisionSeamPackages = map[string]map[string]bool{
	"internal/decision":         nil, // leaf: no internal import at all
	"internal/decision/command": {internalImportPrefix + "decision": true},
}

// TestDecisionSeamStaysProviderAndModelNeutral fails if a decision-seam production
// file imports an internal package it is not allowed to, which would couple the
// provider-neutral decision surface to execution-model routing, a provider
// implementation, or the lifecycle/approval/domain packages.
func TestDecisionSeamStaysProviderAndModelNeutral(t *testing.T) {
	root := moduleRoot(t)
	for pkg, allowed := range decisionSeamPackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			imports, err := scanFileImports(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("parse %s/%s: %v", pkg, entry.Name(), err)
			}
			for _, path := range imports {
				if !strings.HasPrefix(path, internalImportPrefix) {
					continue
				}
				if allowed != nil && allowed[path] {
					continue
				}
				t.Errorf("%s/%s imports %q; the decision seam must stay independent of model routing, providers, and governance state", pkg, entry.Name(), path)
			}
		}
	}
}

// TestDecisionSeamPackagesExist proves the guard protects real packages, so it can
// never be vacuously satisfied.
func TestDecisionSeamPackagesExist(t *testing.T) {
	root := moduleRoot(t)
	for pkg := range decisionSeamPackages {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(pkg)))
		if err != nil || !info.IsDir() {
			t.Errorf("declared decision-seam package %s does not exist as a directory (err=%v)", pkg, err)
		}
	}
}

// fixtureModuleDir is the SEAM-006 out-of-module fake provider.
const fixtureModuleDir = "testdata/fake-decision-provider"

// TestFakeDecisionProviderIsOutOfModule proves the SEAM-006 fixture is a separate Go
// module that imports no agentic-sop package and no third-party package. It can
// therefore stand in for any external adapter: the JSON protocol is all it shares,
// and it can never reach SOP policy, lifecycle, approval, or state.
func TestFakeDecisionProviderIsOutOfModule(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, filepath.FromSlash(fixtureModuleDir))

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("the fixture must be its own Go module: %v", err)
	}
	if strings.Contains(string(gomod), "agentic-sop") {
		t.Fatalf("the fixture module must not be part of the SOP module:\n%s", gomod)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", fixtureModuleDir, err)
	}
	seen := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		seen++
		imports, err := scanFileImports(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, path := range imports {
			if strings.HasPrefix(path, moduleImportPrefix) || strings.Contains(path, "agentic-sop") {
				t.Errorf("%s imports %q; the external fixture must import no SOP package", entry.Name(), path)
			}
			if first := strings.SplitN(path, "/", 2)[0]; strings.Contains(first, ".") {
				t.Errorf("%s imports non-stdlib %q; the fixture must be stdlib only", entry.Name(), path)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("%s has no Go source; the proof would be vacuous", fixtureModuleDir)
	}
}
