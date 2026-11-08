package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/security"
)

// detectRepositoryScoped is the existing content detection that
// `sop security inspect` uses. It is a package variable only so tests can count
// inspections; production always runs security.DetectRepositoryScoped.
var detectRepositoryScoped = security.DetectRepositoryScoped

// securityReadiness is the narrow, deterministic read-only probe the
// pre-execution seam consumes (SEC-LEFT-003). It reuses the same inspection
// `sop security inspect` uses: content detection, capability classification with
// advisory-database state, selection, and the required-readiness projection. It
// never builds an adapter, never runs a scanner, and adds no second lifecycle
// verdict.
//
// Enabled is false when the add-on is not active, so the caller is a strict
// no-op. Required reports whether the configuration declares an active required
// gate; only then may an unsatisfied prerequisite fail closed. Report-only or
// advisory configurations always report through diagnostics and never block a
// task.
type securityReadiness struct {
	Enabled  bool
	Required bool
	Ready    bool
	Reasons  []string
}

// runSecurityReadiness performs exactly one read-only readiness inspection at the
// pre-execution seam. It returns nil, and inspects nothing, when the add-on is
// disabled, so ordinary tasks are unchanged.
//
// The inspection runs once per call and is never cached: adapter executability,
// local rules and advisory-database freshness can change between tasks in the
// same process, so a retained result could hide a later required failure. The
// content detection is performed once and passed to classification, so a single
// invocation never repeats repository discovery.
func runSecurityReadiness(cfg config.Config, dir string, stdout io.Writer) *securityReadiness {
	if !cfg.SecurityGateActive() {
		return nil
	}
	result := securityReadiness{Enabled: true, Required: cfg.Security.RequiredGateActive()}
	detection, err := detectRepositoryScoped(dir, cfg.SecurityExcludes())
	if err != nil {
		result.Ready = !result.Required
		result.Reasons = []string{"security: repository detection failed: " + err.Error()}
		writeReadinessDiagnostics(stdout, result)
		return &result
	}
	selection := security.ResolveSelection(cfg.Security, detection, cfg.Security.Auto)
	selected := security.ClassifyCapabilitiesWithDatabases(detection, selection.EnabledConfigs(), cfg.Security.AdvisoryDatabasePath, cfg.Security.OSVDatabasePath)
	applySelectionState(selected, selection)
	result.Ready, result.Reasons = inspectRequiredReadiness(cfg, selection, selected)
	writeReadinessDiagnostics(stdout, result)
	return &result
}

// writeReadinessDiagnostics emits the readiness status through the existing
// precheck output. A ready required gate and a report-only advisory stay quiet on
// success; an unsatisfied required prerequisite is printed with actionable text.
func writeReadinessDiagnostics(stdout io.Writer, r securityReadiness) {
	if !r.Enabled {
		return
	}
	if r.Ready {
		return
	}
	if r.Required {
		fmt.Fprintln(stdout, "security readiness (required): not ready")
	} else {
		fmt.Fprintln(stdout, "security readiness (report-only): not ready")
	}
	for _, reason := range r.Reasons {
		fmt.Fprintf(stdout, "  - %s\n", reason)
	}
}

// readinessBlocks reports whether an unsatisfied readiness result must stop the
// task before IMPLEMENT. Only an active required gate fails closed; report-only
// and advisory configurations never block.
func readinessBlocks(r *securityReadiness) bool {
	return r != nil && r.Enabled && r.Required && !r.Ready
}

// readinessReason renders the actionable reason text for a blocking readiness
// result. It names the failing prerequisites so the diagnostic and the recorded
// classification reason match.
func readinessReason(r *securityReadiness) string {
	if r == nil {
		return "security: readiness was not evaluated"
	}
	reasons := nonBlankStrings(r.Reasons)
	if len(reasons) == 0 {
		return "security: required prerequisites are not satisfied"
	}
	return "security readiness (required): " + strings.Join(reasons, "; ")
}

// nonBlankStrings trims entries, drops empty ones, and preserves order.
func nonBlankStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
