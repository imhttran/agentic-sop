package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/durablefs"
	"github.com/imhttran/agentic-sop/internal/handoff"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// This file integrates internal/outcome into the governed lifecycle: after the
// quality gate, SOP derives an objective-level acceptance verdict from the
// AUTHORIZED TASK SPECIFICATION — never from a model-generated completion claim.
// The verdict (VERIFIED / PARTIAL / HOLD) is recorded as an artifact and reported,
// so a task's PASS/LOCAL_DONE is never equated with verified objective completion.
//
// The operator-enabled verdict gate (SOP-HARD-01, internal/config.Lifecycle) may
// additionally gate the terminal LOCAL_DONE transition on that verdict: with the
// gate ON, a PARTIAL from an unmet REQUIRED deliverable/check does not yield
// LOCAL_DONE unless a valid, designated-approver waiver exists. The gate is OFF by
// default, so an omitted flag leaves the lifecycle unchanged (the gate adds no
// lifecycle/waiver evidence on the flag-off path).
//
// An operator-declared deterministic acceptance check (a SAFE argv vector, never a
// shell) is the verifiable half of acceptance: when a stage declares checks, every
// check MUST execute and pass for the verdict to be VERIFIED. A `go test` check is
// additionally required to have actually executed at least one PASSING test: it is
// judged from structured `go test -json` events from a single execution. It is met
// only when the process exit status is 0 AND at least one test-level pass event
// (Action=="pass" with a non-empty test name, TestMain excluded) occurred. An
// all-skipped run, a `-run` filter that matched nothing, a package with no test
// files, and a TestMain that returns without calling m.Run are therefore never
// VERIFIED; packages with no test files do not count against an otherwise passing
// run. This replaces the earlier output-text heuristic and yields PARTIAL with
// explicit `no test executed` evidence, so a green-but-empty test run can never
// stand in for verification.
//
// Authorization is form-aware: beyond the generic SAFE classification, flag forms
// that redirect or inject tooling (-exec/-toolexec/-vettool/-o/-overlay/-modfile),
// `go fmt` (which rewrites files), and `git -c` / `--config-env` config injection
// are rejected BEFORE execution, as an unmet boundary that HOLDs the verdict. A
// rejected command therefore never runs and leaves no evidence of execution.
//
// Free-text acceptance criteria have no deterministic determinant here
// (per-criterion operator-owned verifier bindings live in the unmerged harden-001
// work). When the stage declares deterministic checks, such criteria are recorded
// as informational: they are shown alongside the checks but do not themselves hold
// the verdict, because the checks are the verifiable evidence. When no checks exist
// they keep today's semantics — recorded but unverifiable — so a PASS is never
// equated with verified objective completion.

// outcomeArtifact is the persisted objective-level verdict: the status, the
// per-criterion evidence, and the reasons, plus the resolved local lifecycle
// disposition. It is diagnostic record only; the lifecycle field records what the
// operator-enabled verdict gate resolved (LOCAL_DONE / NEEDS_HUMAN), and nothing here
// writes SOP state.
//
// Lifecycle and WaiverDenied are set ONLY when the operator-enabled verdict gate is
// ON, so the flag-off artifact carries neither field (the omitempty contract) and
// the flag-off output stays exactly as it was before the gate existed.
type outcomeArtifact struct {
	Status    outcome.Status            `json:"status"`
	Criteria  []outcome.CriterionResult `json:"criteria"`
	Reasons   []string                  `json:"reasons"`
	Lifecycle outcome.Lifecycle         `json:"lifecycle,omitempty"`
	// WaiverDenied records that the verdict gate blocked LOCAL_DONE because the
	// PARTIAL had an unmet required deliverable/check and no valid waiver was
	// supplied. It is the observable denial: a missing or invalid waiver is never a
	// silent downgrade. It is set only for the waivable PARTIAL case, never for a
	// HOLD (which is non-waivable, so there is no waiver to deny).
	WaiverDenied bool `json:"waiver_denied,omitempty"`
	// Validation is the quality-gate result recorded apart from the acceptance
	// criteria (SOP-003). Set only for a stage with an acceptance mapping.
	Validation *outcome.ValidationResult `json:"validation,omitempty"`
	// Compatibility is the actionable message for a legacy stage that declares no
	// acceptance mapping, so its verdict is never a silent VERIFIED.
	Compatibility string `json:"compatibility,omitempty"`
	// Mappings is the stage's acceptance mapping, recorded so a manual acceptance
	// decision can be validated against it.
	Mappings []outcome.CriterionMapping `json:"acceptance_mapping,omitempty"`
	// Waived records that a valid designated-approver waiver covers every unmet
	// REQUIRED criterion, so mapped acceptance evidence does not block advancement.
	// It is independent of the operator verdict gate: SOP-003 requires mapped evidence
	// to gate advancement even when the legacy gate is off.
	Waived bool `json:"waived,omitempty"`
	// logs maps each failed check's run-relative log artifact path to its redacted
	// full output; writeOutcomeArtifact persists them beside outcome.json.
	logs map[string]string
}

// blocksAdvancement reports whether a mapped stage's verdict lacks required
// evidence, so the task must stop at the human boundary instead of completing.
func (a *outcomeArtifact) blocksAdvancement() bool {
	if a == nil {
		return false
	}
	if a.Waived {
		return false
	}
	return outcome.Result{Status: a.Status, Validation: a.Validation, Compatibility: a.Compatibility}.BlocksAdvancement()
}

