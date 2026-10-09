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
// The deterministic determinants available on this branch are SOP's own checks (the
// gate's build/test/lint/review verdict) and the existence of each declared
// deliverable that names a repository-relative path. A free-text acceptance
// criterion has no deterministic determinant here (per-criterion operator-owned
// verifier bindings live in the unmerged harden-001 work), so it is recorded but
// counts as unverifiable and cannot by itself establish VERIFIED.

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
// no-op. When the specification declares no deterministically verifiable
// acceptance requirement, the deterministic checks are the only evidence and the
// verdict is PARTIAL rather than VERIFIED, so a PASS is never equated with verified
// objective completion. It returns nil only when there is no specification.
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
	// classify SAFE — including any command that uses shell syntax (chaining,
	// pipelines, redirection, substitution, expansion) — is a boundary that holds the
	// outcome and is never executed.
	for i, command := range spec.AcceptanceChecks {
		cmd := command
		argv, aerr := authorizeAcceptanceCommand(cmd)
		if aerr != nil {
			err := aerr
			criteria = append(criteria, outcome.Criterion{
				ID:          fmt.Sprintf("check-%d", i+1),
				Required:    true,
				Boundary:    true,
				Description: "unauthorized acceptance check: " + cmd,
				Command:     cmd,
				Check:       func() error { return err },
			})
			continue
		}
		args := argv
		criteria = append(criteria, outcome.Criterion{
			ID:          fmt.Sprintf("check-%d", i+1),
			Required:    true,
			Description: "acceptance check: " + cmd,
			Command:     cmd,
			Check:       func() error { return runAcceptanceArgv(dir, args) },
		})
	}

	// Free-text acceptance criteria have no deterministic determinant on this branch;
	// recorded but unverifiable, they hold the verdict at PARTIAL rather than letting
	// a PASS imply verified completion.
	for i, c := range spec.AcceptanceCriteria {
		criteria = append(criteria, outcome.Criterion{
			ID:          fmt.Sprintf("criterion-%d", i+1),
			Required:    true,
			Description: c,
		})
	}

	// With no objective-bearing requirement (no deliverable, no acceptance
	// criterion), the deterministic checks are the only evidence, which cannot
	// verify the objective: record an unverifiable required criterion so the verdict
	// is PARTIAL, never VERIFIED.
	if len(spec.Deliverables) == 0 && len(spec.AcceptanceCriteria) == 0 && len(spec.AcceptanceChecks) == 0 {
		criteria = append(criteria, outcome.Criterion{
			ID:          "objective-acceptance",
			Required:    true,
			Description: "the specification declares no deterministically verifiable acceptance requirement",
		})
	}

	res := outcome.Verify(criteria)
	return &outcomeArtifact{Status: res.Status, Criteria: res.Criteria, Reasons: res.Reasons}
}

// authorizeAcceptanceCommand tokenizes an operator-declared acceptance command and
// validates it against the existing command policy, returning the exact argument
// vector that will execute. It reuses toolharness.SplitCommand, which rejects shell
// syntax (command chaining, pipelines, redirection, command substitution,
// environment expansion, and globs) and unterminated quotes, so authorization
// validates the same structure that runs — never a shell. A classification error or
// a non-SAFE class is rejected before execution, so an unauthorized command never
// runs.
func authorizeAcceptanceCommand(command string) ([]string, error) {
	argv, err := toolharness.SplitCommand(command)
	if err != nil {
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

// runAcceptanceArgv executes an authorized acceptance command's argument vector
// directly, without invoking a shell, so nothing beyond the validated argv can run.
// Exit status 0 means met; any other status or a launch failure means not met, and
// the observed result is recorded as evidence.
func runAcceptanceArgv(dir string, argv []string) error {
	if len(argv) == 0 {
		return errors.New("acceptance check has an empty command")
	}
	cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}
	detail := strings.TrimSpace(out.String())
	if len(detail) > 400 {
		detail = detail[:400]
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return fmt.Errorf("acceptance check %q failed (exit %d): %s", strings.Join(argv, " "), exitErr.ExitCode(), detail)
	}
	return fmt.Errorf("acceptance check %q could not run: %v", strings.Join(argv, " "), runErr)
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

// writeOutcomeReport renders the verdict in the run report.
func writeOutcomeReport(b *strings.Builder, res *outcomeArtifact) {
	if res == nil {
		return
	}
	b.WriteString("\n## Outcome\n\n")
	fmt.Fprintf(b, "- Status: `%s`\n", res.Status)
	for _, cr := range res.Criteria {
		state := "met"
		if !cr.Met {
			state = "not met"
		}
		fmt.Fprintf(b, "- `%s` %s — %s\n", cr.ID, state, cr.Evidence)
	}
	for _, r := range res.Reasons {
		fmt.Fprintf(b, "- %s\n", r)
	}
}
