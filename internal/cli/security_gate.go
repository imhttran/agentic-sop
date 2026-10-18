package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/security"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runSecurityGate executes the optional security stage at the existing quality
// seam. Configuration or adapter failures become an INDETERMINATE scan with
// evidence instead of a run infrastructure failure: report-only records that
// result without blocking, and required mode blocks through the quality gate. An
// artifact-write failure is still an error, because evidence that cannot be
// persisted is not evidence.
func runSecurityGate(ctx context.Context, cfg config.Config, dir string, spec *taskfile.Spec, rn *runpkg.Run) (*security.Scan, error) {
	policy := security.Policy{
		Enabled:            true,
		Mode:               cfg.Security.Mode,
		Scope:              cfg.Security.Scope,
		RequiredCategories: cfg.Security.RequiredCategories,
		FailOn:             cfg.Security.FailOn,
		UnknownSeverity:    cfg.Security.UnknownSeverity,
		TimeoutSeconds:     cfg.Security.TimeoutSeconds,
		MaxOutputBytes:     cfg.Security.MaxOutputBytes,
		Exclude:            cfg.SecurityExcludes(),
	}
	scan := &security.Scan{
		ProtocolVersion: security.ProtocolVersion,
		RunID:           rn.State().ID,
		TaskID:          spec.ID,
		StartedAt:       time.Now().UTC(),
		Policy:          policy,
		Metrics:         security.Metrics{AdapterDurationMS: map[string]int64{}},
	}
	indeterminate := func(reason string) (*security.Scan, error) {
		scan.Verdict = security.VerdictIndeterminate
		scan.Coverage = security.CoverageUnknown
		scan.Reasons = append(scan.Reasons, reason)
		scan.EndedAt = time.Now().UTC()
		scan.Metrics.DurationMS = scan.EndedAt.Sub(scan.StartedAt).Milliseconds()
		if _, err := security.WriteArtifacts(rn.Dir(), *scan); err != nil {
			return nil, err
		}
		return scan, nil
	}

	baseline, baselineDiagnostics, err := security.LoadBaseline(dir, cfg.Security.Baseline)
	if err != nil {
		return indeterminate("baseline could not be loaded: " + err.Error())
	}
	scan.Diagnostics = append(scan.Diagnostics, baselineDiagnostics...)
	adapters, err := security.BuildAdapters(cfg.Security.Adapters, security.BuildOptions{
		Timeout:        time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes: cfg.Security.MaxOutputBytes,
	})
	if err != nil {
		return indeterminate("adapter configuration is invalid: " + err.Error())
	}
	runner, err := security.NewRunner(security.RunnerConfig{
		WorkspaceRoot:  dir,
		RunID:          rn.State().ID,
		TaskID:         spec.ID,
		Adapters:       adapters,
		Policy:         policy,
		Timeout:        time.Duration(cfg.Security.TimeoutSeconds) * time.Second,
		MaxOutputBytes: cfg.Security.MaxOutputBytes,
		Baseline:       baseline,
	})
	if err != nil {
		return indeterminate("security configuration is invalid: " + err.Error())
	}
	result, err := runner.Scan(ctx)
	if err != nil {
		return indeterminate("security scan could not produce evidence: " + err.Error())
	}
	scan = &result
	if scan.Diagnostics == nil {
		scan.Diagnostics = baselineDiagnostics
	} else {
		scan.Diagnostics = append(append([]string(nil), baselineDiagnostics...), scan.Diagnostics...)
	}
	if _, err := security.WriteArtifacts(rn.Dir(), *scan); err != nil {
		return nil, fmt.Errorf("write security artifacts: %w", err)
	}
	return scan, nil
}

// securityGateEvidence shapes scan evidence for the single quality gate.
func securityGateEvidence(cfg config.Config, scan *security.Scan) *quality.SecurityEvidence {
	if scan == nil {
		return nil
	}
	blocking := 0
	for _, finding := range scan.Findings {
		if !finding.Suppressed && security.SeverityBlocked(cfg.Security.FailOn, finding.Severity) {
			blocking++
		}
	}
	reason := ""
	if len(scan.Reasons) > 0 {
		reason = scan.Reasons[0]
	}
	if reason == "" && len(scan.Diagnostics) > 0 {
		reason = scan.Diagnostics[0]
	}
	if blocking > 0 {
		reason = fmt.Sprintf("%d blocking finding(s)", blocking)
	}
	return &quality.SecurityEvidence{
		Enabled:    true,
		Required:   cfg.Security.Mode == config.SecurityModeRequired,
		Configured: len(cfg.Security.Adapters) > 0,
		Verdict:    string(scan.Verdict),
		Blocking:   blocking,
		Reason:     reason,
	}
}