// deriveOutcome computes the objective-level verdict from the task specification.
// It always returns a verdict for a non-nil specification — a prompt-driven
// implementation run therefore records evidence-backed outcome, never a silent
// no-op. VERIFIED requires at least one executed, passing acceptance check: a
// deliverable-only or free-text-only specification whose quality gate passed is
// held at PARTIAL, so a PASS is never equated with verified objective completion.
// It returns nil only when there is no specification.
func deriveOutcome(spec *taskfile.Spec, dir string, gate quality.Result) *outcomeArtifact {
	return deriveOutcomeWith(spec, dir, gate, false)
}

// deriveOutcomeWith is deriveOutcome with an explicit acceptance-check Cargo
// permission. When it is OFF (the default) a Cargo acceptance check is never
// authorized; when it is ON the Cargo classifier admits only the supported forms
// inside dir. It is independent of the agent-tool permission.
func deriveOutcomeWith(spec *taskfile.Spec, dir string, gate quality.Result, cargoEnabled bool) *outcomeArtifact {
	if spec == nil {
		return nil
	}
	if len(spec.AcceptanceMapping) > 0 {
		return deriveMappedOutcome(spec, dir, gate, cargoEnabled)
	}
	var criteria []outcome.Criterion

	passed := gate.Decision == quality.Pass
	criteria = append(criteria, outcome.Criterion{
		ID:          "deterministic-checks",
		Required:    true,
		Description: "the configured build/test/lint checks and the review passed",
		Check: func() error {
			if !passed {
				return errors.New("deterministic checks did not pass")
			}
			return nil
		},
	})

	criteria = append(criteria, deliverableCriteria(spec, dir)...)

	// Operator-authorized deterministic acceptance checks: each is validated into an
	// exact argument vector and executed directly (never through a shell),
	// independently of the model completion claim. A command the policy does not
	// classify SAFE — or whose form is one of the rejected dangerous flag forms,
	// `go fmt`, or git config injection — is a boundary that holds the outcome and is
	// never executed. Evidence for an executed check (argv, exit code, bounded
	// output) is captured and attached to the criterion result.
	type checkEvidence struct {
		id  string
		run *checkRun
	}
	var evidence []checkEvidence

	for i, command := range spec.AcceptanceChecks {
		cmd := command
		id := fmt.Sprintf("check-%d", i+1)
		argv, aerr := authorizeAcceptanceCommandWith(cmd, cargoEnabled, dir)
		if aerr != nil {
			err := aerr
			criteria = append(criteria, outcome.Criterion{
				ID:          id,
				Required:    true,
				Boundary:    true,
				Description: "unauthorized acceptance check: " + cmd,
				Command:     cmd,
				Check:       func() error { return err },
			})
			continue
		}
		args := argv
		run := &checkRun{}
		criteria = append(criteria, outcome.Criterion{
			ID:          id,
			Required:    true,
			Description: "acceptance check: " + cmd,
			Command:     cmd,
			Check: func() error {
				err, executed := runAcceptanceCheck(dir, args)
				*run = executed
				run.failed = err != nil
				return err
			},
		})
		evidence = append(evidence, checkEvidence{id: id, run: run})
	}

	// Free-text acceptance criteria have no deterministic determinant on this branch.
	// When the stage declares deterministic checks, the criteria are recorded as
	// informational — shown alongside the checks but non-required, because the checks
	// are the verifiable evidence. When no checks exist they keep today's semantics:
	// recorded but unverifiable, holding the verdict at PARTIAL rather than letting a
	// PASS imply verified completion.
	hasChecks := len(spec.AcceptanceChecks) > 0
	for i, c := range spec.AcceptanceCriteria {
		if hasChecks {
			criteria = append(criteria, outcome.Criterion{
				ID:          fmt.Sprintf("criterion-%d", i+1),
				Required:    false,
				Description: c,
				// A nil Check counts as unverifiable; Required=false keeps it from
				// holding the verdict, and writeOutcomeReport labels such a
				// non-required, unmet criterion "informational".
			})
			continue
		}
		criteria = append(criteria, outcome.Criterion{
			ID:          fmt.Sprintf("criterion-%d", i+1),
			Required:    true,
			Description: c,
		})
	}

	// VERIFIED requires at least one executed, passing acceptance check. Without a
	// declared check, a deliverable or free-text criterion cannot alone verify the
	// objective: record an unverifiable required criterion so a passing gate with
	// only deliverables/criteria is PARTIAL, never VERIFIED. When nothing is declared
	// at all, the same fallback applies.
	if len(spec.AcceptanceChecks) == 0 {
		criteria = append(criteria, outcome.Criterion{
			ID:          "objective-acceptance",
			Required:    true,
			Description: "the specification declares no executed, passing acceptance check",
		})
	}

	res := outcome.Verify(criteria)
	// Verify has executed every authorized check; attach each run's argv, exit code,
	// and bounded output to its criterion. Rejected checks never ran and have no entry.
	logs := map[string]string{}
	for _, e := range evidence {
		res.PropagateEvidence(e.id, checkResultOf(e.id, *e.run, logs))
	}
	art := &outcomeArtifact{Status: res.Status, Criteria: res.Criteria, Reasons: res.Reasons, logs: logs}
	// A legacy stage declares no acceptance mapping, so its criteria cannot be
	// verified: record the actionable compatibility result instead of a silent
	// VERIFIED. No required criterion becomes unmet, so the lifecycle is unchanged.
	if len(spec.AcceptanceCriteria) > 0 {
		ids := make([]string, len(spec.AcceptanceCriteria))
		for i := range ids {
			ids[i] = fmt.Sprintf("criterion-%d", i+1)
		}
		art.Compatibility = outcome.LegacyCompatibility(ids)
		if art.Status == outcome.Verified {
			art.Status = outcome.Partial
			art.Reasons = []string{art.Compatibility}
		}
	}
	return art
}

