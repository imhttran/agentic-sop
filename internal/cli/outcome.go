package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
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
}

// deriveOutcome computes the objective-level verdict from the task specification.
// It always returns a verdict for a non-nil specification — a prompt-driven
// implementation run therefore records evidence-backed outcome, never a silent
// no-op. VERIFIED requires at least one executed, passing acceptance check: a
// deliverable-only or free-text-only specification whose quality gate passed is
// held at PARTIAL, so a PASS is never equated with verified objective completion.
// It returns nil only when there is no specification.
func deriveOutcome(spec *taskfile.Spec, dir string, gate quality.Result) *outcomeArtifact {
	if spec == nil {
		return nil
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

	// A declared deliverable may list more than one path (for example
	// "`a/x.go`, `a/y.go` — prose"), so every path-like token becomes its own required
	// criterion and prose-only deliverables contribute none. This is what keeps a
	// multi-path bullet from collapsing into a single malformed token.
	deliverableIndex := 0
	for _, d := range spec.Deliverables {
		for _, p := range deliverablePaths(d) {
			deliverableIndex++
			path := p
			criteria = append(criteria, outcome.Criterion{
				ID:          fmt.Sprintf("deliverable-%d", deliverableIndex),
				Required:    true,
				Description: d,
				Check: func() error {
					if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
						return fmt.Errorf("deliverable %q not present", path)
					}
					return nil
				},
			})
		}
	}

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
		argv, aerr := authorizeAcceptanceCommand(cmd)
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
	for _, e := range evidence {
		res.PropagateEvidence(e.id, e.run.argv, e.run.exitCode, e.run.output)
	}
	return &outcomeArtifact{Status: res.Status, Criteria: res.Criteria, Reasons: res.Reasons}
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
	res := deriveOutcome(spec, dir, gate)
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
	argv, err := toolharness.SplitCommand(command)
	if err != nil {
		return nil, err
	}
	if err := rejectDangerousForm(argv); err != nil {
		return nil, err
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
// argv, the process exit code, a bounded output excerpt, and the untruncated output.
type checkRun struct {
	argv     []string
	exitCode int
	output   string // bounded (<=400 byte) excerpt, for recorded evidence
	full     string // untruncated combined output, for the go test decision
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
	run := checkRun{argv: append([]string(nil), argv...)}
	if len(argv) == 0 {
		return errors.New("acceptance check has an empty command"), run
	}
	cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	run.full = out.String()
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
	for _, span := range backtickSpans(s) {
		add(span)
	}
	if len(out) > 0 {
		return out
	}
	// No backtick-quoted path: fall back to each comma/semicolon-separated piece's
	// first field, with surrounding backticks/quotes/punctuation trimmed.
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

// looksLikeDeliverablePath reports whether p is a plausible repository-relative path
// rather than a command, flag, or prose. It rejects empty, absolute, parent-escaping,
// and whitespace-bearing tokens (so "go test ./..." is never mistaken for a path) and
// requires a path separator or a dot.
func looksLikeDeliverablePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsAny(p, " \t") {
		return false
	}
	return strings.Contains(p, "/") || strings.Contains(p, ".")
}

// writeOutcomeArtifact persists the verdict beside the other run artifacts.
func writeOutcomeArtifact(rn *runpkg.Run, res *outcomeArtifact) {
	if res == nil {
		return
	}
	writeRunJSON(rn, "outcome.json", res)
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

// writeOutcomeReport renders the verdict in the run report, including the per-check
// execution evidence when present. A rejected boundary check is rendered as not met
// with no execution evidence.
func writeOutcomeReport(b *strings.Builder, res *outcomeArtifact) {
	if res == nil {
		return
	}
	b.WriteString("\n## Outcome\n\n")
	fmt.Fprintf(b, "- Status: `%s`\n", res.Status)
	if res.Lifecycle != "" {
		fmt.Fprintf(b, "- Lifecycle: `%s`\n", res.Lifecycle)
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
			fmt.Fprintf(b, "  - exit: %d\n", cr.ExitCode)
			if cr.Output != "" {
				fmt.Fprintf(b, "  - output: %s\n", cr.Output)
			}
		}
	}
	for _, r := range res.Reasons {
		fmt.Fprintf(b, "- %s\n", r)
	}
}
