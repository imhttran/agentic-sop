package security

import (
	"fmt"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
)

// SelectedAdapter is one adapter that will run, together with the selection
// metadata that must be recorded as evidence. Selection is configuration only;
// the runner never branches on an adapter ID.
type SelectedAdapter struct {
	Config         config.SecurityAdapterConfig
	Selection      SelectionMode
	Languages      []Language
	RepositoryWide bool
}

// DisabledAdapter is one adapter that configuration explicitly disabled. It is
// recorded as evidence and is never executed. A disabled adapter is not a failed
// or passed scan.
type DisabledAdapter struct {
	ID             string
	Category       Category
	Reason         string
	Languages      []Language
	RepositoryWide bool
	Selection      SelectionMode
}

// Overlap reports two or more selected adapters claiming the same capability and
// language. An overlap is allowed only when the operator selected both
// explicitly; it is always visible in evidence.
type Overlap struct {
	Category       Category `json:"category"`
	Language       Language `json:"language,omitempty"`
	RepositoryWide bool     `json:"repository_wide,omitempty"`
	Adapters       []string `json:"adapters"`
}

// CoverageState is the pre-scan or post-scan state of one coverage unit. The
// values distinguish a completed scan from a disabled, missing, or unsupported
// one so a successful adapter cannot hide a gap.
type CoverageState string

const (
	CoverageStateCovered      CoverageState = "covered"
	CoverageStatePartial      CoverageState = "partial"
	CoverageStateMissing      CoverageState = "missing"
	CoverageStateDisabled     CoverageState = "disabled"
	CoverageStateUnsupported  CoverageState = "unsupported"
	CoverageStateFailed       CoverageState = "failed"
	CoverageStateUnconfigured CoverageState = "unconfigured"
)

// LanguageCoverage is the coverage result for one language in one category.
type LanguageCoverage struct {
	Language Language      `json:"language"`
	State    CoverageState `json:"state"`
	Adapters []string      `json:"adapters,omitempty"`
	Reasons  []string      `json:"reasons,omitempty"`
}

// CategoryCoverage is the coverage result for one capability category.
type CategoryCoverage struct {
	Category       Category           `json:"category"`
	Required       bool               `json:"required,omitempty"`
	RepositoryWide bool               `json:"repository_wide,omitempty"`
	State          CoverageState      `json:"state"`
	Languages      []LanguageCoverage `json:"languages,omitempty"`
	Reasons        []string           `json:"reasons,omitempty"`
}

// Selection is the deterministic result of applying global, category, adapter,
// and automatic-selection controls to configuration and detected content.
type Selection struct {
	Adapters    []SelectedAdapter
	Disabled    []DisabledAdapter
	Overlaps    []Overlap
	Coverage    []CategoryCoverage
	Detection   *Detection
	Diagnostics []string
}

// EnabledConfigs returns the configured adapters that will run, in selection
// order. It is the only input to BuildAdapters.
func (s Selection) EnabledConfigs() []config.SecurityAdapterConfig {
	out := make([]config.SecurityAdapterConfig, 0, len(s.Adapters))
	for _, adapter := range s.Adapters {
		out = append(out, adapter.Config)
	}
	return out
}

// AdapterSelections returns the runtime selection metadata keyed by adapter ID.
func (s Selection) AdapterSelections() map[string]AdapterSelection {
	out := make(map[string]AdapterSelection, len(s.Adapters))
	for _, adapter := range s.Adapters {
		out[adapter.Config.ID] = AdapterSelection{
			Mode:           adapter.Selection,
			Languages:      append([]Language(nil), adapter.Languages...),
			RepositoryWide: adapter.RepositoryWide,
		}
	}
	return out
}

// AdapterSelection is the runtime metadata the runner records on an execution.
type AdapterSelection struct {
	Mode           SelectionMode
	Languages      []Language
	RepositoryWide bool
}