// deliverableCriteria returns exactly one required presence criterion per declared
// deliverable LINE that names at least one repository path. The criterion's
// Description is the original line, so the requirement it carries is preserved. Its
// Check stats every path the line names and passes only when all of them exist;
// otherwise it fails with an error that names every missing path and no present one.
// A prose-only line contributes no criterion. Emitting one criterion per line (not
// per path) keeps a multi-path bullet from producing duplicate criteria.
func deliverableCriteria(spec *taskfile.Spec, dir string) []outcome.Criterion {
	var criteria []outcome.Criterion
	for _, d := range spec.Deliverables {
		paths := deliverablePaths(d)
		if len(paths) == 0 {
			continue
		}
		line := d
		linePaths := append([]string(nil), paths...)
		criteria = append(criteria, outcome.Criterion{
			ID:          fmt.Sprintf("deliverable-%d", len(criteria)+1),
			Required:    true,
			Description: line,
			Check: func() error {
				return missingPathError(linePaths, dir)
			},
		})
	}
	return criteria
}

// missingPathError stats every path and returns nil only when all exist. Otherwise
// it returns an error naming each missing path (in declaration order) and no present
// path, so a failure never misreports a satisfied requirement.
func missingPathError(paths []string, dir string) error {
	missing := missingPathsIn(paths, dir)
	if len(missing) == 0 {
		return nil
	}
	if len(missing) == 1 {
		return fmt.Errorf("deliverable %q not present", missing[0])
	}
	quoted := make([]string, len(missing))
	for i, p := range missing {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return fmt.Errorf("deliverable paths not present: %s", strings.Join(quoted, ", "))
}

// missingPathsIn returns the subset of paths that do not exist under dir, in order.
func missingPathsIn(paths []string, dir string) []string {
	var missing []string
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(presencePath(p)))); err != nil {
			missing = append(missing, p)
		}
	}
	return missing
}

// presencePath strips a trailing line-range or line-number suffix (`:N` or `:N-M`)
// from a declared path, so a form such as internal/planner/markdown.go:164-166 is
// checked against the file internal/planner/markdown.go.
func presencePath(p string) string {
	i := strings.LastIndexByte(p, ':')
	if i <= 0 || i == len(p)-1 {
		return p
	}
	suffix := p[i+1:]
	if isLineRange(suffix) {
		return p[:i]
	}
	return p
}

// isLineRange reports whether s is a line number or a line span, such as 164 or
// 164-166.
func isLineRange(s string) bool {
	parts := strings.SplitN(s, "-", 2)
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// missingDeliverables returns the declared deliverable paths that do not exist
// in dir, in declaration order. It applies the same path rule as
// deliverableCriteria, so the lifecycle and the outcome verdict agree on what a
// deliverable is.
func missingDeliverables(spec *taskfile.Spec, dir string) []string {
	var missing []string
	// De-duplicate across lines so a path named on two lines is reported once.
	seen := map[string]bool{}
	for _, d := range spec.Deliverables {
		for _, p := range deliverablePaths(d) {
			if seen[p] {
				continue
			}
			seen[p] = true
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(presencePath(p)))); err != nil {
				missing = append(missing, p)
			}
		}
	}
	return missing
}

// deriveMappedOutcome is the SOP-003 verdict for a stage that declares an
// acceptance mapping. The quality gate is recorded as separate validation; each
// acceptance criterion is met only through its mapped check or an accepting manual
// decision; every declared check still runs and must pass; an unauthorized check
// remains a HOLD boundary.
func deriveMappedOutcome(spec *taskfile.Spec, dir string, gate quality.Result, cargoEnabled bool) *outcomeArtifact {
	criteria := deliverableCriteria(spec, dir)
	mappings := specCriterionMappings(spec)
	mappedChecks := make(map[string]bool, len(mappings))
	for _, m := range mappings {
		if m.Kind == outcome.EvidenceCheck && m.CheckRef != "" {
			mappedChecks[m.CheckRef] = true
		}
	}
	var checks []outcome.CheckResult
	logs := map[string]string{}
	for i, cmd := range spec.AcceptanceChecks {
		id := fmt.Sprintf("check-%d", i+1)
		argv, aerr := authorizeAcceptanceCommandWith(cmd, cargoEnabled, dir)
		if aerr != nil {
			criteria = append(criteria, outcome.Criterion{ID: id, Required: true, Boundary: true, Description: "unauthorized acceptance check: " + cmd, Command: cmd, Check: func() error { return aerr }})
			continue
		}
		err, run := runAcceptanceCheck(dir, argv)
		run.failed = err != nil
		cr := checkResultOf(id, run, logs)
		cr.Command, cr.Executed, cr.Passed, cr.Evidence = cmd, true, err == nil, "met"
		if err != nil {
			cr.Evidence = err.Error()
		}
		checks = append(checks, cr)
		// A check that is mapped to a criterion is EVIDENCE for that criterion; the
		// criterion is the requirement. Adding the raw check as a second required
		// criterion would make a criterion-scoped waiver insufficient (the operator
		// would have to waive both the criterion and its evidence). An unmapped check
		// remains a required, independently gated acceptance check.
		if !mappedChecks[id] {
			criteria = append(criteria, outcome.Criterion{ID: id, Required: true, Description: "acceptance check: " + cmd, Command: cmd, Check: func() error { return err }})
		}
	}
	for i, c := range spec.AcceptanceCriteria {
		criteria = append(criteria, outcome.Criterion{ID: fmt.Sprintf("criterion-%d", i+1), Required: true, Description: c})
	}
	decisions, derr := readAcceptanceDecisions(dir, spec.ID)
	if derr != nil {
		// Acceptance decisions are evidence. A corrupted or unreadable audit must not
		// silently become "no decision": record the error on the verdict so a manual
		// criterion cannot be treated as satisfied and the operator sees why.
		decisions = nil
	}
	// A supported recovery preserves an accepted decision across an approved attempt
	// advance. An unreadable recovery audit fails closed: an earlier decision is not
	// silently accepted without the recorded transition that authorizes it.
	recoveries, rerr := readAcceptanceRecoveries(dir, spec.ID)
	if rerr != nil {
		recoveries = nil
	}
	validation := outcome.ValidationResult{Passed: gate.Decision == quality.Pass, Reasons: gate.Reasons}
	res := outcome.VerifyAcceptanceWithRecoveries(validation, criteria, mappings, checks, decisions, recoveries)
	if derr != nil {
		res.Status = outcome.Partial
		res.Reasons = append(res.Reasons, "acceptance decisions could not be read: "+derr.Error())
	}
	if rerr != nil {
		res.Status = outcome.Partial
		res.Reasons = append(res.Reasons, acceptanceRecoveryReadReason(rerr))
	}
	for _, c := range checks {
		res.PropagateEvidence(c.Ref, c)
	}
	return &outcomeArtifact{Status: res.Status, Criteria: res.Criteria, Reasons: res.Reasons, Validation: res.Validation, Mappings: mappings, logs: logs}
}

