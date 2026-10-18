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
// NOT_APPLICABLE and executes no scanner.
func runSecurity(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 || args[0] != "scan" {
		fmt.Fprintln(stderr, "usage: sop security scan [--json] [--report-only|--required] [--scope workspace|changed-files] [--baseline PATH] [--resume RUN_ID] [--run-id ID] [--config PATH]")
		return exitUsage
	}
	opts, ok := parseSecurityScanArgs(args[1:], stderr)
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

	adapters, err := security.BuildAdapters(cfg.Security.Adapters, security.BuildOptions{
		Timeout:        time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes: cfg.Security.MaxOutputBytes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	policy := security.Policy{
		Enabled:            true,
		Mode:               mode,
		Scope:              scope,
		RequiredCategories: cfg.Security.RequiredCategories,
		FailOn:             cfg.Security.FailOn,
		UnknownSeverity:    cfg.Security.UnknownSeverity,
		TimeoutSeconds:     cfg.Security.TimeoutSeconds,
		MaxOutputBytes:     cfg.Security.MaxOutputBytes,
	}

	runID := opts.runID
	if runID == "" {
		runID = "security-" + time.Now().UTC().Format("20060102T150405Z")
	}
	prior, err := loadPriorSecurityScan(dir, opts.resume)
	if err != nil {
		fmt.Fprintf(stderr, "security: %v\n", err)
		return exitError
	}
	runner, err := security.NewRunner(security.RunnerConfig{
		WorkspaceRoot:  dir,
		RunID:          runID,
		Adapters:       adapters,
		Policy:         policy,
		Timeout:        time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes: cfg.Security.MaxOutputBytes,
		Baseline:       baseline,
		Prior:          prior,
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

type securityScanOptions struct {
	json     bool
	mode     string
	scope    string
	baseline string
	resume   string
	runID    string
	config   string
}

func parseSecurityScanArgs(args []string, stderr io.Writer) (securityScanOptions, bool) {
	var opts securityScanOptions
	usage := "usage: sop security scan [--json] [--report-only|--required] [--scope workspace|changed-files] [--baseline PATH] [--resume RUN_ID] [--run-id ID] [--config PATH]"
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--json":
			opts.json = true
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

// loadPriorSecurityScan loads a previously persisted scan for explicit resume.
// A missing or malformed artifact is an error rather than a silent full rerun, so
// an operator can see why resume could not be used.
func loadPriorSecurityScan(dir, runID string) (*security.Scan, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	path := filepath.Join(dir, config.DirName, "runs", runID, "security", "scan.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("resume: read %s: %w", path, err)
	}
	var scan security.Scan
	if err := json.Unmarshal(data, &scan); err != nil {
		return nil, fmt.Errorf("resume: parse %s: %w", path, err)
	}
	if scan.ProtocolVersion != security.ProtocolVersion {
		return nil, fmt.Errorf("resume: %s speaks protocol %q, want %s", path, scan.ProtocolVersion, security.ProtocolVersion)
	}
	return &scan, nil
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
	fmt.Fprintf(w, "  report: .agent-sdlc/runs/%s/%s\n", scan.RunID, refs.ScanMarkdown)
	fmt.Fprintf(w, "  evidence: .agent-sdlc/runs/%s/%s\n", scan.RunID, refs.ScanJSON)
}
