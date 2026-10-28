package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/security"
)

// runSecurity dispatches the security add-on commands. The add-on is opt-in:
// without an enabled configuration or SOP_SECURITY_ENABLED, `scan` reports
// NOT_APPLICABLE and executes no scanner. `inspect` is read-only and works even
// while the add-on is disabled, so an operator can see detected content and
// proposed capabilities before enabling a scan.
func runSecurity(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, securityUsage)
		return exitUsage
	}
	switch args[0] {
	case "scan":
		return runSecurityScan(args[1:], stdout, stderr, d)
	case "inspect":
		return runSecurityInspect(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintln(stderr, securityUsage)
		return exitUsage
	}
}

const securityUsage = "usage: sop security scan [--json] [--report-only|--required] [--scope workspace|changed-files] [--baseline PATH] [--resume RUN_ID] [--run-id ID] [--config PATH] [--auto]\n       sop security inspect [--json] [--config PATH]"

func runSecurityScan(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseSecurityScanArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	if opts.config != "" {
		configPath := opts.config
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(dir, configPath)
		}
		loaded, lerr := config.Load(configPath)
		if lerr != nil {
			fmt.Fprintf(stderr, "security: config %s: %v\n", configPath, lerr)
			return exitError
		}
		cfg = *loaded
	}
	if !cfg.SecurityEnabled() && !cfg.Security.RequiredGateActive() {
		if opts.json {
			_ = json.NewEncoder(stdout).Encode(map[string]any{
				"verdict":  security.VerdictNotApplicable,
				"coverage": security.CoverageUnknown,
				"reason":   "security add-on is disabled",
			})
		} else {
			fmt.Fprintln(stdout, "security: NOT_APPLICABLE (add-on disabled)")
		}
		return exitOK
	}

	mode := cfg.Security.Mode
	if opts.mode != "" {
		mode = opts.mode
	}
	scope := cfg.Security.Scope
	if opts.scope != "" {
		scope = opts.scope
	}
	baselinePath := cfg.Security.Baseline
	if opts.baseline != "" {
		baselinePath = opts.baseline
	}
	baseline, baselineDiagnostics, err := security.LoadBaseline(dir, baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}

	auto := opts.auto || cfg.Security.Auto
	needDetection := auto || mode == config.SecurityModeRequired || len(cfg.Security.RequiredCategories) > 0
	detection := security.Detection{}
	if needDetection {
		detected, derr := security.DetectRepositoryScoped(dir, cfg.SecurityExcludes())
		if derr != nil {
			fmt.Fprintf(stderr, "security: detect repository: %v\n", derr)
			return exitError
		}
		detection = detected
	}
	selection := security.ResolveSelection(cfg.Security, detection, auto)
	adapterConfigs := selection.EnabledConfigs()

	adapters, err := security.BuildAdapters(adapterConfigs, security.BuildOptions{
		Timeout:         time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes:  cfg.Security.MaxOutputBytes,
		DatabasePath:    cfg.Security.AdvisoryDatabasePath,
		OSVDatabasePath: cfg.Security.OSVDatabasePath,
		StalePolicy:     cfg.Security.StaleAdvisoryDatabase,
		WorkspaceRoot:   dir,
		Evaluation:      &cfg.Security.Evaluation,
	})
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	policy := security.Policy{
		Enabled:               cfg.SecurityEnabled(),
		Mode:                  mode,
		Scope:                 scope,
		RequiredCategories:    cfg.Security.RequiredCategories,
		FailOn:                cfg.Security.FailOn,
		UnknownSeverity:       cfg.Security.UnknownSeverity,
		TimeoutSeconds:        cfg.Security.TimeoutSeconds,
		MaxOutputBytes:        cfg.Security.MaxOutputBytes,
		Exclude:               cfg.SecurityExcludes(),
		AdvisoryDatabasePath:  cfg.Security.AdvisoryDatabasePath,
		OSVDatabasePath:       cfg.Security.OSVDatabasePath,
		StaleAdvisoryDatabase: cfg.Security.StaleAdvisoryDatabase,
		Auto:                  auto,
		RequiredGateActive:    cfg.Security.RequiredGateActive(),
	}

	runID := opts.runID
	if runID == "" {
		runID = "security-" + time.Now().UTC().Format("20060102T150405Z")
	}
	rn, err := runpkg.Open(dir, runID)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	prior, priorNative, err := loadPriorSecurityScan(dir, opts.resume)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	runner, err := security.NewRunner(security.RunnerConfig{
		WorkspaceRoot:      dir,
		RunDir:             rn.Dir(),
		RunID:              runID,
		Adapters:           adapters,
		Policy:             policy,
		Timeout:            time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes:     cfg.Security.MaxOutputBytes,
		Baseline:           baseline,
		Selection:          selection.AdapterSelections(),
		Disabled:           selection.Disabled,
		Overlaps:           selection.Overlaps,
		Detection:          selection.Detection,
		Prior:              prior,
		PriorNativeReports: priorNative,
	})
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	scan.Detection = selection.Detection
	if !needDetection {
		scan.Detection = nil
	}
	scan.Diagnostics = append(append([]string(nil), baselineDiagnostics...), scan.Diagnostics...)
	scan.Diagnostics = append(selection.Diagnostics, scan.Diagnostics...)

	refs, err := security.WriteArtifacts(rn.Dir(), scan)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}

	if opts.json {
		out := struct {
			Scan      security.Scan         `json:"scan"`
			Artifacts security.ArtifactRefs `json:"artifacts"`
		}{Scan: scan, Artifacts: refs}
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			fmt.Fprintf(stderr, "security: %v\n", err)
			return exitError
		}
	} else {
		printSecurityScan(stdout, scan, refs)
	}

	// Report-only records the verdict without blocking delivery: a PASS, a PASS
	// with recorded findings, or a REPORT_ONLY verdict all exit zero. Required mode
	// exits non-zero for anything but a clean PASS.
	if policy.Mode == config.SecurityModeRequired && scan.Verdict != security.VerdictPass && scan.Verdict != security.VerdictReportOnly {
		return exitError
	}
	return exitOK
}

