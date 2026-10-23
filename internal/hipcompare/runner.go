package hipcompare

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Arm identifies one side of the comparison.
type Arm string

const (
	// ArmBaseline runs the baseline instruction set.
	ArmBaseline Arm = "baseline"
	// ArmCandidate runs the candidate instruction set in an isolated worktree.
	ArmCandidate Arm = "candidate"
)

// DisposableWorktree is the minimal worktree lifecycle the runner needs. It is
// satisfied by the production Git worktree lifecycle and by a fake in tests, so
// the comparison reuses the existing Git worktree tooling rather than shelling
// out on its own.
type DisposableWorktree interface {
	// Create adds a linked worktree at path from commit on a new branch.
	Create(ctx context.Context, path, commit, branch string) error
	// Remove deletes the worktree at path.
	Remove(ctx context.Context, path string) error
}

// ArmExecutor runs one arm's task inside worktree. One instance is built and
// passed to both arms, so the executor, model, settings, tool permissions, and
// verification commands cannot differ between arms.
type ArmExecutor interface {
	// Execute runs the task in in.Worktree and returns the observed outcome.
	Execute(ctx context.Context, in ExecutorInput) (ExecutorResult, error)
}

// ExecutorInput is the shared, identical description handed to the executor for
// every arm. Nothing arm-specific appears here except the arm's own instructions:
// the worktree and starting commit are explicit fields so the executor cannot
// choose a different base for one side.
type ExecutorInput struct {
	Manifest             *TaskManifest
	VerificationCommands []string
	AllowedPaths         []string
	Worktree             string
	StartingCommit       string
	// Executor and its settings are shared verbatim by both arms.
	Executor        string
	Model           string
	Settings        string
	ToolPermissions []string
	// Instructions is the arm's instruction text (a prompt or skill revision).
	// The executor applies it only to this arm's declared files, already inside the
	// manifest's allowed list; the runner enforces the guard after execution too.
	Instructions InstructionSet
}

// ExecutorResult is what one arm produced.
type ExecutorResult struct {
	Completed          bool
	AcceptancePassed   bool
	HumanInterventions int
	TokensUsed         int
	// TokensReported distinguishes a measured zero from an unavailable token
	// measure. A command executor that cannot count tokens reports false.
	TokensReported bool
	// Output is the executor's bounded combined output. The runner persists it
	// beside the copied run artifacts when an evidence root is configured.
	Output string
	// InstructionsApplied lists the candidate instruction files actually written
	// into the worktree before the executor ran.
	InstructionsApplied []string
	Failures            []string
	EvidencePaths       []string
}

// ArmResult is the recorded outcome of one arm execution. It is always produced
// by an executed arm, even a failed one, so the results file never loses an arm.
type ArmResult struct {
	Arm                 Arm      `json:"arm"`
	Worktree            string   `json:"worktree"`
	StartingCommit      string   `json:"starting_commit"`
	Completed           bool     `json:"completed"`
	AcceptancePassed    bool     `json:"acceptance_passed"`
	HumanInterventions  int      `json:"human_interventions"`
	TokensUsed          int      `json:"tokens_used"`
	TokensReported      bool     `json:"tokens_reported"`
	Output              string   `json:"output,omitempty"`
	InstructionsApplied []string `json:"instructions_applied,omitempty"`
	ElapsedMillis       int64    `json:"elapsed_millis"`
	Failures            []string `json:"failures"`
	EvidencePaths       []string `json:"evidence_paths"`
	ChangedPaths        []string `json:"changed_paths"`
	LimitFailure        bool     `json:"limit_failure"`
	LimitFailureReason  string   `json:"limit_failure_reason,omitempty"`
}

// RunnerConfig is everything Run needs. The executor settings are held once and
// shared by both arms, so an A/B difference cannot come from a settings change.
type RunnerConfig struct {
	Manifest  *TaskManifest
	Baseline  *InstructionSet
	Candidate *InstructionSet
	Budget    Budget
	// Executor fields are shared by both arms.
	Executor        string
	Model           string
	Settings        string
	ToolPermissions []string
	// StartingCommit is the commit both arms start from. It is required.
	StartingCommit string
	Worktrees      DisposableWorktree
	WorktreeRoot   string
	// Exec is the shared arm executor.
	Exec ArmExecutor
	// Repository is the primary checkout: worktrees are created around it.
	// EvidenceRoot, when set, receives a copy of each arm's SOP run artifacts and
	// captured executor output before the worktree is removed.
	Repository   string
	EvidenceRoot string
	// Ledger is the shared experiment-budget reservation ledger. Each arm is
	// reserved before it launches and remains counted on failure or restart.
	Ledger    *ExperimentLedger
	OutputDir string
	Now       func() time.Time
	// PriorExecutions is the number of arm executions already recorded for this
	// fingerprint in earlier invocations. It is added to this run's executions when
	// the budget gate checks the ceiling, so a caller cannot exceed the fixed count
	// by re-invoking the command.
	PriorExecutions int
	// PriorArms are the arm records already captured for this fingerprint in earlier
	// invocations. They are carried forward so a resumed incomplete comparison keeps
	// every executed arm in the results file and never loses execution accounting.
	PriorArms []ArmResult
}