// checkResultOf projects an executed check onto outcome evidence. A failed check
// also gets a full-log artifact path under the run directory, registered in logs
// with its (already redacted) complete output.
func checkResultOf(id string, run checkRun, logs map[string]string) outcome.CheckResult {
	c := outcome.CheckResult{Ref: id, Argv: run.argv, ExitCode: run.exitCode, Output: run.output, WorkDir: run.dir}
	if run.failed {
		c.LogArtifact = "checks/" + id + ".log"
		logs[c.LogArtifact] = run.full
	}
	return c
}

// specCriterionMappings projects the spec's '- criterion N: check M' /
// '- criterion N: manual: <evidence>' lines onto outcome mappings keyed by the
// criterion-N / check-M ids deriveMappedOutcome uses.
func specCriterionMappings(spec *taskfile.Spec) []outcome.CriterionMapping {
	var out []outcome.CriterionMapping
	for _, line := range spec.AcceptanceMapping {
		m := planner.ParseAcceptanceMapping(line)
		cm := outcome.CriterionMapping{CriterionID: fmt.Sprintf("criterion-%d", m.Criterion), Required: true, Kind: outcome.EvidenceCheck, Attempt: spec.Attempt}
		if m.Criterion >= 1 && m.Criterion <= len(spec.AcceptanceCriteria) {
			cm.Description = spec.AcceptanceCriteria[m.Criterion-1]
		}
		switch {
		case m.Manual != "":
			cm.Kind, cm.ManualEvidence = outcome.EvidenceManual, m.Manual
		case m.Check != 0:
			cm.CheckRef = fmt.Sprintf("check-%d", m.Check)
		}
		out = append(out, cm)
	}
	return out
}

// waiverForTask returns the designated-approver waiver supplied for this run, or nil
// when none was supplied.
//
// The repository provides no designated-approver identity source, so the only
// supply path today is an explicit operator input; absent that, there is no valid
// waiver and the gate fails closed to NEEDS_HUMAN (the denial is observable). This
// is the explicit, documented placeholder contract: it intentionally returns nil
// rather than inventing an approver identity, so the gate is strictly fail-closed
// on the live path. The valid-waiver branch is exercised directly at the
// outcome-package level (outcome.ValidateWaiver / outcome.ResolveLifecycle). Any
// future operator-supplied waiver is owned by the CLI/config layer; this stub is
// where it would be read.
func waiverForTask(spec *taskfile.Spec) *outcome.Waiver {
	_ = spec
	return nil
}

// applyLifecycleGate records the resolved local lifecycle disposition on the outcome
// artifact using the operator-enabled verdict gate (SOP-HARD-01).
//
// With the gate OFF (the default) it records NOTHING: the lifecycle field stays empty
// and WaiverDenied stays false, so the flag-off artifact and output are byte-identical
// to before the gate existed. With the gate ON, it records the resolved disposition:
// a PARTIAL from an unmet required deliverable/check resolves to NEEDS_HUMAN (and sets
// WaiverDenied, the observable denial) unless a valid, designated-approver waiver
// covers every unmet required criterion; an informational-only PARTIAL and VERIFIED
// resolve to LOCAL_DONE; a HOLD resolves to NEEDS_HUMAN without setting WaiverDenied
// (a HOLD is an authority boundary, not a waiver the operator failed to supply).
func applyLifecycleGate(cfg config.Config, spec *taskfile.Spec, res *outcomeArtifact) {
	applyLifecycleGateWithWaiver(cfg, spec, res, waiverForTask(spec))
}

