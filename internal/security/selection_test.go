package security

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

func boolPtr(value bool) *bool { return &value }

func mixedDetection(t *testing.T) Detection {
	t.Helper()
	return Detection{
		Root:      ".",
		rootAbs:   t.TempDir(),
		Languages: []Language{LanguageGo, LanguageRust},
		Manifests: []Manifest{
			{Path: "go.mod", Language: LanguageGo, Kind: "go.mod"},
			{Path: "Cargo.toml", Language: LanguageRust, Kind: "cargo.toml"},
		},
		Lockfiles: []Lockfile{
			{Path: "Cargo.lock", Language: LanguageRust, Present: true},
		},
	}
}

func selectedIDs(selection Selection) []string {
	var ids []string
	for _, adapter := range selection.Adapters {
		ids = append(ids, adapter.Config.ID)
	}
	return ids
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestSelectionGlobalDisablePreventsEveryAdapter(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled: boolPtr(false),
		Adapters: []config.SecurityAdapterConfig{
			{ID: "semgrep", Category: config.SecurityCategorySAST},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	if len(selection.Adapters) != 0 {
		t.Fatalf("adapters = %v, want none", selectedIDs(selection))
	}
	if len(selection.Disabled) != 1 || selection.Disabled[0].Reason == "" {
		t.Fatalf("disabled = %+v, want one reasoned entry", selection.Disabled)
	}
}

func TestSelectionCategoryDisableWinsOverAdapterEnable(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:    boolPtr(true),
		Categories: map[string]*bool{"sast": boolPtr(false)},
		Adapters: []config.SecurityAdapterConfig{
			{ID: "semgrep", Category: config.SecurityCategorySAST, Enabled: boolPtr(true), Command: []string{"semgrep"}},
			{ID: "gitleaks", Category: config.SecurityCategorySecrets, Command: []string{"gitleaks"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), false)
	if len(selection.Adapters) != 1 || selection.Adapters[0].Config.ID != "gitleaks" {
		t.Fatalf("adapters = %v, want only gitleaks", selectedIDs(selection))
	}
	if len(selection.Disabled) != 1 || selection.Disabled[0].ID != "semgrep" {
		t.Fatalf("disabled = %+v, want semgrep", selection.Disabled)
	}
}

func TestSelectionExplicitDisableIsNotUndoneByAuto(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:              boolPtr(true),
		Auto:                 true,
		AdvisoryDatabasePath: ".agent-sdlc/advisory-db",
		Adapters: []config.SecurityAdapterConfig{
			{ID: "semgrep", Category: config.SecurityCategorySAST, Command: []string{"semgrep"}},
			{ID: "gitleaks", Category: config.SecurityCategorySecrets, Command: []string{"gitleaks"}},
			{ID: "cargo-audit", Category: config.SecurityCategoryDependencies, Command: []string{"cargo", "audit"}, Enabled: boolPtr(false)},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	for _, adapter := range selection.Adapters {
		if adapter.Config.Category == config.SecurityCategoryDependencies {
			for _, language := range adapter.Languages {
				if language == LanguageRust {
					t.Fatalf("auto selection re-added disabled Rust dependency coverage: %v", selectedIDs(selection))
				}
			}
		}
	}
	found := false
	for _, disabled := range selection.Disabled {
		if disabled.ID == "cargo-audit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cargo-audit disable was not recorded: %+v", selection.Disabled)
	}
}

func TestSelectionAutoFillsGoWithOSVAndRustWithCargoAudit(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:              boolPtr(true),
		Auto:                 true,
		AdvisoryDatabasePath: ".agent-sdlc/advisory-db",
		OSVDatabasePath:      ".agent-sdlc/osv-db",
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	byID := map[string]SelectedAdapter{}
	for _, adapter := range selection.Adapters {
		byID[adapter.Config.ID] = adapter
	}
	osv, ok := byID[AdapterIDOSVScanner]
	if !ok {
		t.Fatalf("OSV was not selected for Go: %v", selectedIDs(selection))
	}
	if osv.Config.Database != ".agent-sdlc/osv-db" {
		t.Errorf("OSV database = %q, want osv_database_path", osv.Config.Database)
	}
	if len(osv.Languages) != 1 || osv.Languages[0] != LanguageGo {
		t.Errorf("OSV languages = %v, want [go]", osv.Languages)
	}
	audit, ok := byID[AdapterIDCargoAudit]
	if !ok {
		t.Fatalf("cargo-audit was not selected for Rust: %v", selectedIDs(selection))
	}
	if audit.Config.Database != ".agent-sdlc/advisory-db" {
		t.Errorf("cargo-audit database = %q", audit.Config.Database)
	}
	if audit.Selection != SelectionAutomatic || osv.Selection != SelectionAutomatic {
		t.Errorf("automatic selection mode missing: %+v %+v", osv.Selection, audit.Selection)
	}
	if len(selection.Overlaps) != 0 {
		t.Errorf("automatic selection produced overlaps: %+v", selection.Overlaps)
	}
}

func TestSelectionAutoFillsOnlyMissingLanguage(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:              boolPtr(true),
		Auto:                 true,
		AdvisoryDatabasePath: ".agent-sdlc/advisory-db",
		OSVDatabasePath:      ".agent-sdlc/osv-db",
		Adapters: []config.SecurityAdapterConfig{
			{ID: "cargo-audit", Category: config.SecurityCategoryDependencies, Command: []string{"cargo", "audit"}, Languages: []string{"rust"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	ids := selectedIDs(selection)
	if !containsID(ids, AdapterIDCargoAudit) || !containsID(ids, AdapterIDOSVScanner) {
		t.Fatalf("adapters = %v, want cargo-audit and OSV", ids)
	}
	for _, adapter := range selection.Adapters {
		if adapter.Config.ID == AdapterIDOSVScanner && (len(adapter.Languages) != 1 || adapter.Languages[0] != LanguageGo) {
			t.Fatalf("OSV should fill only Go: %+v", adapter)
		}
	}
}

func TestSelectionAutoDoesNotSelectOverlappingOSVForRust(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:              boolPtr(true),
		Auto:                 true,
		AdvisoryDatabasePath: ".agent-sdlc/advisory-db",
		OSVDatabasePath:      ".agent-sdlc/osv-db",
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	for _, adapter := range selection.Adapters {
		if adapter.Config.ID == AdapterIDOSVScanner {
			for _, language := range adapter.Languages {
				if language == LanguageRust {
					t.Fatalf("automatic OSV covered Rust, overlapping cargo-audit: %+v", adapter)
				}
			}
		}
	}
}

func TestSelectionExplicitOverlapIsVisible(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled: boolPtr(true),
		Adapters: []config.SecurityAdapterConfig{
			{ID: "semgrep", Category: config.SecurityCategorySAST, Command: []string{"semgrep"}, Languages: []string{"go"}},
			{ID: "private-sast", Category: config.SecurityCategorySAST, Protocol: config.SecurityProtocolExternal, Command: []string{"./private"}, Languages: []string{"go"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), false)
	found := false
	for _, overlap := range selection.Overlaps {
		if overlap.Category == CategorySAST && overlap.Language == LanguageGo && len(overlap.Adapters) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit overlap was not reported: %+v", selection.Overlaps)
	}
}

func TestSelectionUndeclaredExternalCoverageCannotProveLanguages(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled: boolPtr(true),
		Adapters: []config.SecurityAdapterConfig{
			{ID: "private-deps", Category: config.SecurityCategoryDependencies, Protocol: config.SecurityProtocolExternal, Command: []string{"./private"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), false)
	if len(selection.Adapters) != 1 {
		t.Fatalf("adapters = %+v", selection.Adapters)
	}
	adapter := selection.Adapters[0]
	if len(adapter.Languages) != 0 || adapter.RepositoryWide {
		t.Fatalf("undeclared external adapter claimed coverage: %+v", adapter)
	}
	exec := Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: "private-deps", Category: CategoryDependencies,
		Status: StatusCompleted, Coverage: CoverageComplete,
	}
	reasons := requiredCoverageReasons([]Execution{exec}, Policy{
		Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"},
	}, selection.Detection)
	found := false
	for _, reason := range reasons {
		if strings.Contains(reason, "undeclared") {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasons = %v, want an undeclared-coverage reason", reasons)
	}
}

func TestSelectionUndeclaredExternalCoverageBlocksAutoOverlap(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled: boolPtr(true),
		Auto:    true,
		Adapters: []config.SecurityAdapterConfig{
			{ID: "private-deps", Category: config.SecurityCategoryDependencies, Protocol: config.SecurityProtocolExternal, Command: []string{"./private"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), true)
	for _, adapter := range selection.Adapters {
		if adapter.Selection == SelectionAutomatic && adapter.Config.Category == config.SecurityCategoryDependencies {
			t.Fatalf("auto selection added %s over undeclared explicit coverage", adapter.Config.ID)
		}
	}
}

func TestSelectionDisabledCoverageIsAGap(t *testing.T) {
	cfg := config.SecurityConfig{
		Enabled:    boolPtr(true),
		Categories: map[string]*bool{"dependencies": boolPtr(false)},
		Adapters: []config.SecurityAdapterConfig{
			{ID: "cargo-audit", Category: config.SecurityCategoryDependencies, Command: []string{"cargo", "audit"}},
		},
	}
	selection := ResolveSelection(cfg, mixedDetection(t), false)
	evaluation := EvaluatePolicyWithDetection([]Execution{
		{
			ProtocolVersion: ProtocolVersion, AdapterID: "cargo-audit", Category: CategoryDependencies,
			Status: StatusSkipped, Coverage: CoverageUnknown, Disabled: true, Languages: []Language{LanguageRust},
		},
		{
			ProtocolVersion: ProtocolVersion, AdapterID: "gitleaks", Category: CategorySecrets,
			Status: StatusCompleted, Coverage: CoverageComplete, RepositoryWide: true,
		},
	}, Policy{Enabled: true, Mode: "report-only", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, selection.Detection)
	if evaluation.Verdict == VerdictPass {
		t.Fatalf("disabled dependency coverage produced PASS: %+v", evaluation.Reasons)
	}
	found := false
	for _, reason := range evaluation.Reasons {
		if strings.Contains(reason, "disabled coverage") {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasons = %v, want disabled coverage", evaluation.Reasons)
	}
}