type securityInspectOptions struct {
	json   bool
	config string
}

// runSecurityInspect reports detected repository content and the capability
// status of configured and proposed adapters. It never executes a scanner, so it
// is safe while the add-on is disabled and report-only.
func runSecurityInspect(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseSecurityInspectArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	if opts.config != "" {
		configPath := opts.config
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(dir, configPath)
		}
		loaded, lerr := config.Load(configPath)
		if lerr != nil {
			fmt.Fprintf(stderr, "security: config %s: %v\n", configPath, lerr)
			return exitError
		}
		cfg = *loaded
	}
	detection, err := security.DetectRepositoryScoped(dir, cfg.SecurityExcludes())
	if err != nil {
		fmt.Fprintf(stderr, "security: detect repository: %v\n", err)
		return exitError
	}
	configured := security.ClassifyCapabilitiesWithDatabases(detection, cfg.Security.Adapters, cfg.Security.AdvisoryDatabasePath, cfg.Security.OSVDatabasePath)
	proposed := security.SelectCandidatesWithDatabases(detection, cfg.Security.Adapters, cfg.Security.AdvisoryDatabasePath, cfg.Security.OSVDatabasePath)
	selection := security.ResolveSelection(cfg.Security, detection, cfg.Security.Auto)
	selected := security.ClassifyCapabilitiesWithDatabases(detection, selection.EnabledConfigs(), cfg.Security.AdvisoryDatabasePath, cfg.Security.OSVDatabasePath)
	applySelectionState(configured, selection)
	applySelectionState(selected, selection)
	requiredReady, requiredReasons := inspectRequiredReadiness(cfg, selection, selected)
	result := struct {
		Version              int                         `json:"version"`
		Enabled              bool                        `json:"enabled"`
		Mode                 string                      `json:"mode,omitempty"`
		Scope                string                      `json:"scope,omitempty"`
		Auto                 bool                        `json:"auto"`
		Detection            security.Detection          `json:"detection"`
		Configured           []security.AdapterCandidate `json:"configured"`
		Selected             []security.AdapterCandidate `json:"selected"`
		Proposed             []security.AdapterCandidate `json:"proposed"`
		Disabled             []security.DisabledAdapter  `json:"disabled,omitempty"`
		Overlaps             []security.Overlap          `json:"overlaps,omitempty"`
		Coverage             []security.CategoryCoverage `json:"coverage,omitempty"`
		RequiredReady        bool                        `json:"required_ready"`
		RequiredReasons      []string                    `json:"required_reasons,omitempty"`
		SelectionDiagnostics []string                    `json:"selection_diagnostics,omitempty"`
	}{
		Version: 1, Enabled: cfg.SecurityEnabled(), Mode: cfg.Security.Mode, Scope: cfg.Security.Scope,
		Auto: cfg.Security.Auto, Detection: detection, Configured: configured, Selected: selected, Proposed: proposed,
		Disabled: selection.Disabled, Overlaps: selection.Overlaps, Coverage: selection.Coverage,
		RequiredReady: requiredReady, RequiredReasons: requiredReasons, SelectionDiagnostics: selection.Diagnostics,
	}
	if opts.json {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "security: %v\n", err)
			return exitError
		}
		return exitOK
	}
	fmt.Fprintf(stdout, "security inspect: enabled=%t mode=%s scope=%s auto=%t\n", result.Enabled, result.Mode, result.Scope, result.Auto)
	fmt.Fprintf(stdout, "  languages: %s\n", detectionLanguageText(detection.Languages))
	for _, manifest := range detection.Manifests {
		fmt.Fprintf(stdout, "  manifest: %s (%s)\n", manifest.Path, manifest.Language)
	}
	for _, lockfile := range detection.Lockfiles {
		state := "present"
		if !lockfile.Present {
			state = "MISSING"
		}
		fmt.Fprintf(stdout, "  lockfile: %s (%s, %s)\n", lockfile.Path, lockfile.Language, state)
	}
	fmt.Fprintln(stdout, "  configured adapters:")
	printCapabilities(stdout, configured)
	fmt.Fprintln(stdout, "  selected adapters:")
	printCapabilities(stdout, selected)
	fmt.Fprintln(stdout, "  proposed adapters:")
	printCapabilities(stdout, proposed)
	if len(selection.Diagnostics) > 0 {
		fmt.Fprintln(stdout, "  selection diagnostics:")
		for _, diagnostic := range selection.Diagnostics {
			fmt.Fprintf(stdout, "    %s\n", diagnostic)
		}
	}
	if len(selection.Overlaps) > 0 {
		fmt.Fprintln(stdout, "  overlapping coverage:")
		for _, overlap := range selection.Overlaps {
			scope := string(overlap.Language)
			if overlap.RepositoryWide {
				scope = "repository-wide"
			}
			fmt.Fprintf(stdout, "    %s (%s): %s\n", overlap.Category, scope, strings.Join(overlap.Adapters, ", "))
		}
	}
	if len(selection.Coverage) > 0 {
		fmt.Fprintln(stdout, "  coverage:")
		for _, coverage := range selection.Coverage {
			required := ""
			if coverage.Required {
				required = " (required)"
			}
			fmt.Fprintf(stdout, "    %s%s: %s\n", coverage.Category, required, coverage.State)
			for _, language := range coverage.Languages {
				fmt.Fprintf(stdout, "      %s: %s", language.Language, language.State)
				if len(language.Adapters) > 0 {
					fmt.Fprintf(stdout, " (%s)", strings.Join(language.Adapters, ", "))
				}
				fmt.Fprintln(stdout)
			}
		}
	}
	fmt.Fprintf(stdout, "  required-mode readiness: %t\n", requiredReady)
	for _, reason := range requiredReasons {
		fmt.Fprintf(stdout, "    %s\n", reason)
	}
	return exitOK
}

