package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/security"
)

// requiredConfig builds a config with the add-on enabled and the given required
// categories, so the readiness projection sees the same inputs the CLI produces.
func requiredConfig(categories ...string) config.Config {
	enabled := true
	return config.Config{Security: config.SecurityConfig{
		Enabled:            &enabled,
		Mode:               config.SecurityModeRequired,
		RequiredCategories: categories,
	}}
}

// coveredSelection is the deterministic selection shape for a repository whose
// required categories are fully covered.
func coveredSelection(covered ...string) security.Selection {
	var coverage []security.CategoryCoverage
	for _, category := range covered {
		coverage = append(coverage, security.CategoryCoverage{
			Category:  security.Category(category),
			Required:  true,
			State:     security.CoverageStateCovered,
			Languages: []security.LanguageCoverage{{Language: security.LanguageGo, State: security.CoverageStateCovered}},
		})
	}
	return security.Selection{
		Coverage:  coverage,
		Detection: &security.Detection{Languages: []security.Language{security.LanguageGo}},
	}
}

// TestInspectRequiredReadinessScopesToRequiredCategories pins the projection the
// operator reads from `sop security inspect`. An adapter outside a required
// category must never make the required gate look unsatisfiable, and every
// required-category gap must still fail closed.
func TestInspectRequiredReadinessScopesToRequiredCategories(t *testing.T) {
	cargoUnsupported := security.AdapterCandidate{
		ID:       security.AdapterIDCargoAudit,
		Category: security.CategoryDependencies,
		Status:   security.CapabilityUnsupported,
		Reason:   "no Rust content with a Cargo.lock",
	}
	cargoUnavailable := security.AdapterCandidate{
		ID:       security.AdapterIDCargoAudit,
		Category: security.CategoryDependencies,
		Status:   security.CapabilityUnavailable,
		Reason:   "cargo-audit not found",
	}
	semgrepAvailable := security.AdapterCandidate{
		ID:       security.AdapterIDSemgrep,
		Category: security.CategorySAST,
		Status:   security.CapabilityAvailable,
	}
	semgrepUnavailable := security.AdapterCandidate{
		ID:       security.AdapterIDSemgrep,
		Category: security.CategorySAST,
		Status:   security.CapabilityUnavailable,
		Reason:   "semgrep not found",
	}

	cases := []struct {
		name      string
		cfg       config.Config
		selection security.Selection
		selected  []security.AdapterCandidate
		wantReady bool
		wantWhy   string
	}{
		{
			name:      "optional unsupported adapter does not block",
			cfg:       requiredConfig("sast"),
			selection: coveredSelection("sast"),
			selected:  []security.AdapterCandidate{semgrepAvailable, cargoUnsupported},
			wantReady: true,
		},
		{
			name:      "optional unavailable adapter does not block",
			cfg:       requiredConfig("sast", "secrets"),
			selection: coveredSelection("sast", "secrets"),
			selected:  []security.AdapterCandidate{semgrepAvailable, cargoUnavailable},
			wantReady: true,
		},
		{
			name:      "required unavailable adapter fails closed",
			cfg:       requiredConfig("dependencies"),
			selection: coveredSelection("dependencies"),
			selected:  []security.AdapterCandidate{cargoUnavailable},
			wantReady: false,
			wantWhy:   "adapter cargo-audit prerequisite is unavailable: cargo-audit not found",
		},
		{
			name:      "required unsupported adapter fails closed",
			cfg:       requiredConfig("dependencies"),
			selection: coveredSelection("dependencies"),
			selected:  []security.AdapterCandidate{cargoUnsupported},
			wantReady: false,
			wantWhy:   "no Rust content with a Cargo.lock",
		},
		{
			name:      "required unavailable adapter fails closed even beside a covered one",
			cfg:       requiredConfig("sast", "dependencies"),
			selection: coveredSelection("sast", "dependencies"),
			selected:  []security.AdapterCandidate{semgrepAvailable, cargoUnavailable},
			wantReady: false,
			wantWhy:   "adapter cargo-audit prerequisite is unavailable",
		},
		{
			name: "required category with partial coverage fails closed",
			cfg:  requiredConfig("sast"),
			selection: security.Selection{
				Coverage: []security.CategoryCoverage{{
					Category:  security.CategorySAST,
					Required:  true,
					State:     security.CoverageStatePartial,
					Languages: []security.LanguageCoverage{{Language: security.LanguageGo, State: security.CoverageStatePartial}},
				}},
				Detection: &security.Detection{Languages: []security.Language{security.LanguageGo}},
			},
			selected:  []security.AdapterCandidate{semgrepUnavailable},
			wantReady: false,
		},
		{
			name:      "required category with no coverage evidence fails closed",
			cfg:       requiredConfig("secrets"),
			selection: coveredSelection("sast"),
			selected:  []security.AdapterCandidate{semgrepAvailable},
			wantReady: false,
			wantWhy:   "required category secrets has no coverage evidence",
		},
		{
			name:      "no required categories fails closed",
			cfg:       requiredConfig(),
			selection: coveredSelection("sast"),
			wantReady: false,
			wantWhy:   "required mode needs at least one required category",
		},
		{
			name: "disabled add-on fails closed",
			cfg: func() config.Config {
				c := requiredConfig("sast")
				disabled := false
				c.Security.Enabled = &disabled
				return c
			}(),
			selection: coveredSelection("sast"),
			selected:  []security.AdapterCandidate{semgrepAvailable},
			wantReady: false,
			wantWhy:   "global security.enabled is false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ready, reasons := inspectRequiredReadiness(tc.cfg, tc.selection, tc.selected)
			if ready != tc.wantReady {
				t.Fatalf("ready = %t, want %t (reasons: %v)", ready, tc.wantReady, reasons)
			}
			if tc.wantWhy != "" && !strings.Contains(strings.Join(reasons, "; "), tc.wantWhy) {
				t.Fatalf("reasons = %v, want one containing %q", reasons, tc.wantWhy)
			}
		})
	}
}

// TestInspectReadinessReportsOptionalAdaptersWithoutBlocking keeps the operator-visible
// message honest: an optional adapter that cannot run must not silently disappear
// from the reasons when it is the only thing that differs from a clean projection.
// It is reported by the capability list, not by readiness.
func TestInspectReadinessIgnoresOptionalAdapterReasons(t *testing.T) {
	ready, reasons := inspectRequiredReadiness(
		requiredConfig("sast"),
		coveredSelection("sast"),
		[]security.AdapterCandidate{
			{ID: security.AdapterIDSemgrep, Category: security.CategorySAST, Status: security.CapabilityAvailable},
			{ID: security.AdapterIDCargoAudit, Category: security.CategoryDependencies, Status: security.CapabilityUnsupported, Reason: "no Rust content"},
		},
	)
	if !ready {
		t.Fatalf("ready = false, want true: %v", reasons)
	}
	if strings.Contains(strings.Join(reasons, "; "), "cargo-audit") {
		t.Fatalf("an optional adapter leaked into the readiness reasons: %v", reasons)
	}
}
