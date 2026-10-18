package security

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestAggregateLanguageState(t *testing.T) {
	covered := LanguageCoverage{State: CoverageStateCovered}
	unconfigured := LanguageCoverage{State: CoverageStateUnconfigured}
	partial := LanguageCoverage{State: CoverageStatePartial}
	disabled := LanguageCoverage{State: CoverageStateDisabled}
	missing := LanguageCoverage{State: CoverageStateMissing}
	unsupported := LanguageCoverage{State: CoverageStateUnsupported}
	failed := LanguageCoverage{State: CoverageStateFailed}

	for _, tc := range []struct {
		name      string
		languages []LanguageCoverage
		want      CoverageState
	}{
		{name: "empty", want: CoverageStateUnconfigured},
		{name: "all unconfigured", languages: []LanguageCoverage{unconfigured, unconfigured}, want: CoverageStateUnconfigured},
		{name: "covered and unconfigured", languages: []LanguageCoverage{covered, unconfigured}, want: CoverageStatePartial},
		{name: "unconfigured and covered", languages: []LanguageCoverage{unconfigured, covered}, want: CoverageStatePartial},
		{name: "all covered", languages: []LanguageCoverage{covered, covered}, want: CoverageStateCovered},
		{name: "disabled and unconfigured", languages: []LanguageCoverage{disabled, unconfigured}, want: CoverageStateDisabled},
		{name: "partial and unconfigured", languages: []LanguageCoverage{partial, unconfigured}, want: CoverageStatePartial},
		{name: "missing and covered", languages: []LanguageCoverage{missing, covered}, want: CoverageStateMissing},
		{name: "failed and covered", languages: []LanguageCoverage{failed, covered}, want: CoverageStateFailed},
		{name: "unsupported and covered", languages: []LanguageCoverage{unsupported, covered}, want: CoverageStateUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := aggregateLanguageState(tc.languages); got != tc.want {
				t.Fatalf("aggregateLanguageState = %s, want %s", got, tc.want)
			}
			reversed := append([]LanguageCoverage(nil), tc.languages...)
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			if got := aggregateLanguageState(reversed); got != tc.want {
				t.Fatalf("reversed aggregateLanguageState = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestExpectedCoverageReportsUnconfiguredAndPartial(t *testing.T) {
	detection := mixedDetection(t)
	cfg := config.SecurityConfig{Enabled: boolPtr(true)}
	selection := ResolveSelection(cfg, detection, false)
	states := map[Category]CoverageState{}
	for _, category := range selection.Coverage {
		states[category.Category] = category.State
	}
	for _, category := range []Category{CategorySAST, CategoryDependencies} {
		if states[category] != CoverageStateUnconfigured {
			t.Errorf("%s state = %s, want unconfigured", category, states[category])
		}
	}

	cfg.Adapters = []config.SecurityAdapterConfig{{
		ID: "semgrep", Category: config.SecurityCategorySAST, Command: []string{"semgrep"}, Languages: []string{"go"},
	}}
	selection = ResolveSelection(cfg, detection, false)
	for _, category := range selection.Coverage {
		if category.Category != CategorySAST {
			continue
		}
		if category.State != CoverageStatePartial {
			t.Fatalf("mixed SAST coverage = %s, want partial (%+v)", category.State, category.Languages)
		}
		var goCovered, rustUnconfigured bool
		for _, language := range category.Languages {
			switch language.Language {
			case LanguageGo:
				goCovered = language.State == CoverageStateCovered
			case LanguageRust:
				rustUnconfigured = language.State == CoverageStateUnconfigured
			}
		}
		if !goCovered || !rustUnconfigured {
			t.Fatalf("SAST languages = %+v, want Go covered and Rust unconfigured", category.Languages)
		}
		return
	}
	t.Fatal("SAST coverage was not reported")
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

func TestAutomaticOSVSelectionRequiresVersionPin(t *testing.T) {
	detection := mixedDetection(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go"}, time.Now().UTC())
	cfg := config.SecurityConfig{
		Enabled: boolPtr(true), Auto: true, OSVDatabasePath: database,
	}
	selection := ResolveSelection(cfg, detection, true)
	var osv *SelectedAdapter
	for i := range selection.Adapters {
		if selection.Adapters[i].Config.ID == AdapterIDOSVScanner {
			osv = &selection.Adapters[i]
		}
	}
	if osv == nil {
		t.Fatalf("automatic OSV selection was not produced: %v", selectedIDs(selection))
	}
	if strings.TrimSpace(osv.Config.Version) != "" {
		t.Fatalf("automatic OSV selection invented a version pin: %q", osv.Config.Version)
	}
	candidates := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{osv.Config}, "", database)
	if len(candidates) != 1 || candidates[0].Status != CapabilityUnsupported {
		t.Fatalf("candidate = %+v, want unsupported without a version pin", candidates)
	}
	if !strings.Contains(candidates[0].Reason, "exact version pin") {
		t.Fatalf("reason = %q, want the version-pin prerequisite", candidates[0].Reason)
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