// applySelectionState overlays the effective enable/disable and selection state
// onto capability candidates. Classification itself stays a pure content probe.
func applySelectionState(candidates []security.AdapterCandidate, selection security.Selection) {
	disabled := map[string]security.DisabledAdapter{}
	for _, adapter := range selection.Disabled {
		disabled[strings.ToLower(adapter.ID)] = adapter
	}
	explicit := map[string]string{}
	for _, adapter := range selection.Adapters {
		explicit[strings.ToLower(adapter.Config.ID)] = string(adapter.Selection)
	}
	for i := range candidates {
		key := strings.ToLower(candidates[i].ID)
		if adapter, ok := disabled[key]; ok {
			candidates[i].Disabled = true
			candidates[i].DisabledReason = adapter.Reason
		}
		if mode, ok := explicit[key]; ok {
			candidates[i].Selection = mode
		}
	}
}

// inspectRequiredReadiness is a read-only projection of required-mode readiness.
// It never approves or bypasses anything; it only names missing prerequisites.
func inspectRequiredReadiness(cfg config.Config, selection security.Selection, selected []security.AdapterCandidate) (bool, []string) {
	if len(cfg.Security.RequiredCategories) == 0 {
		return false, []string{"required mode needs at least one required category"}
	}
	if !cfg.SecurityEnabled() {
		return false, []string{"global security.enabled is false; a required gate cannot be satisfied"}
	}
	coverage := map[string]security.CategoryCoverage{}
	for _, category := range selection.Coverage {
		coverage[string(category.Category)] = category
	}
	ready := true
	var reasons []string
	for _, raw := range cfg.Security.RequiredCategories {
		category := strings.ToLower(strings.TrimSpace(raw))
		entry, ok := coverage[category]
		if !ok {
			if selection.Detection != nil && len(selection.Detection.Languages) == 0 && availableCategoryAdapter(selected, category) {
				continue
			}
			ready = false
			reasons = append(reasons, fmt.Sprintf("required category %s has no coverage evidence", category))
			continue
		}
		if entry.State != security.CoverageStateCovered {
			ready = false
			reasons = append(reasons, fmt.Sprintf("required category %s coverage is %s", category, entry.State))
		}
		if entry.RepositoryWide && entry.State == security.CoverageStateCovered {
			continue
		}
		for _, language := range entry.Languages {
			if language.State != security.CoverageStateCovered {
				ready = false
				reasons = append(reasons, fmt.Sprintf("required %s coverage for %s is %s", category, language.Language, language.State))
			}
		}
	}
	for _, candidate := range selected {
		if candidate.Status == security.CapabilityAvailable {
			continue
		}
		ready = false
		if candidate.Reason != "" {
			reasons = append(reasons, fmt.Sprintf("adapter %s prerequisite is %s: %s", candidate.ID, candidate.Status, candidate.Reason))
		} else {
			reasons = append(reasons, fmt.Sprintf("adapter %s prerequisite is %s", candidate.ID, candidate.Status))
		}
	}
	if len(reasons) == 0 {
		reasons = []string{"all required categories and detected languages have selected, available adapters"}
	}
	return ready, reasons
}