// ResolveSelection applies the documented precedence:
//
//  1. global disable prevents every scanner from running;
//  2. category disable prevents every adapter in that category;
//  3. adapter disable prevents that adapter;
//  4. explicit selection wins over automatic selection;
//  5. automatic selection fills only unconfigured, enabled coverage;
//  6. automatic selection cannot undo an explicit disable;
//  7. overlapping automatic coverage is not selected.
//
// It never executes a scanner and never reads a database.
func ResolveSelection(cfg config.SecurityConfig, detection Detection, auto bool) Selection {
	selection := Selection{Detection: &detection}
	enabled := cfg.Enabled != nil && *cfg.Enabled

	// claim records explicit configuration coverage, including disabled adapters.
	// An explicit disable therefore blocks automatic selection for that coverage.
	claimed := map[Category]map[Language]string{}
	claimedWhole := map[Category]bool{}
	claim := func(category Category, languages []Language, repositoryWide bool, id string) {
		if claimed[category] == nil {
			claimed[category] = map[Language]string{}
		}
		// A repository-wide adapter, or an adapter with undeclared language
		// coverage, may cover the whole category. Automatic selection must not add
		// an overlapping adapter on top of an explicit selection.
		if repositoryWide || len(languages) == 0 {
			claimedWhole[category] = true
		}
		for _, language := range languages {
			if _, ok := claimed[category][language]; !ok {
				claimed[category][language] = id
			}
		}
	}

	configuredByID := map[string]config.SecurityAdapterConfig{}
	for _, raw := range cfg.Adapters {
		id := normalizeAdapterID(raw.ID)
		if id == "" {
			continue
		}
		if _, ok := configuredByID[id]; !ok {
			configuredByID[id] = raw
		}
		category := adapterCategory(raw)
		languages, repositoryWide := adapterLanguageMetadata(raw, id, category)
		if !enabled {
			selection.Disabled = append(selection.Disabled, DisabledAdapter{
				ID: id, Category: category, Reason: "global security.enabled is false",
				Languages: languages, RepositoryWide: repositoryWide, Selection: SelectionExplicit,
			})
			continue
		}
		if category != "" && !cfg.CategoryEnabled(string(category)) {
			selection.Disabled = append(selection.Disabled, DisabledAdapter{
				ID: id, Category: category, Reason: fmt.Sprintf("category %s is disabled by configuration", category),
				Languages: languages, RepositoryWide: repositoryWide, Selection: SelectionExplicit,
			})
			claim(category, languages, repositoryWide, id)
			continue
		}
		if raw.Enabled != nil && !*raw.Enabled {
			selection.Disabled = append(selection.Disabled, DisabledAdapter{
				ID: id, Category: category, Reason: "adapter is disabled by configuration",
				Languages: languages, RepositoryWide: repositoryWide, Selection: SelectionExplicit,
			})
			claim(category, languages, repositoryWide, id)
			continue
		}
		selection.Adapters = append(selection.Adapters, SelectedAdapter{
			Config: raw, Selection: SelectionExplicit, Languages: languages, RepositoryWide: repositoryWide,
		})
		claim(category, languages, repositoryWide, id)
	}

	if enabled && auto {
		selection.Adapters = append(selection.Adapters, automaticAdapters(cfg, detection, claimed, claimedWhole, configuredByID, selection.Adapters)...)
		sort.SliceStable(selection.Adapters, func(i, j int) bool {
			return normalizeAdapterID(selection.Adapters[i].Config.ID) < normalizeAdapterID(selection.Adapters[j].Config.ID)
		})
	}

	selection.Overlaps = selectedOverlaps(selection.Adapters, detection)
	for _, overlap := range selection.Overlaps {
		selection.Diagnostics = append(selection.Diagnostics, overlapDiagnostic(overlap))
	}
	for _, disabled := range selection.Disabled {
		selection.Diagnostics = append(selection.Diagnostics, fmt.Sprintf("%s: %s", disabled.ID, disabled.Reason))
	}
	selection.Coverage = expectedCoverage(selection.Adapters, selection.Disabled, detection, cfg)
	selection.Coverage = refineDependencyCoverage(selection.Coverage, selection.Adapters, detection, cfg)
	return selection
}

// refineDependencyCoverage keeps the read-only readiness projection honest: a
// selected dependency adapter that cannot account for every in-scope obligation
// (or lacks a pin/database/executable) must not project language coverage as
// covered. The scan itself still runs and fails closed.
func refineDependencyCoverage(coverage []CategoryCoverage, adapters []SelectedAdapter, detection Detection, cfg config.SecurityConfig) []CategoryCoverage {
	for i := range coverage {
		if coverage[i].Category != CategoryDependencies {
			continue
		}
		for j := range coverage[i].Languages {
			language := &coverage[i].Languages[j]
			if language.State != CoverageStateCovered {
				continue
			}
			covered := false
			reason := ""
			for _, adapter := range adapters {
				if adapterCategory(adapter.Config) != CategoryDependencies {
					continue
				}
				if !adapter.RepositoryWide && !containsLanguage(adapter.Languages, language.Language) {
					continue
				}
				id := normalizeAdapterID(adapter.Config.ID)
				status := CapabilityAvailable
				switch id {
				case AdapterIDCargoAudit:
					status = capabilityForCargoAudit(detection, adapter.Config, cfg.AdvisoryDatabasePath)
				case AdapterIDOSVScanner:
					status = capabilityForOSV(detection, adapter.Config, cfg.OSVDatabasePath)
				}
				if status == CapabilityAvailable || status == CapabilityProposed {
					covered = true
					break
				}
				if reason == "" {
					reason = capabilityReason(id, adapter.Config, detection, cfg.AdvisoryDatabasePath, cfg.OSVDatabasePath)
				}
			}
			if covered {
				continue
			}
			language.State = CoverageStateUnsupported
			if reason == "" {
				reason = "selected dependency adapter cannot prove this language's coverage"
			}
			language.Reasons = append(language.Reasons, reason)
		}
		coverage[i].State = aggregateLanguageState(coverage[i].Languages)
	}
	return coverage
}