// Comparison is the final result the writers serialize.
type Comparison struct {
	Version        string      `json:"version"`
	TaskID         string      `json:"task_id"`
	StartingCommit string      `json:"starting_commit"`
	Fingerprint    Fingerprint `json:"fingerprint"`
	Arms           []ArmResult `json:"arms"`
	Budget         Budget      `json:"budget_usage"`
	Executions     int         `json:"executions"`
	// Completed is true only when every planned arm ran and none hit a limit. It
	// records batch completion, not experimental success; Verdict carries the
	// acceptance result.
	Completed bool `json:"completed"`
	// Verdict states whether the comparison is eligible, not eligible, or
	// inconclusive. It is derived from the arm acceptance results and telemetry.
	Verdict string `json:"verdict"`
}

// BudgetError reports an attempt to execute beyond the fixed budget.
type BudgetError struct{ Reason string }

func (e *BudgetError) Error() string { return e.Reason }

// DisallowedEditError reports a candidate edit outside the declared allowed list.
type DisallowedEditError struct{ Path string }

func (e *DisallowedEditError) Error() string {
	return fmt.Sprintf("rejected: the candidate edited %q, which is not in allowed_candidate_paths", e.Path)
}

// Run executes both arms from the same starting commit in separate disposable
// worktrees, rejecting any candidate edit outside the declared allowed list.
func Run(ctx context.Context, cfg RunnerConfig, fp Fingerprint) (Comparison, error) {
	if err := cfg.Budget.Validate(); err != nil {
		return Comparison{}, err
	}
	if err := cfg.Manifest.Validate(); err != nil {
		return Comparison{}, err
	}
	if err := cfg.Baseline.Validate(); err != nil {
		return Comparison{}, fmt.Errorf("baseline: %w", err)
	}
	if len(cfg.Baseline.Files) != 0 {
		return Comparison{}, errors.New("baseline must not declare edits: only the candidate may apply a prompt or skill change")
	}
	if err := cfg.Candidate.ValidateFor(cfg.Manifest); err != nil {
		return Comparison{}, fmt.Errorf("candidate: %w", err)
	}
	if cfg.Exec == nil {
		return Comparison{}, errors.New("Run: no arm executor configured")
	}
	if cfg.Worktrees == nil {
		return Comparison{}, errors.New("Run: no worktree manager configured")
	}
	if cfg.Ledger == nil {
		return Comparison{}, errors.New("Run: shared budget ledger is required")
	}
	if cfg.StartingCommit == "" {
		return Comparison{}, errors.New("Run: starting commit is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	cmp := Comparison{
		Version:        SchemaVersion,
		TaskID:         cfg.Manifest.ID,
		StartingCommit: cfg.StartingCommit,
		Fingerprint:    fp,
		Budget:         cfg.Budget,
		Executions:     cfg.PriorExecutions,
	}
	base := cfg.WorktreeRoot
	if base == "" {
		base = os.TempDir()
	}

	steps := []struct {
		arm   Arm
		set   *InstructionSet
		apply bool
	}{
		{ArmBaseline, cfg.Baseline, false},
		{ArmCandidate, cfg.Candidate, true},
	}

	// Budget gate: the shared ledger reserves each arm before launch. The
	// per-comparison PriorExecutions value is retained only for result accounting;
	// it cannot substitute for the shared reservation.
	planned := len(steps)
	_ = planned

	// Carry forward prior arm records so a resumed incomplete comparison keeps its
	// execution accounting and never loses an already-executed arm.
	if len(cfg.PriorArms) > 0 {
		cmp.Arms = append(cmp.Arms, cfg.PriorArms...)
	}

	allOK := true
	for _, s := range steps {
		reservation, rerr := cfg.Ledger.Reserve(fp.Digest, cfg.Manifest.ID, s.arm, cfg.OutputDir)
		if rerr != nil {
			cmp.Verdict = "Inconclusive: the shared experiment budget refused the next execution."
			return cmp, rerr
		}
		res, err := runArm(ctx, cfg, base, fp, s.arm, s.set, s.apply, now)
		status := LedgerFailed
		if res != nil {
			if res.LimitFailure {
				status = LedgerLimitFailure
			} else if res.Completed && res.AcceptancePassed {
				status = LedgerCompleted
			}
		}
		if cerr := cfg.Ledger.Complete(reservation, status); cerr != nil && err == nil {
			err = cerr
		}
		if res != nil {
			cmp.Arms = append(cmp.Arms, *res)
			cmp.Executions++
			if res.LimitFailure {
				allOK = false
			}
		}
		if err != nil {
			return cmp, err
		}
		if res != nil && res.LimitFailure {
			return cmp, nil
		}
	}
	cmp.Completed = allOK && len(cmp.Arms) >= planned
	cmp.Verdict = verdict(cmp)
	return cmp, nil
}

// enforceExecutionBudget checks the fixed execution ceiling, counting the
// executions a prior invocation already recorded for this fingerprint.
func enforceExecutionBudget(b Budget, prior, planned int) error {
	ceiling := b.ExecutionCeiling()
	if prior < 0 {
		return &BudgetError{Reason: fmt.Sprintf("budget: prior executions %d is invalid", prior)}
	}
	if prior+planned > ceiling {
		return &BudgetError{Reason: fmt.Sprintf(
			"budget exhaustion: %d prior + %d planned executions exceed the fixed maximum %d; refusing to invoke the adapter",
			prior, planned, ceiling)}
	}
	return nil
}

func runArm(ctx context.Context, cfg RunnerConfig, base string, fp Fingerprint, arm Arm, set *InstructionSet, apply bool, now func() time.Time) (*ArmResult, error) {
	commit := cfg.StartingCommit
	// A unique suffix per arm keeps repeated or concurrent runs from colliding on
	// the same worktree path or branch name. The comparison fingerprint's first
	// short hash plus a timestamp gives a stable-but-unique identity.
	short := fpShort(fp)
	suffix := fmt.Sprintf("%s-%d", short, now().UnixNano())
	name := fmt.Sprintf("hipcompare-%s-%s-%s", cfg.Manifest.ID, arm, suffix)
	branch := fmt.Sprintf("hipcompare/%s/%s/%s", cfg.Manifest.ID, arm, suffix)
	worktree := filepath.Join(base, name)

	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create worktree root: %w", err)
	}
	_ = os.RemoveAll(worktree)
	if err := cfg.Worktrees.Create(ctx, worktree, commit, branch); err != nil {
		return nil, fmt.Errorf("create %s worktree: %w", arm, err)
	}
	// Always remove the worktree, whether the arm succeeded, failed, or timed out.
	defer func() { _ = cfg.Worktrees.Remove(ctx, worktree) }()

	input := ExecutorInput{
		Manifest:             cfg.Manifest,
		VerificationCommands: cfg.Manifest.VerificationCommands,
		AllowedPaths:         cfg.Manifest.AllowedCandidatePaths,
		Worktree:             worktree,
		StartingCommit:       commit,
		Executor:             cfg.Executor,
		Model:                cfg.Model,
		Settings:             cfg.Settings,
		ToolPermissions:      append([]string{}, cfg.ToolPermissions...),
		Instructions:         *set,
	}

	started := now()
	deadline := started.Add(time.Duration(cfg.Budget.PerRunTimeSeconds) * time.Second)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	ec, execErr := cfg.Exec.Execute(runCtx, input)
	elapsed := now().Sub(started).Milliseconds()

	res := &ArmResult{
		Arm:                 arm,
		Worktree:            name,
		StartingCommit:      commit,
		HumanInterventions:  ec.HumanInterventions,
		TokensUsed:          ec.TokensUsed,
		TokensReported:      ec.TokensReported,
		Output:              ec.Output,
		InstructionsApplied: append([]string{}, ec.InstructionsApplied...),
		ElapsedMillis:       elapsed,
		Failures:            append([]string{}, ec.Failures...),
		EvidencePaths:       append([]string{}, ec.EvidencePaths...),
		Completed:           ec.Completed,
		AcceptancePassed:    ec.AcceptancePassed,
	}

	// Preserve the arm's SOP run artifacts and captured output before the deferred
	// worktree removal discards them. A copy failure is visible but does not hide
	// the arm result.
	if cfg.EvidenceRoot != "" {
		if err := persistArmEvidence(cfg.EvidenceRoot, worktree, cfg.Manifest.ID, arm, res); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("evidence copy failed: %v", err))
		}
	}

	// Change detection records the task's implementation footprint. It deliberately
	// does not reject ordinary task edits: both arms are expected to do the task. The
	// candidate instruction guard is enforced before the executor runs, in
	// commandExecutor.Execute, where only the declared prompt/skill files may be
	// applied. Harness-owned SOP state and the staged task file are not task edits.
	changed, cerr := changedPaths(ctx, worktree, commit)
	if cerr != nil {
		res.Failures = append(res.Failures, fmt.Sprintf("change detection failed: %v", cerr))
		return res, cerr
	}
	res.ChangedPaths = filterHarnessPaths(changed, cfg.Manifest.TaskFile)

	// Enforce the time limit as an observation, even when the executor ignores the
	// context. A run that overran its deadline is a limit failure, never a completed
	// arm.
	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded || elapsed > timeLimitMillis(cfg.Budget.PerRunTimeSeconds) {
			res.Failures = append(res.Failures, "time limit exceeded")
			res.LimitFailure = true
			res.LimitFailureReason = fmt.Sprintf("per-run time limit of %ds exceeded", cfg.Budget.PerRunTimeSeconds)
		} else {
			// No automatic retry: a failure is recorded and execution stops.
			res.Failures = append(res.Failures, execErr.Error())
		}
		return res, nil
	}

	if elapsed > timeLimitMillis(cfg.Budget.PerRunTimeSeconds) {
		res.Failures = append(res.Failures, "time limit exceeded")
		res.LimitFailure = true
		res.LimitFailureReason = fmt.Sprintf("per-run time limit of %ds exceeded", cfg.Budget.PerRunTimeSeconds)
		res.Completed = false
		return res, nil
	}

	if ec.TokensReported && ec.TokensUsed > cfg.Budget.PerRunTokenLimit {
		res.Failures = append(res.Failures, "token limit exceeded")
		res.LimitFailure = true
		res.LimitFailureReason = fmt.Sprintf("per-run token limit of %d exceeded (%d used)", cfg.Budget.PerRunTokenLimit, ec.TokensUsed)
		res.Completed = false
		return res, nil
	}

	if !ec.Completed {
		res.Failures = append(res.Failures, "arm did not complete")
	}
	if !ec.AcceptancePassed {
		res.Failures = append(res.Failures, "acceptance checks did not pass")
	}
	return res, nil
}