func availableCategoryAdapter(candidates []security.AdapterCandidate, category string) bool {
	for _, candidate := range candidates {
		if string(candidate.Category) == category && candidate.Status == security.CapabilityAvailable {
			return true
		}
	}
	return false
}

func parseSecurityInspectArgs(args []string, stderr io.Writer) (securityInspectOptions, bool) {
	var opts securityInspectOptions
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--json":
			opts.json = true
		case arg == "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, securityUsage)
				return opts, false
			}
			i++
			opts.config = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--config="):
			opts.config = strings.TrimPrefix(arg, "--config=")
		default:
			fmt.Fprintln(stderr, securityUsage)
			return opts, false
		}
	}
	return opts, true
}

func detectionLanguageText(languages []security.Language) string {
	if len(languages) == 0 {
		return "(none)"
	}
	values := make([]string, 0, len(languages))
	for _, language := range languages {
		values = append(values, string(language))
	}
	return strings.Join(values, ", ")
}

func printCapabilities(w io.Writer, candidates []security.AdapterCandidate) {
	if len(candidates) == 0 {
		fmt.Fprintln(w, "    (none)")
		return
	}
	for _, candidate := range candidates {
		fmt.Fprintf(w, "    %s (%s): %s", candidate.ID, candidate.Category, candidate.Status)
		if candidate.Reason != "" {
			fmt.Fprintf(w, " [%s]", candidate.Reason)
		}
		if candidate.Disabled {
			fmt.Fprintf(w, " [disabled: %s]", candidate.DisabledReason)
		}
		if candidate.Selection != "" {
			fmt.Fprintf(w, " selection=%s", candidate.Selection)
		}
		if candidate.Rules != "" {
			fmt.Fprintf(w, " rules=%s", candidate.Rules)
		}
		if len(candidate.Languages) > 0 {
			values := make([]string, 0, len(candidate.Languages))
			for _, language := range candidate.Languages {
				values = append(values, string(language))
			}
			fmt.Fprintf(w, " languages=%s", strings.Join(values, ","))
		}
		fmt.Fprintln(w)
	}
}

