package security

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// detectFixture detects one testdata fixture directory.
func detectFixture(t *testing.T, name string) Detection {
	t.Helper()
	root := "testdata/detect/" + name
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatalf("DetectRepository(%q): %v", root, err)
	}
	return detection
}

func TestDetectGoOnly(t *testing.T) {
	d := detectFixture(t, "go-only")
	if !d.HasLanguage(LanguageGo) {
		t.Fatalf("languages = %v, want go", d.Languages)
	}
	if d.HasLanguage(LanguageRust) {
		t.Fatalf("languages = %v, must not contain rust", d.Languages)
	}
	present := d.PresentLockfiles()
	if len(present) != 1 || present[0].Path != "go.sum" {
		t.Fatalf("present lockfiles = %+v, want go.sum", present)
	}
	if missing := d.MissingLockfiles(); len(missing) != 0 {
		t.Fatalf("missing lockfiles = %+v, want none", missing)
	}
}

func TestDetectRustOnly(t *testing.T) {
	d := detectFixture(t, "rust-only")
	if !d.HasLanguage(LanguageRust) {
		t.Fatalf("languages = %v, want rust", d.Languages)
	}
	// A Rust-only tree has no go.mod, so it must report no Go language.
	if d.HasLanguage(LanguageGo) {
		t.Fatalf("languages = %v, must not contain go", d.Languages)
	}
	present := d.PresentLockfiles()
	if len(present) != 1 || present[0].Path != "Cargo.lock" {
		t.Fatalf("present lockfiles = %+v, want Cargo.lock", present)
	}
}

func TestDetectMixed(t *testing.T) {
	d := detectFixture(t, "mixed")
	if !d.HasLanguage(LanguageGo) || !d.HasLanguage(LanguageRust) {
		t.Fatalf("languages = %v, want go and rust", d.Languages)
	}
	want := []string{"Cargo.lock", "ffi/Cargo.lock", "go.sum"}
	var got []string
	for _, lf := range d.PresentLockfiles() {
		got = append(got, lf.Path)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("present lockfiles = %v, want %v", got, want)
	}
}

func TestDetectCargoWorkspaceLockfiles(t *testing.T) {
	d := detectFixture(t, "cargo-workspace")
	if len(d.Workspaces) != 1 {
		t.Fatalf("workspaces = %+v, want one", d.Workspaces)
	}
	ws := d.Workspaces[0]
	if ws.Manifest != "Cargo.toml" {
		t.Fatalf("workspace manifest = %q, want Cargo.toml", ws.Manifest)
	}
	wantMembers := []string{"crates/alpha", "crates/beta"}
	if !reflect.DeepEqual(ws.Resolved, wantMembers) {
		t.Fatalf("resolved members = %v, want %v", ws.Resolved, wantMembers)
	}

	// A Cargo workspace has one lockfile at the workspace root. Member
	// directories do not own a Cargo.lock, even when a stale file is present.
	present := map[string]bool{}
	for _, lf := range d.PresentLockfiles() {
		present[lf.Path] = true
	}
	if !present["Cargo.lock"] {
		t.Fatalf("missing workspace root Cargo.lock; present = %v", present)
	}
	if present["crates/alpha/Cargo.lock"] || present["crates/beta/Cargo.lock"] {
		t.Fatalf("workspace member lockfiles must not be treated as owned lockfiles; present = %v", present)
	}
	for _, lf := range d.Lockfiles {
		if lf.Language == LanguageRust && lf.Path != "Cargo.lock" {
			t.Fatalf("workspace reported unexpected Rust lockfile %q", lf.Path)
		}
	}
}

func TestDetectMissingLockfileReported(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \"missing\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, lf := range d.MissingLockfiles() {
		if lf.Path == "Cargo.lock" && !lf.Present {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing Cargo.lock was not reported: %+v", d.Lockfiles)
	}
	for _, lf := range d.PresentLockfiles() {
		if lf.Path == "Cargo.lock" {
			t.Fatalf("missing Cargo.lock was reported present")
		}
	}
}

func TestDetectStableAcrossRuns(t *testing.T) {
	for _, name := range []string{"go-only", "rust-only", "mixed", "cargo-workspace"} {
		first := detectFixture(t, name)
		second := detectFixture(t, name)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("fixture %q: detection differs across runs:\n%+v\n%+v", name, first, second)
		}
	}
}

func TestSelectCandidatesDeterministic(t *testing.T) {
	d := detectFixture(t, "mixed")
	configured := []config.SecurityAdapterConfig{
		{ID: "semgrep", Category: config.SecurityCategorySAST, Rules: "testdata/real/rules.yaml"},
	}
	first := SelectCandidates(d, configured)
	second := SelectCandidates(d, configured)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("selection differs across runs:\n%+v\n%+v", first, second)
	}
}

func TestSelectRustOnlyRejectsGoRuleset(t *testing.T) {
	d := detectFixture(t, "rust-only")
	configured := []config.SecurityAdapterConfig{
		{ID: "semgrep", Category: config.SecurityCategorySAST, Rules: "testdata/real/rules.yaml"},
	}
	for _, candidate := range SelectCandidates(d, configured) {
		if candidate.ID != AdapterIDSemgrep {
			continue
		}
		if candidate.Rules != "" {
			t.Fatalf("rust-only selected a Go-limited Semgrep ruleset %q", candidate.Rules)
		}
		if candidate.Status != CapabilityUnsupported {
			t.Fatalf("rust-only Semgrep status = %q, want unsupported", candidate.Status)
		}
	}
}

