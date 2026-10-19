package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// issueMentions reports whether any detection issue contains every fragment.
func issueMentions(issues []string, fragments ...string) bool {
	for _, issue := range issues {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(issue, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func unsafeWorkspaceReferenceFixture(t *testing.T, ref string) string {
	t.Helper()
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestFile(t, root, "crates/app/Cargo.toml",
		"[package]\nname = \"app\"\nversion = \"0.1.0\"\nworkspace = \""+ref+"\"\n")
	cargoTestFile(t, root, "crates/app/src/main.rs", "fn main() {}\n")
	cargoTestLock(t, root, ".")
	return root
}

// An absolute or repository-escaping package.workspace reference must surface as
// an explicit unsupported layout instead of being silently reinterpreted as an
// independent package.
func TestCargoUnsafeWorkspaceReferenceIsUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  string
	}{
		{name: "absolute", ref: "/tmp/outside"},
		{name: "escapes repository", ref: "../../../outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := unsafeWorkspaceReferenceFixture(t, tc.ref)
			detection, err := DetectRepository(root)
			if err != nil {
				t.Fatal(err)
			}
			if !issueMentions(detection.Issues, "package.workspace", "unsupported layout") {
				t.Fatalf("issues = %v, want an explicit unsupported-layout issue for %q", detection.Issues, tc.ref)
			}
			if gap := dependencyInventoryGap(detection, []Language{LanguageRust}, nil, ""); gap == "" {
				t.Fatal("readiness projection reported no gap for an unsupported layout")
			}
			for name, adapter := range map[string]Adapter{
				"cargo-audit": cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
					t.Fatal("cargo-audit must not run for an unsupported workspace reference")
					return nil, nil, 0, nil
				}),
				"osv": osvAdapterForTest(t, root, t.TempDir(), fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
					t.Fatal("osv-scanner must not run for an unsupported workspace reference")
					return nil, nil, 0, nil
				}}),
			} {
				exec, err := adapter.Scan(context.Background(), requestFor(t, root))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
					t.Fatalf("%s status/coverage = %s/%s, want failed and not complete (%v)", name, exec.Status, exec.Coverage, exec.Diagnostics)
				}
				if !strings.Contains(strings.Join(exec.Diagnostics, " "), "unsupported or ambiguous") {
					t.Errorf("%s diagnostics = %v, want an unsupported-layout diagnostic", name, exec.Diagnostics)
				}
				evaluation := EvaluatePolicy([]Execution{exec}, Policy{
					Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"},
					FailOn: []string{"high"}, UnknownSeverity: "indeterminate",
				})
				if evaluation.Verdict == VerdictPass {
					t.Errorf("%s required mode passed an unsupported layout: %+v", name, evaluation)
				}
			}
		})
	}
}

// Valid in-repository references must keep working, including a reference to a
// workspace root that is not an ancestor of the package (a nested workspace).
func TestCargoValidWorkspaceReferenceOutsideNestedRootIsPreserved(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "nested/ws/Cargo.toml", "[workspace]\nmembers = [\"../../pkg\"]\n")
	cargoTestFile(t, root, "pkg/Cargo.toml", "[package]\nname = \"pkg\"\nversion = \"0.1.0\"\nworkspace = \"../nested/ws\"\n")
	cargoTestFile(t, root, "pkg/src/lib.rs", "pub fn f() {}\n")
	cargoTestLock(t, root, "nested/ws")

	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Issues) != 0 {
		t.Fatalf("issues = %v, want a valid in-repository reference", detection.Issues)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"pkg"}) {
		t.Fatalf("workspaces = %+v, want the pkg member", detection.Workspaces)
	}
	present := detection.PresentLockfiles()
	if len(present) != 1 || present[0].Path != "nested/ws/Cargo.lock" {
		t.Fatalf("lockfiles = %+v, want nested/ws/Cargo.lock", present)
	}
}