// automaticAdapters fills only coverage that no explicit adapter, enabled or
// disabled, has claimed. It never re-adds an explicitly disabled ID, never adds a
// second adapter for a claimed language, and never selects an overlapping
// automatic candidate.
func automaticAdapters(cfg config.SecurityConfig, detection Detection, claimed map[Category]map[Language]string, claimedWhole map[Category]bool, configuredByID map[string]config.SecurityAdapterConfig, selected []SelectedAdapter) []SelectedAdapter {
	selectedIDs := map[string]bool{}
	for _, adapter := range selected {
		selectedIDs[normalizeAdapterID(adapter.Config.ID)] = true
	}
	detected := map[Language]bool{}
	for _, language := range detection.Languages {
		detected[language] = true
	}
	if len(detected) == 0 {
		return nil
	}
	var out []SelectedAdapter
	add := func(raw config.SecurityAdapterConfig, languages []Language, repositoryWide bool) {
		id := normalizeAdapterID(raw.ID)
		if id == "" || selectedIDs[id] {
			return
		}
		// A configured ID is authoritative even when its declared language scope
		// does not cover this gap. Re-adding it would create a duplicate ID and
		// silently widen an explicit selection.
		if _, configured := configuredByID[id]; configured {
			return
		}
		selectedIDs[id] = true
		out = append(out, SelectedAdapter{
			Config: raw, Selection: SelectionAutomatic, Languages: languages, RepositoryWide: repositoryWide,
		})
	}

	if cfg.CategoryEnabled(config.SecurityCategorySecrets) {
		if !claimedWhole[CategorySecrets] && len(claimed[CategorySecrets]) == 0 {
			raw := config.SecurityAdapterConfig{
				ID: AdapterIDGitleaks, Category: config.SecurityCategorySecrets,
				Protocol: config.SecurityProtocolBuiltin, Command: []string{"gitleaks"},
			}
			if template, ok := configuredByID[AdapterIDGitleaks]; ok {
				raw.Version = template.Version
			}
			add(raw, nil, true)
		}
	}

	if cfg.CategoryEnabled(config.SecurityCategorySAST) && !claimedWhole[CategorySAST] {
		var missing []Language
		for _, language := range detection.Languages {
			if _, ok := claimed[CategorySAST][language]; !ok {
				missing = append(missing, language)
			}
		}
		if len(missing) > 0 {
			raw := config.SecurityAdapterConfig{
				ID: AdapterIDSemgrep, Category: config.SecurityCategorySAST,
				Protocol: config.SecurityProtocolBuiltin, Command: []string{"semgrep"},
			}
			if template, ok := configuredByID[AdapterIDSemgrep]; ok {
				raw.Rules = template.Rules
				raw.Version = template.Version
			}
			add(raw, missing, false)
		}
	}

	if cfg.CategoryEnabled(config.SecurityCategoryDependencies) && !claimedWhole[CategoryDependencies] {
		if detected[LanguageGo] {
			if _, ok := claimed[CategoryDependencies][LanguageGo]; !ok {
				if !selectedIDs[AdapterIDOSVScanner] {
					raw := config.SecurityAdapterConfig{
						ID: AdapterIDOSVScanner, Category: config.SecurityCategoryDependencies,
						Protocol: config.SecurityProtocolBuiltin, Command: []string{"osv-scanner"},
						Database: strings.TrimSpace(cfg.OSVDatabasePath), Languages: []string{config.SecurityLanguageGo},
					}
					if template, ok := configuredByID[AdapterIDOSVScanner]; ok {
						raw.Version = template.Version
						if strings.TrimSpace(raw.Database) == "" {
							raw.Database = template.Database
						}
					}
					add(raw, []Language{LanguageGo}, false)
				}
			}
		}
		if detected[LanguageRust] {
			if _, ok := claimed[CategoryDependencies][LanguageRust]; !ok {
				// cargo-audit remains the deterministic legacy automatic choice for
				// Rust. OSV for Rust requires an explicit selection so overlapping
				// automatic coverage is never run twice.
				raw := config.SecurityAdapterConfig{
					ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
					Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
					Database: strings.TrimSpace(cfg.AdvisoryDatabasePath), Languages: []string{config.SecurityLanguageRust},
				}
				if template, ok := configuredByID[AdapterIDCargoAudit]; ok {
					raw.Version = template.Version
					if strings.TrimSpace(raw.Database) == "" {
						raw.Database = template.Database
					}
				}
				add(raw, []Language{LanguageRust}, false)
			}
		}
	}
	return out
}

