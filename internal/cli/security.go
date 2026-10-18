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
	if !cfg.SecurityEnabled() {
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
	adapterConfigs := cfg.Security.Adapters
	var detection *security.Detection
	if auto {
		detected, derr := security.DetectRepositoryScoped(dir, cfg.SecurityExcludes())
		if derr != nil {
			fmt.Fprintf(stderr, "security: detect repository: %v\n", derr)
			return exitError
		}
		detection = &detected
		adapterConfigs = security.AutoAdapterConfigs(detected, cfg.Security.Adapters, cfg.Security.AdvisoryDatabasePath)
	}

	adapters, err := security.BuildAdapters(adapterConfigs, security.BuildOptions{
		Timeout:        time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes: cfg.Security.MaxOutputBytes,
		DatabasePath:   cfg.Security.AdvisoryDatabasePath,
		StalePolicy:    cfg.Security.StaleAdvisoryDatabase,
	})
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	policy := security.Policy{
		Enabled:               true,
		Mode:                  mode,
		Scope:                 scope,
		RequiredCategories:    cfg.Security.RequiredCategories,
		FailOn:                cfg.Security.FailOn,
		UnknownSeverity:       cfg.Security.UnknownSeverity,
		TimeoutSeconds:        cfg.Security.TimeoutSeconds,
		MaxOutputBytes:        cfg.Security.MaxOutputBytes,
		Exclude:               cfg.SecurityExcludes(),
		AdvisoryDatabasePath:  cfg.Security.AdvisoryDatabasePath,
		StaleAdvisoryDatabase: cfg.Security.StaleAdvisoryDatabase,
		Auto:                  auto,
	}

	runID := opts.runID
	if runID == "" {
		runID = "security-" + time.Now().UTC().Format("20060102T150405Z")
	}
	prior, priorNative, err := loadPriorSecurityScan(dir, opts.resume)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	runner, err := security.NewRunner(security.RunnerConfig{
		WorkspaceRoot:      dir,
		RunID:              runID,
		Adapters:           adapters,
		Policy:             policy,
		Timeout:            time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes:     cfg.Security.MaxOutputBytes,
		Baseline:           baseline,
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
	scan.Detection = detection
	scan.Diagnostics = append(baselineDiagnostics, scan.Diagnostics...)

	rn, err := runpkg.Open(dir, runID)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
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

	// Report-only records the verdict without blocking delivery. Required mode
	// exits non-zero for FAIL or INDETERMINATE.
	if policy.Mode == config.SecurityModeRequired && scan.Verdict != security.VerdictPass {
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
	configured := security.ClassifyCapabilitiesWithDatabase(detection, cfg.Security.Adapters, cfg.Security.AdvisoryDatabasePath)
	proposed := security.SelectCandidatesWithDatabase(detection, cfg.Security.Adapters, cfg.Security.AdvisoryDatabasePath)
	result := struct {
		Version    int                         `json:"version"`
		Enabled    bool                        `json:"enabled"`
		Mode       string                      `json:"mode,omitempty"`
		Scope      string                      `json:"scope,omitempty"`
		Auto       bool                        `json:"auto"`
		Detection  security.Detection          `json:"detection"`
		Configured []security.AdapterCandidate `json:"configured"`
		Proposed   []security.AdapterCandidate `json:"proposed"`
	}{
		Version: 1, Enabled: cfg.SecurityEnabled(), Mode: cfg.Security.Mode, Scope: cfg.Security.Scope,
		Auto: cfg.Security.Auto, Detection: detection, Configured: configured, Proposed: proposed,
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
	fmt.Fprintln(stdout, "  proposed adapters:")
	printCapabilities(stdout, proposed)
	return exitOK
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
		if candidate.Rules != "" {
			fmt.Fprintf(w, " rules=%s", candidate.Rules)
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
		if adapter.Reused {
			fmt.Fprint(w, ", reused")
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