type securityScanOptions struct {
	json     bool
	mode     string
	scope    string
	baseline string
	resume   string
	runID    string
	config   string
	auto     bool
}

func parseSecurityScanArgs(args []string, stderr io.Writer) (securityScanOptions, bool) {
	var opts securityScanOptions
	usage := securityUsage
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--json":
			opts.json = true
		case arg == "--auto":
			opts.auto = true
		case arg == "--report-only":
			opts.mode = config.SecurityModeReportOnly
		case arg == "--required":
			opts.mode = config.SecurityModeRequired
		case arg == "--scope":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return opts, false
			}
			i++
			opts.scope = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--scope="):
			opts.scope = strings.TrimPrefix(arg, "--scope=")
		case arg == "--baseline":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return opts, false
			}
			i++
			opts.baseline = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--baseline="):
			opts.baseline = strings.TrimPrefix(arg, "--baseline=")
		case arg == "--resume":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return opts, false
			}
			i++
			opts.resume = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--resume="):
			opts.resume = strings.TrimPrefix(arg, "--resume=")
		case arg == "--run-id":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return opts, false
			}
			i++
			opts.runID = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--run-id="):
			opts.runID = strings.TrimPrefix(arg, "--run-id=")
		case arg == "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return opts, false
			}
			i++
			opts.config = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--config="):
			opts.config = strings.TrimPrefix(arg, "--config=")
		default:
			fmt.Fprintln(stderr, usage)
			return opts, false
		}
	}
	if opts.scope != "" && opts.scope != config.SecurityScopeWorkspace && opts.scope != config.SecurityScopeChanged {
		fmt.Fprintf(stderr, "security: unknown --scope %q (want workspace, changed-files)\n", opts.scope)
		return opts, false
	}
	if opts.mode != "" && opts.mode != config.SecurityModeReportOnly && opts.mode != config.SecurityModeRequired {
		fmt.Fprintf(stderr, "security: unknown mode %q (want report-only, required)\n", opts.mode)
		return opts, false
	}
	return opts, true
}

// loadPriorSecurityScan loads a previously persisted scan for explicit resume
// and the sanitized native reports it references. A missing or malformed scan is
// an error rather than a silent full rerun, so an operator can see why resume
// could not be used. Native reports are verified by digest; an unusable report is
// omitted, which makes the runner rescan that adapter.
func loadPriorSecurityScan(dir, runID string) (*security.Scan, map[string][]byte, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, nil, nil
	}
	runDir := filepath.Join(dir, config.DirName, "runs", runID)
	path := filepath.Join(runDir, "security", "scan.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("resume: read %s: %w", path, err)
	}
	var scan security.Scan
	if err := json.Unmarshal(data, &scan); err != nil {
		return nil, nil, fmt.Errorf("resume: parse %s: %w", path, err)
	}
	if scan.ProtocolVersion != security.ProtocolVersion {
		return nil, nil, fmt.Errorf("resume: %s speaks protocol %q, want %s", path, scan.ProtocolVersion, security.ProtocolVersion)
	}
	return &scan, security.LoadNativeReports(runDir, &scan), nil
}

