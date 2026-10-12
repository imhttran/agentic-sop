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
// An operator-declared deterministic acceptance check (a SAFE argv vector, never a
// shell) is the verifiable half of acceptance: when a stage declares checks, every
// check MUST execute and pass for the verdict to be VERIFIED. A `go test` check is
// additionally required to have actually executed at least one test: a check that
// runs zero tests (no test files, `-run` matching nothing, or every test skipped)
// yields PARTIAL with explicit `no test executed` evidence, so a green-but-empty
// test run can never stand in for verification.
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
// per-criterion evidence, and the reasons. It is diagnostic record only; nothing
// reads it back to drive a decision.
type outcomeArtifact struct {
	Status   outcome.Status            `json:"status"`
	Criteria []outcome.CriterionResult `json:"criteria"`
	Reasons  []string                  `json:"reasons"`
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

	for i, d := range spec.Deliverables {
		p, ok := deliverablePath(d)
		if !ok {
			continue
		}
		path := p
		criteria = append(criteria, outcome.Criterion{
			ID:          fmt.Sprintf("deliverable-%d", i+1),
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
// argv, the process exit code, and a bounded output excerpt.
type checkRun struct {
	argv     []string
	exitCode int
	output   string
}

// runAcceptanceCheck executes an authorized acceptance check's argument vector,
// records its execution evidence, and applies the ACC-003 `go test` execution rule
// on top of it: a `go test` command that exits 0 but executed no test yields a
// "no test executed" error (PARTIAL) rather than a pass, so an empty-but-green test
// run cannot verify the objective. Non-go commands behave exactly as runAcceptanceArgv.
func runAcceptanceCheck(dir string, argv []string) (error, checkRun) {
	err, run := runAcceptanceArgv(dir, argv)
	if !isGoTestArgv(argv) {
		return err, run
	}
	// The command ran to completion with exit status 0; check that it actually
	// executed a test. A failing go test already returns a non-nil error above.
	if err != nil {
		return err, run
	}
	if !goTestExecuted(run.output) {
		return fmt.Errorf("acceptance check %q executed no test: no test executed", strings.Join(argv, " ")), run
	}
	return nil, run
}

// isGoTestArgv reports whether argv is a `go test` invocation (the command whose
// result is only meaningful when at least one test actually ran).
func isGoTestArgv(argv []string) bool {
	return len(argv) >= 2 && argv[0] == "go" && argv[1] == "test"
}

// goTestExecuted inspects the combined output of a `go test` command and reports
// whether at least one test actually executed. It is deliberately conservative:
// only the explicit no-execution signals are treated as "no test executed", so a
// run that produced a normal `ok` with no per-test lines is assumed to have run
// something. The signals are the ones `go test` emits when nothing ran:
//
//   - "no test files" — the package(s) declared no test files;
//   - "no tests to run" — the `-run` filter matched nothing;
//   - verbose output whose only test verdicts are skips (every test `t.Skip`).
func goTestExecuted(output string) bool {
	if strings.Contains(output, "no test files") {
		return false
	}
	if strings.Contains(output, "no tests to run") {
		return false
	}
	if strings.Contains(output, "--- SKIP:") && !strings.Contains(output, "--- PASS:") {
		return false
	}
	return true
}

// runAcceptanceArgv executes an authorized acceptance command's argument vector
// directly, without invoking a shell, so nothing beyond the validated argv can run.
// Exit status 0 means met; any other status or a launch failure means not met. It
// returns the error (nil when met) together with the observed execution evidence:
// the exact argv, the process exit code, and a bounded (<=400 byte) excerpt of the
// combined stdout/stderr.
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
	run.output = boundOutput(out.String())
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

// deliverablePath extracts a repository-relative path from a declared deliverable,
// or reports ok=false when the deliverable is prose rather than a path. It accepts
// a bare first token that looks like a path (contains a slash or a dot) and rejects
// absolute and parent-escaping forms.
func deliverablePath(s string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return "", false
	}
	p := strings.Trim(fields[0], "`\"'")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
		return "", false
	}
	if !strings.Contains(p, "/") && !strings.Contains(p, ".") {
		return "", false
	}
	return p, true
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
// renders nothing when the task declared no objective (res == nil).
func writeOutcomeSummary(w io.Writer, res *outcomeArtifact) {
	if res == nil {
		return
	}
	fmt.Fprintf(w, "outcome: %s\n", res.Status)
	for _, r := range res.Reasons {
		fmt.Fprintf(w, "  - %s\n", r)
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