// applyLifecycleGateWithWaiver is applyLifecycleGate with an explicit waiver: the
// caller supplies the designated-approver waiver (nil when none was recorded). The
// run path supplies the recorded waiver (recordedWaiverForTask); the package-local
// tests call the 3-argument form, which passes the no-waiver path.
func applyLifecycleGateWithWaiver(cfg config.Config, spec *taskfile.Spec, res *outcomeArtifact, waiver *outcome.Waiver) {
	if res == nil {
		return
	}
	r := outcome.Result{Status: res.Status, Criteria: res.Criteria, Reasons: res.Reasons}
	// A valid waiver clears advancement for a mapped stage even when the legacy
	// verdict gate is OFF. This is the explicit, traceable acceptance decision the
	// plan requires; it never applies to a HOLD, which is non-waivable.
	if waiver != nil && outcome.ValidateWaiver(r, *waiver) == nil {
		res.Waived = true
	}
	// Flag OFF (the default): the lifecycle is unchanged — ResolveLifecycle returns
	// LOCAL_DONE for every verdict — so the recorded disposition is LOCAL_DONE and no
	// waiver denial is possible. Flag ON: the pinned lifecycle table applies.
	res.Lifecycle = outcome.ResolveLifecycle(cfg.VerdictGateEnabled(), r, waiver)
	// The waiver denial is observable only for the waivable PARTIAL case: a HOLD is
	// non-waivable, so there is no missing waiver to report (its NEEDS_HUMAN is the
	// authority boundary itself, not a denied waiver).
	if cfg.VerdictGateEnabled() && res.Status == outcome.Partial && r.HasUnmetRequired() && res.Lifecycle != outcome.LocalDone {
		res.WaiverDenied = true
	}
}

// deriveGatedOutcome derives the objective-level verdict and, when the
// operator-enabled verdict gate is ON, resolves and records the local lifecycle
// disposition on it. It is the single production call site for the gate: every
// lifecycle path that records an outcome verdict routes through here, so the flag-on
// acceptance behavior is exercised in a real run rather than only in unit tests.
func deriveGatedOutcome(cfg config.Config, spec *taskfile.Spec, dir string, gate quality.Result) *outcomeArtifact {
	res := deriveOutcomeWith(spec, dir, gate, cfg.CargoAcceptanceCheckEnabled())
	// A recorded, designated-approver waiver is the only input that can resolve a
	// gated PARTIAL-with-unmet-required to LOCAL_DONE. It is read from the task's
	// append-only audit; absent one, the gate fails closed (waiver == nil).
	var waiver *outcome.Waiver
	if spec != nil {
		waiver = recordedWaiverForTask(dir, spec.ID)
	}
	applyLifecycleGateWithWaiver(cfg, spec, res, waiver)
	return res
}

// rejectedGoFlagForms are the flag names whose presence in any spelling (-flag
// value, -flag=value, --flag value, --flag=value) redirects or injects tooling and
// must never be authorized as an acceptance check.
var rejectedGoFlagForms = []string{"exec", "toolexec", "vettool", "o", "overlay", "modfile"}

// authorizeAcceptanceCommand tokenizes an operator-declared acceptance command and
// validates it against the command policy and the hardened form rules, returning the
// exact argument vector that will execute. It reuses toolharness.SplitCommand, which
// rejects shell syntax (command chaining, pipelines, redirection, command
// substitution, environment expansion, and globs) and unterminated quotes, so
// authorization validates the same structure that runs — never a shell. A
// classification error, a non-SAFE class, or a rejected dangerous form is returned
// before execution, so an unauthorized command never runs.
func authorizeAcceptanceCommand(command string) ([]string, error) {
	return authorizeAcceptanceCommandWith(command, false, "")
}

// authorizeAcceptanceCommandWith is authorizeAcceptanceCommand with an explicit
// acceptance-check Cargo permission and repository root. When the permission is
// OFF (the default) a Cargo command is never authorized; when it is ON the Cargo
// classifier admits only the supported forms inside root. It is independent of
// the agent-tool permission.
func authorizeAcceptanceCommandWith(command string, cargoEnabled bool, root string) ([]string, error) {
	argv, err := toolharness.SplitCommand(command)
	if err != nil {
		return nil, err
	}
	if err := rejectDangerousForm(argv); err != nil {
		return nil, err
	}
	if len(argv) > 0 && filepath.Base(argv[0]) == "cargo" {
		class, cerr := commandpolicy.CargoClassify(argv, cargoEnabled, root)
		if cerr != nil {
			return nil, fmt.Errorf("acceptance check %q is not authorized: %v", command, cerr)
		}
		if class != commandpolicy.Safe {
			return nil, fmt.Errorf("acceptance check %q is not authorized (%s)", command, class)
		}
		return argv, nil
	}
	class, err := commandpolicy.Classify(argv)
	if err != nil {
		return nil, err
	}
	if class != commandpolicy.Safe {
		return nil, fmt.Errorf("acceptance check %q is not authorized (%s)", command, class)
	}
	return argv, nil
}

// rejectDangerousForm rejects acceptance commands whose form is not permitted even
// though it may otherwise classify SAFE:
//
//   - a go/build/vet flag that redirects or injects tooling (-exec, -toolexec,
//     -vettool, -o, -overlay, -modfile), in every spelling;
//   - `go fmt` (and `go fmt ...`), which rewrites files;
//   - `git -c ...`, `git -ccore...=...` and `git --config-env ...` config injection.
func rejectDangerousForm(argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	// git config injection: `-c`, `-cK=V`, `--config-env` (value or =value) are
	// rejected; a plain read-only git command stays authorized.
	if argv[0] == "git" {
		for _, a := range argv[1:] {
			if a == "-c" || strings.HasPrefix(a, "-c") || a == "--config-env" || strings.HasPrefix(a, "--config-env=") {
				return fmt.Errorf("acceptance check %q injects git configuration", strings.Join(argv, " "))
			}
		}
	}
	// `go fmt` rewrites files.
	if argv[0] == "go" && len(argv) >= 2 && argv[1] == "fmt" {
		return fmt.Errorf("acceptance check %q rewrites files (go fmt)", strings.Join(argv, " "))
	}
	// go tooling flags that redirect or inject tooling, in every spelling.
	for _, a := range argv[1:] {
		name := flagName(a)
		if name == "" {
			continue
		}
		for _, bad := range rejectedGoFlagForms {
			if name == bad {
				return fmt.Errorf("acceptance check %q uses unauthorized flag -%s", strings.Join(argv, " "), bad)
			}
		}
	}
	return nil
}