// Cargo ignores ordinary files that match a members glob and reports only a
// matched directory that has no Cargo.toml. Discovery follows that behavior so a
// repository Cargo accepts is not rejected for an unrelated file.
func TestCargoMemberGlobIgnoresFilesAndReportsNonPackageDirectories(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestFile(t, root, "crates/app/src/main.rs", "fn main() {}\n")
	cargoTestFile(t, root, "crates/README.txt", "not a package\n")
	cargoTestFile(t, root, "crates/notes/readme.md", "not a package\n")
	cargoTestLock(t, root, ".")

	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if issueMentions(detection.Issues, "README.txt") {
		t.Fatalf("issues = %v, want ordinary files to be ignored like Cargo", detection.Issues)
	}
	if !issueMentions(detection.Issues, "crates/notes without Cargo.toml") {
		t.Fatalf("issues = %v, want the matched directory without Cargo.toml reported", detection.Issues)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"crates/app"}) {
		t.Fatalf("workspaces = %+v, want the valid package member preserved", detection.Workspaces)
	}
	if gap := dependencyInventoryGap(detection, []Language{LanguageRust}, nil, ""); gap == "" {
		t.Fatal("readiness projection reported no gap for a non-package member directory")
	}
}

// Discovery is pure Go: it must never invoke the Cargo toolchain.
func TestCargoDiscoveryDoesNotExecuteCargo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH shim is POSIX-only")
	}
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")

	bin := t.TempDir()
	marker := filepath.Join(bin, "cargo-invoked")
	script := fmt.Sprintf("#!/bin/sh\n: > %q\nexit 1\n", marker)
	if err := os.WriteFile(filepath.Join(bin, "cargo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := DetectRepository(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("dependency discovery executed Cargo")
	}
}

// Freshness is decided against a supplied clock, so the exact 90-day boundary
// and unknown metadata are deterministic.
func TestCargoDatabaseStaleBoundaryAndUnknownFreshness(t *testing.T) {
	updated := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		last  string
		now   time.Time
		stale bool
		known bool
	}{
		{name: "fresh", last: "2026-10-01T00:00:00Z", now: updated.Add(89 * 24 * time.Hour), known: true},
		{name: "exact boundary is fresh", last: "2026-10-01T00:00:00Z", now: updated.Add(cargoAuditStaleMaxAge), known: true},
		{name: "past boundary is stale", last: "2026-10-01T00:00:00Z", now: updated.Add(cargoAuditStaleMaxAge + time.Second), stale: true, known: true},
		{name: "empty is unknown", last: "", now: updated},
		{name: "malformed is unknown", last: "not-a-timestamp", now: updated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stale, known := cargoDatabaseStale(tc.last, tc.now)
			if stale != tc.stale || known != tc.known {
				t.Fatalf("stale/known = %t/%t, want %t/%t", stale, known, tc.stale, tc.known)
			}
		})
	}
}

// The tracked advisory-db fixture must stay fresh with the injected test clock
// and age predictably far from the real calendar date.
func TestCargoDatabaseFreshnessUsesInjectedClock(t *testing.T) {
	dir := cargoDatabaseFixture(t)
	if _, stale := cargoDatabaseFreshness(dir, cargoAuditTestNow); stale {
		t.Fatal("fixture database is stale at the fixed test clock")
	}
	if _, stale := cargoDatabaseFreshness(dir, cargoAuditTestNow.Add(120*24*time.Hour)); !stale {
		t.Fatal("fixture database did not age when the clock advanced")
	}
}

// Resume evidence must be invalidated when a previously fresh database ages,
// even though its content identity is unchanged.
func TestCargoAuditResumeInvalidatesWhenDatabaseAges(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter := cargoAdapterAt(t, cargoDatabaseFixture(t), "", cargoAuditTestNow,
		func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
		})
	req := requestFor(t, root)
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	current, err := adapter.(provenanceProvider).currentProvenance(req)
	if err != nil {
		t.Fatal(err)
	}
	current.PolicyIdentity = policy.Identity(Baseline{})
	prior := Scan{
		ProtocolVersion: ProtocolVersion,
		RunID:           "prior",
		Policy:          policy,
		Adapters: []Execution{{
			ProtocolVersion: ProtocolVersion,
			AdapterID:       AdapterIDCargoAudit,
			Category:        CategoryDependencies,
			Status:          StatusCompleted,
			Coverage:        CoverageComplete,
			Provenance:      current,
		}},
	}
	runner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "run", Adapters: []Adapter{adapter}, Policy: policy, Prior: &prior})
	if err != nil {
		t.Fatal(err)
	}
	if _, reused := runner.maybeReuse(req, adapter.Descriptor()); !reused {
		t.Fatal("fresh evidence was not reused")
	}
	built := adapter.(*cargoAuditAdapter)
	built.now = func() time.Time { return cargoAuditTestNow.Add(120 * 24 * time.Hour) }
	if _, reused := runner.maybeReuse(req, adapter.Descriptor()); reused {
		t.Fatal("aged database evidence was reused")
	}
}
