package security

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// RunnerConfig is one bounded security stage.
type RunnerConfig struct {
	WorkspaceRoot  string
	RunID          string
	TaskID         string
	AttemptID      string
	Adapters       []Adapter
	Policy         Policy
	Timeout        time.Duration
	MaxOutputBytes int
	Baseline       Baseline
	// Prior, when set, is a previously persisted scan used only for explicit
	// resume. A completed adapter result is reused only when every identity still
	// matches; any mismatch re-executes that adapter.
	Prior *Scan
}

// Runner orchestrates adapters. It performs no retries of its own; the caller's
// existing run/attempt/continuation budgets remain the only continuation loop.
type Runner struct {
	cfg      RunnerConfig
	policyID string
}

// NewRunner validates the stage configuration. It is deterministic and does not
// execute anything.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	root, err := ensureWorkspaceRoot(cfg.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	cfg.WorkspaceRoot = root
	if strings.TrimSpace(cfg.RunID) == "" {
		return nil, fmt.Errorf("security: run id is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 1 << 20
	}
	seen := make(map[string]bool, len(cfg.Adapters))
	categories := make(map[Category]bool, len(cfg.Adapters))
	for _, adapter := range cfg.Adapters {
		descriptor := adapter.Descriptor()
		if err := descriptor.Validate(); err != nil {
			return nil, err
		}
		if seen[descriptor.AdapterID] {
			return nil, fmt.Errorf("security: duplicate adapter id %q", descriptor.AdapterID)
		}
		seen[descriptor.AdapterID] = true
		for _, category := range descriptor.Categories {
			categories[category] = true
		}
	}
	if cfg.Policy.Enabled {
		for _, raw := range cfg.Policy.RequiredCategories {
			category := Category(strings.ToLower(strings.TrimSpace(raw)))
			if !categories[category] {
				return nil, fmt.Errorf("security: required category %q has no configured adapter", raw)
			}
		}
	}
	return &Runner{cfg: cfg, policyID: cfg.Policy.Identity(cfg.Baseline)}, nil
}