// flagName extracts the flag name from an argument in any of the -f, -f=v, --f and
// --f=v spellings, returning "" for a non-flag argument. The leading dashes are
// stripped and any "=value" suffix removed (so -o=x yields "o" and -o yields "o").
func flagName(arg string) string {
	if !strings.HasPrefix(arg, "-") {
		return ""
	}
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == "" {
		return ""
	}
	if i := strings.IndexByte(trimmed, '='); i >= 0 {
		trimmed = trimmed[:i]
	}
	return trimmed
}

// checkRun is the structured evidence from one executed acceptance check: the exact
// argv, the working directory, the process exit code, a bounded output excerpt, and
// the untruncated output. Both outputs are secret-redacted.
type checkRun struct {
	argv     []string
	dir      string
	exitCode int
	output   string // bounded (<=400 byte) excerpt, for recorded evidence
	full     string // untruncated combined output, for the go test decision and log artifact
	failed   bool   // set by the caller once the check's verdict is known
}

// runAcceptanceCheck executes an authorized acceptance check's argument vector
// exactly once, records its execution evidence, and applies the structured `go test`
// execution rule on top of it. A `go test` check is run with `-json` (added when the
// operator did not already request it) and judged from the resulting event stream: it
// is met only when the process exits 0 AND at least one test-level pass event
// occurred, so an all-skipped, zero-test, unmatched `-run`, or TestMain-without-m.Run
// run yields a "no test executed" error (PARTIAL) rather than a pass. Non-go-test
// commands are judged solely by exit status, exactly as runAcceptanceArgv does.
//
// The command is executed once: the same run's exit status and captured output back
// both the pass/fail decision and the recorded evidence, so no second invocation is
// made for events.
func runAcceptanceCheck(dir string, argv []string) (error, checkRun) {
	if !isGoTestArgv(argv) {
		return runAcceptanceArgv(dir, argv)
	}
	// Force structured output for go test checks (never duplicating an operator's
	// -json, never altering -count), then execute that argv exactly once.
	argv = ensureGoTestJSON(argv)
	err, run := runAcceptanceArgv(dir, argv)
	if err != nil {
		// A non-zero exit already means not met; report it with the executed argv.
		return err, run
	}
	// Judge the full event stream, not the bounded evidence excerpt, so a pass event
	// beyond the 400-byte excerpt still counts.
	met, reason := goTestJSONMet(run.exitCode, run.full)
	if !met {
		return fmt.Errorf("acceptance check %q executed no passing test: %s", strings.Join(argv, " "), reason), run
	}
	return nil, run
}

// isGoTestArgv reports whether argv is a `go test` invocation (the command whose
// result is only meaningful when at least one test actually ran).
func isGoTestArgv(argv []string) bool {
	return len(argv) >= 2 && argv[0] == "go" && argv[1] == "test"
}

// runAcceptanceArgv executes an authorized acceptance command's argument vector
// directly, without invoking a shell, so nothing beyond the validated argv can run.
// Exit status 0 means met; any other status or a launch failure means not met. It
// returns the error (nil when met) together with the observed execution evidence:
// the exact argv, the process exit code, and a bounded (<=400 byte) excerpt of the
// combined stdout/stderr. The untruncated combined output is also retained so the
// `go test -json` decision can inspect the full event stream.
func runAcceptanceArgv(dir string, argv []string) (error, checkRun) {
	run := checkRun{argv: append([]string(nil), argv...), dir: dir}
	if len(argv) == 0 {
		return errors.New("acceptance check has an empty command"), run
	}
	cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	// Redact before bounding, so neither the excerpt nor the log carries a secret.
	run.full = handoff.Redact(out.String(), os.Environ())
	run.output = boundOutput(run.full)
	if runErr == nil {
		run.exitCode = 0
		return nil, run
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		run.exitCode = exitErr.ExitCode()
		return fmt.Errorf("acceptance check %q failed (exit %d): %s", strings.Join(argv, " "), run.exitCode, run.output), run
	}
	return fmt.Errorf("acceptance check %q could not run: %v", strings.Join(argv, " "), runErr), run
}

// boundOutput returns a bounded excerpt of a check's combined output: trimmed, and
// truncated to at most 400 bytes so the recorded evidence stays small.
func boundOutput(s string) string {
	detail := strings.TrimSpace(s)
	if len(detail) > 400 {
		detail = detail[:400]
	}
	return detail
}

// deliverablePaths extracts every repository-relative path a declared deliverable
// names, in order and de-duplicated. A deliverable bullet may list several paths (for
// example "`internal/a.go`, `internal/b.go` — prose"); each becomes its own
// deliverable so it is verified individually. Backtick-quoted tokens are preferred
// when present; otherwise each comma/semicolon-separated piece contributes its first
// path-like field. Prose-only deliverables contribute no path.
func deliverablePaths(s string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] || !looksLikeDeliverablePath(p) {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	spans := backtickSpans(s)
	for _, span := range spans {
		add(span)
	}
	// Backticks are the authoritative quoting: when a line quotes anything, only
	// the quoted tokens may name a path. This keeps descriptive prose outside the
	// backticks (a command, a symbol, or a sentence) from contributing a path.
	if len(spans) > 0 {
		return out
	}
	// No backtick spans: fall back to each comma/semicolon-separated piece's first
	// field, with surrounding backticks/quotes/punctuation trimmed. The bare-token
	// rule below still rejects prose words, commands, symbols, and URLs.
	for _, piece := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		fields := strings.Fields(strings.TrimSpace(piece))
		if len(fields) == 0 {
			continue
		}
		add(strings.Trim(fields[0], "`\"'.,"))
	}
	return out
}