// selectedOverlaps reports every capability+language claimed by more than one
// selected adapter. Explicit overlaps are allowed and remain visible; automatic
// selection is designed not to create them.
func selectedOverlaps(adapters []SelectedAdapter, detection Detection) []Overlap {
	type key struct {
		category       Category
		language       Language
		repositoryWide bool
	}
	groups := map[key][]string{}
	for _, adapter := range adapters {
		category := adapterCategory(adapter.Config)
		if adapter.RepositoryWide {
			groups[key{category: category, repositoryWide: true}] = append(groups[key{category: category, repositoryWide: true}], adapter.Config.ID)
			continue
		}
		for _, language := range adapter.Languages {
			k := key{category: category, language: language}
			groups[k] = append(groups[k], adapter.Config.ID)
		}
	}
	var out []Overlap
	for k, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		out = append(out, Overlap{Category: k.category, Language: k.language, RepositoryWide: k.repositoryWide, Adapters: ids})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return !out[i].RepositoryWide
	})
	return out
}

func overlapDiagnostic(overlap Overlap) string {
	scope := string(overlap.Language)
	if overlap.RepositoryWide {
		scope = "repository-wide"
	}
	return fmt.Sprintf("overlapping coverage in category %s (%s): %s; both are explicitly selected and will run", overlap.Category, scope, strings.Join(overlap.Adapters, ", "))
}

// expectedCoverage reports the pre-scan coverage implied by selection. It is used
// by `security inspect`; the post-scan report is derived from actual executions.
func expectedCoverage(adapters []SelectedAdapter, disabled []DisabledAdapter, detection Detection, cfg config.SecurityConfig) []CategoryCoverage {
	required := map[string]bool{}
	for _, raw := range cfg.RequiredCategories {
		required[strings.ToLower(strings.TrimSpace(raw))] = true
	}
	categories := []Category{CategorySAST, CategorySecrets, CategoryDependencies}
	var out []CategoryCoverage
	for _, category := range categories {
		coverage := CategoryCoverage{Category: category, Required: required[string(category)]}
		if category == CategorySecrets {
			coverage.RepositoryWide = true
			ids := adapterIDsFor(adapters, category, "", true)
			if len(ids) > 0 {
				coverage.State = CoverageStateCovered
				coverage.Reasons = append(coverage.Reasons, "repository-wide scanner selected")
			} else if ids := disabledIDsFor(disabled, category, "", true); len(ids) > 0 {
				coverage.State = CoverageStateDisabled
				coverage.Reasons = append(coverage.Reasons, "repository-wide scanner is disabled: "+strings.Join(ids, ", "))
			} else if coverage.Required {
				coverage.State = CoverageStateMissing
				coverage.Reasons = append(coverage.Reasons, "required category has no enabled adapter")
			} else {
				continue
			}
			out = append(out, coverage)
			continue
		}
		// SAST and dependencies are language-scoped. Every detected language is
		// reported separately so one language cannot hide another.
		languages := detection.Languages
		if len(languages) == 0 {
			continue
		}
		for _, language := range languages {
			lc := LanguageCoverage{Language: language}
			ids := adapterIDsFor(adapters, category, language, false)
			if len(ids) > 0 {
				lc.State = CoverageStateCovered
				lc.Adapters = ids
			} else if ids := disabledIDsFor(disabled, category, language, false); len(ids) > 0 {
				lc.State = CoverageStateDisabled
				lc.Adapters = ids
				lc.Reasons = append(lc.Reasons, "explicitly disabled coverage")
			} else if coverage.Required {
				lc.State = CoverageStateMissing
				lc.Reasons = append(lc.Reasons, "required language coverage has no enabled adapter")
			} else {
				lc.State = CoverageStateUnconfigured
			}
			coverage.Languages = append(coverage.Languages, lc)
		}
		coverage.State = aggregateLanguageState(coverage.Languages)
		out = append(out, coverage)
	}
	return out
}