// Scan runs every configured adapter once and evaluates policy. A disabled
// add-on returns NOT_APPLICABLE without executing an adapter.
func (r *Runner) Scan(ctx context.Context) (Scan, error) {
	started := time.Now().UTC()
	scan := Scan{
		ProtocolVersion: ProtocolVersion,
		RunID:           r.cfg.RunID,
		TaskID:          r.cfg.TaskID,
		StartedAt:       started,
		Policy:          r.cfg.Policy,
		Metrics:         Metrics{AdapterDurationMS: map[string]int64{}},
	}
	content, err := ComputeContentIdentity(ctx, r.cfg.WorkspaceRoot)
	if err != nil {
		return scan, err
	}
	scope, err := ComputeScope(ctx, r.cfg.WorkspaceRoot, r.cfg.Policy.Scope)
	if err != nil {
		return scan, err
	}
	scope.Excluded = NormalizeExcludes(r.cfg.Policy.Exclude)
	scan.Content = content
	scan.Scope = scope

	if !r.cfg.Policy.Enabled {
		scan.Verdict = VerdictNotApplicable
		scan.Coverage = CoverageUnknown
		scan.Reasons = []string{"security add-on is disabled"}
		scan.EndedAt = time.Now().UTC()
		scan.Metrics.DurationMS = scan.EndedAt.Sub(scan.StartedAt).Milliseconds()
		return scan, nil
	}
	if len(r.cfg.Adapters) == 0 {
		scan.Verdict = VerdictIndeterminate
		scan.Coverage = CoverageUnknown
		scan.Reasons = []string{"security add-on is enabled but no adapter is configured"}
		scan.EndedAt = time.Now().UTC()
		scan.Metrics.DurationMS = scan.EndedAt.Sub(scan.StartedAt).Milliseconds()
		return scan, nil
	}

	req := Request{
		ProtocolVersion: ProtocolVersion,
		RunID:           r.cfg.RunID,
		TaskID:          r.cfg.TaskID,
		AttemptID:       r.cfg.AttemptID,
		WorkspaceRoot:   r.cfg.WorkspaceRoot,
		Content:         content,
		Scope:           scope,
	}
	if deadline, ok := ctx.Deadline(); ok {
		req.DeadlineMS = time.Until(deadline).Milliseconds()
	}

	var diagnostics []string
	for _, adapter := range r.cfg.Adapters {
		descriptor := adapter.Descriptor()
		adapterStart := time.Now().UTC()
		exec, reused := r.maybeReuse(req, descriptor)
		if !reused {
			exec, err = adapter.Scan(ctx, req)
			if err != nil {
				exec = newExecution(descriptor.AdapterID, descriptor.Categories[0], descriptor.AdapterVersion, "", r.policyID, req)
				exec.Status = StatusFailed
				exec.Diagnostics = []string{"adapter failed: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
			}
		}
		if exec.ProtocolVersion == "" {
			exec.ProtocolVersion = ProtocolVersion
		}
		exec.AdapterID = descriptor.AdapterID
		if exec.Category == "" {
			exec.Category = descriptor.Categories[0]
		}
		if exec.AdapterVersion == "" {
			exec.AdapterVersion = descriptor.AdapterVersion
		}
		exec.Provenance.PolicyIdentity = r.policyID
		if exec.Provenance.ContentIdentity.Commit == "" && exec.Provenance.ContentIdentity.Tree == "" && exec.Provenance.ContentIdentity.Digest == "" {
			exec.Provenance.ContentIdentity = content
		}
		if !exec.Reused {
			exec.StartedAt = nonzeroTime(exec.StartedAt, adapterStart)
			exec.EndedAt = nonzeroTime(exec.EndedAt, time.Now().UTC())
		}
		sanitizeExecution(&exec)
		if err := exec.Validate(); err != nil {
			exec.Status = StatusFailed
			exec.Coverage = CoverageUnknown
			exec.Diagnostics = append(exec.Diagnostics, "contract validation failed: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		}

		// Baseline/suppression matching happens per execution, before policy. The
		// finding remains visible and only loses its blocking effect.
		findings, baselineDiagnostics := r.cfg.Baseline.Apply(exec.Findings, time.Now().UTC())
		exec.Findings = findings
		diagnostics = append(diagnostics, baselineDiagnostics...)
		scan.Adapters = append(scan.Adapters, exec)

		duration := int64(0)
		if !exec.Reused && !exec.StartedAt.IsZero() && !exec.EndedAt.IsZero() {
			duration = exec.EndedAt.Sub(exec.StartedAt).Milliseconds()
		}
		scan.Metrics.AdapterDurationMS[exec.AdapterID] = duration
		if exec.Status != StatusCompleted {
			scan.Metrics.ExecutionFailures++
		}
	}

	evaluation := EvaluatePolicy(scan.Adapters, r.cfg.Policy)
	scan.Findings = evaluation.Findings
	scan.Verdict = evaluation.Verdict
	scan.Coverage = evaluation.Coverage
	scan.Reasons = evaluation.Reasons
	scan.Diagnostics = append(diagnostics, evaluation.Diagnostics...)
	scan.Metrics.Findings = len(scan.Findings)
	for _, finding := range scan.Findings {
		if finding.Suppressed {
			scan.Metrics.Suppressed++
		}
	}
	scan.EndedAt = time.Now().UTC()
	scan.Metrics.DurationMS = scan.EndedAt.Sub(scan.StartedAt).Milliseconds()
	return scan, nil
}

// maybeReuse returns a prior completed execution only when the current adapter,
// content, rules, configuration, scanner version, and policy identities all still
// match. The check is deliberately fail-closed: a missing current identity means
// no reuse.
func (r *Runner) maybeReuse(req Request, descriptor Descriptor) (Execution, bool) {
	if r.cfg.Prior == nil {
		return Execution{}, false
	}
	provider, ok := adapterProvenanceProvider(r.cfg.Adapters, descriptor.AdapterID)
	if !ok {
		return Execution{}, false
	}
	current, err := provider.currentProvenance(req)
	if err != nil {
		return Execution{}, false
	}
	current.PolicyIdentity = r.policyID
	current.ContentIdentity = req.Content
	if strings.TrimSpace(current.ScannerVersion) == "" {
		return Execution{}, false
	}
	for _, prior := range r.cfg.Prior.Adapters {
		if prior.AdapterID != descriptor.AdapterID || prior.Category != descriptor.Categories[0] {
			continue
		}
		if prior.Status != StatusCompleted {
			return Execution{}, false
		}
		if !sameContent(prior.Provenance.ContentIdentity, current.ContentIdentity) ||
			prior.Provenance.PolicyIdentity != current.PolicyIdentity ||
			prior.Provenance.ConfigurationDigest != current.ConfigurationDigest ||
			prior.Provenance.RulesIdentity != current.RulesIdentity ||
			prior.Provenance.ScannerVersion != current.ScannerVersion {
			return Execution{}, false
		}
		reused := prior
		reused.Reused = true
		return reused, true
	}
	return Execution{}, false
}

type provenanceProvider interface {
	currentProvenance(req Request) (Provenance, error)
}

func adapterProvenanceProvider(adapters []Adapter, id string) (provenanceProvider, bool) {
	for _, adapter := range adapters {
		if adapter.Descriptor().AdapterID != id {
			continue
		}
		provider, ok := adapter.(provenanceProvider)
		return provider, ok
	}
	return nil, false
}

func sameContent(a, b ContentIdentity) bool {
	return reflect.DeepEqual(a, b)
}