// backtickSpans returns the contents of each `backtick`-delimited span in s, in order.
func backtickSpans(s string) []string {
	var spans []string
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			return spans
		}
		rest := s[i+1:]
		j := strings.IndexByte(rest, '`')
		if j < 0 {
			return spans
		}
		spans = append(spans, rest[:j])
		s = rest[j+1:]
	}
}

// looksLikeDeliverablePath reports whether p names a repository-relative path rather
// than a command, flag, URL, or descriptive prose. Recognition is syntactic: it never
// calls os.Stat, so an absent path is still recognized and a missing-file check still
// fails correctly.
//
// Recognized forms:
//   - a path with a separator: internal/provider/provider.go, docs/reference/
//   - a line-range suffix on such a path: internal/planner/markdown.go:164-166
//   - a bare root-level file name: Makefile, LICENSE, go.mod
//   - a bare file name with an extension: setup.cfg
//
// Rejected forms:
//   - empty, absolute, parent-escaping, and whitespace-bearing tokens (so
//     "go test ./..." is never a path)
//   - a URL with a scheme or "://" (https://example.com/x)
//   - a descriptive code symbol: a dotted identifier with no separator
//     (ModelInfo.Locality, errors.New, fmt.Errorf)
//   - a bare token with no extension and no separator (a command or prose word)
func looksLikeDeliverablePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsAny(p, " \t") {
		return false
	}
	// A URL is never a repository path.
	if strings.Contains(p, "://") || strings.Contains(p, "@") {
		return false
	}
	if i := strings.LastIndexByte(p, ':'); i >= 0 {
		if !isLineRange(p[i+1:]) {
			return false
		}
	}
	if strings.Contains(p, "/") {
		// A separator marks a path. Reject a scheme-less code selector that still
		// carries a dotted tail without a file extension only when it has no file
		// part; a path segment may legitimately contain a dot, so a separator wins.
		return true
	}
	// No separator: accept a bare root-level file only when it carries a file
	// extension (go.mod, setup.cfg). A dotted identifier (ModelInfo.Locality) has a
	// dotted tail with no separator and is a symbol, not a root-level file; reject
	// it by requiring the dotted name to contain no uppercase run that marks a
	// code selector. A bare name with no dot (Makefile, LICENSE) is accepted when it
	// is a single clean token.
	if strings.Contains(p, ".") {
		if looksLikeCodeSelector(p) {
			return false
		}
		return true
	}
	return isCleanBareName(p)
}

// looksLikeCodeSelector reports whether a dotted, separator-free token reads as a
// code reference such as ModelInfo.Locality or errors.New: a capitalized segment is
// a type selector, and a lowercase method form such as errors.New or fmt.Errorf is a
// package-qualified symbol. A file name keeps its dotted name lowercase with a known
// file extension, so the extension decides.
func looksLikeCodeSelector(p string) bool {
	i := strings.LastIndexByte(p, '.')
	if i <= 0 || i == len(p)-1 {
		return true
	}
	tail := p[i+1:]
	for _, r := range tail {
		if r < 'a' || r > 'z' {
			// An upper-case or non-lowercase tail is a symbol segment (Locality), not
			// a lowercase file extension.
			return true
		}
	}
	// A lowercase dotted name is a file only with a known extension; a symbol such as
	// errors.New has a lowercase tail that is not a file extension.
	return !isKnownFileExtension(tail)
}

// isKnownFileExtension reports whether ext (without a leading dot) is a file
// extension that a bare, separator-free repository file may carry.
func isKnownFileExtension(ext string) bool {
	switch ext {
	case "go", "md", "markdown", "txt", "json", "jsonl", "yaml", "yml", "toml", "cfg", "conf", "ini",
		"mod", "sum", "work", "sh", "bash", "zsh", "ps1", "bat", "cmd", "mk", "am", "make",
		"py", "rb", "php", "rs", "js", "mjs", "cjs", "ts", "tsx", "jsx", "vue", "svelte",
		"java", "kt", "c", "h", "cc", "cpp", "hpp", "swift", "html", "htm", "css", "scss",
		"sass", "less", "sql", "proto", "xml", "csv", "tsv", "patch", "diff", "lock", "bazel",
		"nix", "build", "env", "example", "service", "plist":
		return true
	}
	return false
}