func aggregateLanguageState(languages []LanguageCoverage) CoverageState {
	if len(languages) == 0 {
		return CoverageStateUnconfigured
	}
	present := map[CoverageState]bool{}
	for _, language := range languages {
		present[language.State] = true
	}
	// Deterministic precedence, independent of language order. A category is
	// covered only when every applicable unit is covered; a mix of covered and
	// unconfigured units is partial, not covered.
	switch {
	case present[CoverageStateFailed]:
		return CoverageStateFailed
	case present[CoverageStateUnsupported]:
		return CoverageStateUnsupported
	case present[CoverageStateMissing]:
		return CoverageStateMissing
	case present[CoverageStateDisabled]:
		return CoverageStateDisabled
	case present[CoverageStatePartial]:
		return CoverageStatePartial
	case present[CoverageStateUnconfigured] && present[CoverageStateCovered]:
		return CoverageStatePartial
	case present[CoverageStateUnconfigured]:
		return CoverageStateUnconfigured
	case present[CoverageStateCovered]:
		return CoverageStateCovered
	default:
		// Unknown internal state is fail-closed rather than treated as covered.
		return CoverageStateUnconfigured
	}
}

func adapterIDsFor(adapters []SelectedAdapter, category Category, language Language, repositoryWide bool) []string {
	var ids []string
	for _, adapter := range adapters {
		if adapterCategory(adapter.Config) != category {
			continue
		}
		if repositoryWide {
			if adapter.RepositoryWide {
				ids = append(ids, adapter.Config.ID)
			}
			continue
		}
		if adapter.RepositoryWide || containsLanguage(adapter.Languages, language) {
			ids = append(ids, adapter.Config.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func disabledIDsFor(disabled []DisabledAdapter, category Category, language Language, repositoryWide bool) []string {
	var ids []string
	for _, adapter := range disabled {
		if adapter.Category != category {
			continue
		}
		if repositoryWide {
			if adapter.RepositoryWide {
				ids = append(ids, adapter.ID)
			}
			continue
		}
		if adapter.RepositoryWide || containsLanguage(adapter.Languages, language) {
			ids = append(ids, adapter.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// adapterCategory resolves the category of a configured adapter, including the
// built-in default for an omitted category.
func adapterCategory(raw config.SecurityAdapterConfig) Category {
	category := strings.ToLower(strings.TrimSpace(raw.Category))
	if category == "" {
		category = builtinCategory(normalizeAdapterID(raw.ID))
	}
	return Category(category)
}

func builtinCategory(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case AdapterIDSemgrep:
		return config.SecurityCategorySAST
	case AdapterIDGitleaks:
		return config.SecurityCategorySecrets
	case AdapterIDCargoAudit, AdapterIDOSVScanner:
		return config.SecurityCategoryDependencies
	default:
		return ""
	}
}

// adapterLanguageMetadata returns the language coverage an adapter declares.
// Built-ins have stable defaults; an external adapter without metadata is
// deliberately left undeclared so it cannot prove language-specific required
// coverage.
func adapterLanguageMetadata(raw config.SecurityAdapterConfig, id string, category Category) ([]Language, bool) {
	// Secrets scanners are language-independent capability providers. They cover
	// the repository as a whole, including legacy external secrets adapters.
	if id == AdapterIDGitleaks || category == CategorySecrets {
		return nil, true
	}
	if raw.RepositoryWide != nil && *raw.RepositoryWide {
		return nil, true
	}
	if len(raw.Languages) > 0 {
		return configuredLanguages(raw.Languages), false
	}
	switch id {
	case AdapterIDSemgrep, AdapterIDOSVScanner:
		return []Language{LanguageGo, LanguageRust}, false
	case AdapterIDCargoAudit:
		return []Language{LanguageRust}, false
	default:
		// External adapter with no language metadata: unknown coverage.
		return nil, false
	}
}

func configuredLanguages(values []string) []Language {
	out := make([]Language, 0, len(values))
	for _, value := range values {
		language := Language(strings.ToLower(strings.TrimSpace(value)))
		if isKnownLanguage(language) {
			out = append(out, language)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsLanguage(values []Language, language Language) bool {
	for _, value := range values {
		if value == language {
			return true
		}
	}
	return false
}

func normalizeAdapterID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