func TestSelectGoOnlySelectsGoRuleset(t *testing.T) {
	d := detectFixture(t, "go-only")
	configured := []config.SecurityAdapterConfig{
		{ID: "semgrep", Category: config.SecurityCategorySAST, Rules: "testdata/real/rules.yaml"},
	}
	var got string
	for _, candidate := range SelectCandidates(d, configured) {
		if candidate.ID == AdapterIDSemgrep {
			got = candidate.Rules
		}
	}
	if got != "testdata/real/rules.yaml" {
		t.Fatalf("go-only semgrep rules = %q, want testdata/real/rules.yaml", got)
	}
}

func TestClassifyCapabilitiesVocabulary(t *testing.T) {
	d := detectFixture(t, "mixed")
	configured := []config.SecurityAdapterConfig{
		{ID: "semgrep", Category: config.SecurityCategorySAST, Command: []string{"semgrep-does-not-exist"}},
		{ID: "gitleaks", Category: config.SecurityCategorySecrets, Command: []string{"gitleaks-does-not-exist"}},
		{ID: "unknown-builtin", Category: config.SecurityCategorySAST, Command: []string{"nope"}},
	}
	got := ClassifyCapabilities(d, configured)
	if len(got) != len(configured) {
		t.Fatalf("classified %d adapters, want %d", len(got), len(configured))
	}
	for _, candidate := range got {
		if !isKnownCapabilityStatus(candidate.Status) {
			t.Fatalf("adapter %q has status %q outside the allowed vocabulary", candidate.ID, candidate.Status)
		}
	}
}

func TestClassifyCapabilitiesProposedAndUnsupported(t *testing.T) {
	// No languages detected means a built-in scanner cannot cover the content.
	empty := Detection{}
	got := ClassifyCapabilities(empty, []config.SecurityAdapterConfig{
		{ID: "semgrep", Category: config.SecurityCategorySAST, Command: []string{"semgrep"}},
	})
	if len(got) != 1 || got[0].Status != CapabilityUnsupported {
		t.Fatalf("classification = %+v, want unsupported", got)
	}

	d := detectFixture(t, "rust-only")
	proposed := SelectCandidatesWithDatabase(d, nil, cargoDatabaseFixture(t))
	statuses := map[string]CapabilityStatus{}
	for _, candidate := range proposed {
		statuses[candidate.ID] = candidate.Status
	}
	if statuses[AdapterIDGitleaks] != CapabilityProposed {
		t.Fatalf("gitleaks status = %q, want proposed", statuses[AdapterIDGitleaks])
	}
	if statuses[AdapterIDSemgrep] != CapabilityUnsupported {
		t.Fatalf("semgrep status without rules = %q, want unsupported", statuses[AdapterIDSemgrep])
	}
	if statuses[AdapterIDCargoAudit] != CapabilityProposed {
		t.Fatalf("cargo-audit status with database = %q, want proposed", statuses[AdapterIDCargoAudit])
	}
}

func TestDetectSourceWithoutManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasLanguage(LanguageGo) {
		t.Fatalf("languages = %v, want Go from source content", d.Languages)
	}
	if len(d.Lockfiles) != 0 {
		t.Fatalf("lockfiles = %+v, want none without a manifest", d.Lockfiles)
	}
}

func TestAutoAdapterSelectionFollowsContent(t *testing.T) {
	for name, wantCargo := range map[string]bool{"go-only": false, "rust-only": true, "mixed": true} {
		d := detectFixture(t, name)
		adapters := AutoAdapterConfigs(d, nil, "advisory-db")
		ids := map[string]bool{}
		for _, adapter := range adapters {
			ids[adapter.ID] = true
		}
		if !ids[AdapterIDSemgrep] || !ids[AdapterIDGitleaks] {
			t.Errorf("%s: adapters = %v, want semgrep and gitleaks", name, ids)
		}
		if ids[AdapterIDCargoAudit] != wantCargo {
			t.Errorf("%s: cargo-audit selected = %t, want %t", name, ids[AdapterIDCargoAudit], wantCargo)
		}
	}
}

func TestAutoAdapterKeepsExplicitCategory(t *testing.T) {
	d := detectFixture(t, "rust-only")
	configured := []config.SecurityAdapterConfig{{
		ID: "private-deps", Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolExternal, Command: []string{"/opt/private-deps"},
	}}
	adapters := AutoAdapterConfigs(d, configured, "advisory-db")
	var sawPrivate, sawCargo bool
	for _, adapter := range adapters {
		switch adapter.ID {
		case "private-deps":
			sawPrivate = true
		case AdapterIDCargoAudit:
			sawCargo = true
		}
	}
	if !sawPrivate {
		t.Fatal("explicit dependency adapter was removed")
	}
	if sawCargo {
		t.Fatal("auto mode replaced an explicit dependency category with cargo-audit")
	}
}