// isCleanBareName reports whether p is a single bare word (no dot, no punctuation)
// that may name a conventional root-level file such as Makefile or LICENSE.
// Recognition is syntactic: it never calls os.Stat, so an absent root-level file is
// still recognized and its missing-file check still fails. It accepts only the
// conventional root-level file-name set: a general CamelCase word is a code symbol
// (ModelInfo), not a path, and prose words are never paths.
func isCleanBareName(p string) bool {
	if p == "" {
		return false
	}
	for _, r := range p {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return knownRootFiles[p]
}

// knownRootFiles lists conventional root-level file names that carry no extension
// and no mixed case, so the bare-name rule accepts them syntactically. Recognition
// does not depend on the file existing.
var knownRootFiles = map[string]bool{
	"Makefile":      true,
	"GNUmakefile":   true,
	"LICENSE":       true,
	"COPYING":       true,
	"NOTICE":        true,
	"CHANGELOG":     true,
	"AUTHORS":       true,
	"README":        true,
	"Dockerfile":    true,
	"Containerfile": true,
	"Vagrantfile":   true,
	"Justfile":      true,
	"Procfile":      true,
	"Gemfile":       true,
	"Rakefile":      true,
	"Taskfile":      true,
	"Brewfile":      true,
	"Podfile":       true,
	"Fastfile":      true,
	"Appfile":       true,
	"Snapfile":      true,
	"Matchfile":     true,
	"Pluginfile":    true,
	"Dangerfile":    true,
	"Guardfile":     true,
	"Capfile":       true,
	"Thorfile":      true,
	"Berksfile":     true,
	"Cheffile":      true,
	"Puppetfile":    true,
	"Cartfile":      true,
	"WORKSPACE":     true,
	"BUILD":         true,
}

// writeOutcomeArtifact persists the verdict beside the other run artifacts, with
// each failed check's full-log artifact. The logs live in the run directory and
// share its lifecycle; no separate retention rule applies. A write failure is
// returned, never swallowed: evidence the operator was told about must exist on
// disk, and a failed check without its log artifact is incomplete evidence.
func writeOutcomeArtifact(rn *runpkg.Run, res *outcomeArtifact) error {
	if res == nil {
		return nil
	}
	for path, content := range res.logs {
		if err := durablefs.WriteFile(filepath.Join(rn.Dir(), filepath.FromSlash(path)), []byte(content)); err != nil {
			return fmt.Errorf("write check evidence %s: %w", path, err)
		}
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("encode outcome artifact: %w", err)
	}
	if err := durablefs.WriteFile(filepath.Join(rn.Dir(), "outcome.json"), append(data, '\n')); err != nil {
		return fmt.Errorf("write outcome artifact: %w", err)
	}
	return nil
}

// writeOutcomeSummary prints the objective-level verdict beneath the gate summary,
// so an operator sees VERIFIED/PARTIAL/HOLD separately from PASS/LOCAL_DONE. It
// renders nothing when the task declared no objective (res == nil). It also surfaces
// the gate's resolution when the operator-enabled verdict gate is ON: the resolved
// lifecycle, and — for the waivable PARTIAL case — an explicit denial line, so the
// denial is observable rather than a silent downgrade. With the gate OFF the
// lifecycle/waiver fields are empty and nothing extra is rendered, so the flag-off
// output is unchanged.
func writeOutcomeSummary(w io.Writer, res *outcomeArtifact) {
	if res == nil {
		return
	}
	fmt.Fprintf(w, "outcome: %s\n", res.Status)
	if res.Validation != nil {
		fmt.Fprintf(w, "  validation: %s\n", validationState(res.Validation))
	}
	for _, r := range res.Reasons {
		fmt.Fprintf(w, "  - %s\n", r)
	}
	if res.Lifecycle != "" {
		fmt.Fprintf(w, "lifecycle: %s\n", res.Lifecycle)
	}
	if res.WaiverDenied {
		fmt.Fprintln(w, "  waiver: denied — a required deliverable/check is unmet and no valid designated-approver waiver exists")
	}
}

// validationState renders the separately recorded validation result.
func validationState(v *outcome.ValidationResult) string {
	if v.Passed {
		return "passed"
	}
	if len(v.Reasons) == 0 {
		return "failed"
	}
	return "failed: " + strings.Join(v.Reasons, "; ")
}

// writeOutcomeReport renders the verdict in the run report, including the per-check
// execution evidence when present. A rejected boundary check is rendered as not met
// with no execution evidence.
func writeOutcomeReport(b *strings.Builder, res *outcomeArtifact) {
	if res == nil {
		return
	}
	b.WriteString("\n## Outcome\n\n")
	fmt.Fprintf(b, "- Status: `%s`\n", res.Status)
	if res.Validation != nil {
		fmt.Fprintf(b, "- Validation: %s\n", validationState(res.Validation))
	}
	if res.Compatibility != "" {
		fmt.Fprintf(b, "- Compatibility: %s\n", res.Compatibility)
	}
	if res.Lifecycle != "" {
		fmt.Fprintf(b, "- Lifecycle: `%s`\n", res.Lifecycle)
	}
	if res.Waived {
		b.WriteString("- Waived: a valid designated-approver waiver covers every unmet required criterion\n")
	}
	if res.WaiverDenied {
		b.WriteString("- Waiver: denied — a required deliverable/check is unmet and no valid designated-approver waiver exists\n")
	}
	for _, cr := range res.Criteria {
		state := "met"
		switch {
		case !cr.Met && !cr.Required:
			// A non-required criterion with no deterministic check is informational:
			// the stage's declared checks are the verifiable evidence.
			state = "informational"
		case !cr.Met:
			state = "not met"
		}
		// A criterion backed by an acceptance check names the exact command, so the
		// report shows the operator which command was (or was not) met.
		desc := cr.Description
		if cr.Command != "" {
			desc = cr.Command
		}
		fmt.Fprintf(b, "- `%s` %s — %s (%s)\n", cr.ID, state, desc, cr.Evidence)
		// Surface executed-check evidence where present. A rejected boundary check
		// carries no argv and shows no execution evidence.
		if len(cr.Argv) > 0 {
			fmt.Fprintf(b, "  - argv: `%s`\n", strings.Join(cr.Argv, " "))
			if cr.WorkDir != "" {
				fmt.Fprintf(b, "  - workdir: `%s`\n", cr.WorkDir)
			}
			fmt.Fprintf(b, "  - exit: %d\n", cr.ExitCode)
			if cr.Output != "" {
				fmt.Fprintf(b, "  - output: %s\n", cr.Output)
			}
			if cr.LogArtifact != "" {
				fmt.Fprintf(b, "  - log: `%s`\n", cr.LogArtifact)
			}
		}
	}
	for _, r := range res.Reasons {
		fmt.Fprintf(b, "- %s\n", r)
	}
}