func printSecurityScan(w io.Writer, scan security.Scan, refs security.ArtifactRefs) {
	fmt.Fprintf(w, "security: %s (coverage %s, %dms)\n", scan.Verdict, scan.Coverage, scan.Metrics.DurationMS)
	if scan.Content.Commit != "" {
		fmt.Fprintf(w, "  commit: %s\n", scan.Content.Commit)
	}
	if scan.Content.Tree != "" {
		fmt.Fprintf(w, "  tree: %s\n", scan.Content.Tree)
	}
	if scan.Content.Dirty {
		fmt.Fprintf(w, "  working tree: DIRTY (content digest %s)\n", scan.Content.Digest)
	}
	for _, adapter := range scan.Adapters {
		fmt.Fprintf(w, "  adapter %s (%s): %s, coverage %s", adapter.AdapterID, adapter.Category, adapter.Status, adapter.Coverage)
		if adapter.Disabled {
			fmt.Fprint(w, ", disabled")
		} else if adapter.Selection != "" {
			fmt.Fprintf(w, ", selection %s", adapter.Selection)
		}
		if adapter.RepositoryWide {
			fmt.Fprint(w, ", repository-wide")
		} else if len(adapter.Languages) > 0 {
			fmt.Fprintf(w, ", languages %s", detectionLanguageText(adapter.Languages))
		}
		if adapter.Reused {
			fmt.Fprint(w, ", reused")
		}
		fmt.Fprintln(w)
	}
	if len(scan.Overlaps) > 0 {
		for _, overlap := range scan.Overlaps {
			scope := string(overlap.Language)
			if overlap.RepositoryWide {
				scope = "repository-wide"
			}
			fmt.Fprintf(w, "  overlap: %s (%s): %s\n", overlap.Category, scope, strings.Join(overlap.Adapters, ", "))
		}
	}
	for _, coverage := range scan.CoverageReport {
		fmt.Fprintf(w, "  coverage %s: %s\n", coverage.Category, coverage.State)
		for _, language := range coverage.Languages {
			fmt.Fprintf(w, "    %s: %s\n", language.Language, language.State)
		}
	}
	for _, adapter := range scan.Adapters {
		if adapter.Disabled || adapter.Provenance.AdvisoryDatabaseIdentity == "" {
			continue
		}
		fmt.Fprintf(w, "  database %s: %s", adapter.AdapterID, adapter.Provenance.AdvisoryDatabaseIdentity)
		if adapter.Provenance.AdvisoryDatabaseStale {
			fmt.Fprint(w, " (STALE)")
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  findings: %d (suppressed %d)\n", scan.Metrics.Findings, scan.Metrics.Suppressed)
	for _, reason := range scan.Reasons {
		fmt.Fprintf(w, "  - %s\n", reason)
	}
	if scan.RequiredMode != nil {
		fmt.Fprintf(w, "  required-mode readiness: %t (projected %s)\n", scan.RequiredMode.Ready, scan.RequiredMode.Verdict)
	}
	if scan.Detection != nil {
		fmt.Fprintf(w, "  detected languages: %s\n", detectionLanguageText(scan.Detection.Languages))
		for _, lockfile := range scan.Detection.Lockfiles {
			state := "present"
			if !lockfile.Present {
				state = "MISSING"
			}
			fmt.Fprintf(w, "  lockfile: %s (%s, %s)\n", lockfile.Path, lockfile.Language, state)
		}
	}
	fmt.Fprintf(w, "  report: .agent-sdlc/runs/%s/%s\n", scan.RunID, refs.ScanMarkdown)
	fmt.Fprintf(w, "  evidence: .agent-sdlc/runs/%s/%s\n", scan.RunID, refs.ScanJSON)
}