// filterHarnessPaths removes SOP's own runtime state and the staged task file
// from the arm's implementation footprint. Those paths are harness plumbing, not
// task output, and they must not make a clean arm look like a baseline edit.
func filterHarnessPaths(paths []string, taskFile string) []string {
	out := make([]string, 0, len(paths))
	task := filepath.Clean(taskFile)
	for _, p := range paths {
		clean := filepath.Clean(p)
		if clean == ".agent-sdlc" || strings.HasPrefix(clean, ".agent-sdlc"+string(filepath.Separator)) {
			continue
		}
		if taskFile != "" && clean == task {
			continue
		}
		out = append(out, p)
	}
	return out
}

// persistArmEvidence copies the arm's SOP run artifacts and captured output into
// the configured evidence root before the disposable worktree is removed. Missing
// run artifacts are not fabricated; the executor output is still preserved.
func persistArmEvidence(root, worktree, taskID string, arm Arm, res *ArmResult) error {
	dir := filepath.Join(root, string(arm))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if res.Output != "" {
		path := filepath.Join(dir, "executor-output.txt")
		if err := os.WriteFile(path, []byte(redactText(res.Output)), 0o644); err != nil {
			return err
		}
		res.EvidencePaths = append(res.EvidencePaths, path)
	}
	if len(res.InstructionsApplied) > 0 {
		path := filepath.Join(dir, "instructions-applied.txt")
		if err := os.WriteFile(path, []byte(strings.Join(res.InstructionsApplied, "\n")+"\n"), 0o644); err != nil {
			return err
		}
		res.EvidencePaths = append(res.EvidencePaths, path)
	}
	runSrc := filepath.Join(worktree, ".agent-sdlc", "runs", taskID)
	runDst := filepath.Join(dir, "runs", taskID)
	if _, err := os.Stat(runSrc); err == nil {
		if err := copyTree(runSrc, runDst); err != nil {
			return err
		}
		res.EvidencePaths = append(res.EvidencePaths, runDst)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// copyTree copies a directory or file without following symlinks. It is used only
// for bounded run artifacts inside the disposable worktree.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode().Perm())
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// fpShort returns the first 12 hex characters of the fingerprint digest, for
// worktree naming.
func fpShort(fp Fingerprint) string {
	if len(fp.Digest) >= 12 {
		return fp.Digest[:12]
	}
	return fp.Digest
}

// disallowed returns the first changed path outside allowed, or "".
func disallowed(changed []string, allowed []string) string {
	ok := map[string]bool{}
	for _, p := range allowed {
		ok[filepath.Clean(p)] = true
	}
	sorted := append([]string{}, changed...)
	sort.Strings(sorted)
	for _, p := range sorted {
		if !ok[filepath.Clean(p)] {
			return p
		}
	}
	return ""
}
