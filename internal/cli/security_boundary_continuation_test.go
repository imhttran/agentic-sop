package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/store"
)

// Security-boundary classification and continuation-budget tests (CTRL008
// regression).
//
// A productive invocation that exhausted its bounded budget without mutating is a
// resumable continuation, even when its explanation mentions security-relevant
// words (secret, token, prompt, "raw"). Only an actual security DECISION — an
// action that changes a trust or privilege boundary — is a human boundary. These
// tests pin that end to end through the CLI, and pin that CONTINUE consumes its own
// bounded continuation budget rather than the retry budget.

// ctrl008RunReason is the real CTRL008 outcome: discovery completed, no repository
// change was made, the remaining work is fully scoped, and the agent's prose
// mentions prompt/secret/token fields that must not be rendered.
const ctrl008RunReason = "No repository edits were performed in this invocation; discovery completed but tools ended before any write. " +
	"Remaining, fully-scoped work: add regression tests asserting absent data never renders PASS and no raw prompt/secret text " +
	"is shown for a fixture task carrying prompt/token fields. A later bounded invocation should perform these edits."

// TestRunCTRL008ProductiveDiscoveryContinues pins the CTRL008 fix: a no-mutation,
// security-worded discovery run is CONTINUE / INCOMPLETE_IMPLEMENTATION at LOW risk
// with an AUTO_CONTINUE decision — never SECURITY_BOUNDARY or NEEDS_HUMAN.
func TestRunCTRL008ProductiveDiscoveryContinues(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Render activity detail", Status: domain.PLANNED, MaxAttempts: 3})
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl008RunReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want a non-pass continuation", code)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") || strings.Contains(stdout, "SECURITY_BOUNDARY") {
		t.Fatalf("stdout = %q, want no human boundary for productive incomplete work", stdout)
	}
	for _, want := range []string{
		"run S001: CONTINUE",
		"classification: CONTINUE",
		"risk=LOW",
		"decision=AUTO_CONTINUE",
		"S001 CONTINUE (requeued)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}

	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.Continue || cls.Kind != failure.IncompleteImplementation {
		t.Errorf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", cls)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED (CONTINUE requeues rather than blocking)", got.Status)
	}
}

// TestRunContinuationUsesContinuationBudgetNotRetryBudget proves requirement 11:
// CONTINUE is bounded by max_continuations and never consumes the retry budget.
// max_attempts is 1, so consuming the retry budget would exhaust the task
// immediately; two continuations still succeed, and the attempt count is untouched.
func TestRunContinuationUsesContinuationBudgetNotRetryBudget(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\nautonomy:\n  max_continuations: 2\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.PLANNED, MaxAttempts: 1})
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl008RunReason}}

	for i := 0; i < 2; i++ {
		code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
		if code != exitError {
			t.Fatalf("run %d: code=%d, want %d", i, code, exitError)
		}
		if !strings.Contains(stdout, "S001 CONTINUE (requeued)") {
			t.Fatalf("run %d: stdout = %q, want a continuation, not a retry-budget block", i, stdout)
		}
	}

	// A third continuation exceeds max_continuations: a terminal stuck state, never
	// a human boundary.
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("final run: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "continuation budget exhausted") {
		t.Errorf("stdout = %q, want the bounded continuation budget message", stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, continuation exhaustion must not be reported as NEEDS_HUMAN", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.BLOCKED || got.BlockedReason != domain.CONTINUATION_EXHAUSTED {
		t.Errorf("task = %s/%s, want BLOCKED/CONTINUATION_EXHAUSTED", got.Status, got.BlockedReason)
	}
	if got.Attempt != 0 {
		t.Errorf("attempt = %d, want 0 (a CONTINUE must not consume the retry budget)", got.Attempt)
	}
}

// TestRunSecurityDecisionRequiresHuman proves a genuine security DECISION — an
// action that changes a trust boundary — still stops at the human boundary, even
// though the same prose vocabulary in an ordinary implementation reason does not.
func TestRunSecurityDecisionRequiresHuman(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "the required change would weaken authentication and expose local command execution to remote unauthenticated clients",
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN for a security decision", stdout)
	}
	if !strings.Contains(stdout, "risk=HIGH") {
		t.Errorf("stdout = %q, want HIGH risk for a security decision", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind != failure.SecurityBoundary || cls.Disposition != failure.NeedsHuman {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/SECURITY_BOUNDARY", cls)
	}
}

// TestRunIdenticalNoProgressContinuationIsBounded proves requirement 15: repeated
// identical no-progress continuations reach the continuation ceiling and stop as a
// bounded terminal state, without being mislabeled NEEDS_HUMAN.
func TestRunIdenticalNoProgressContinuationIsBounded(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\nautonomy:\n  max_continuations: 2\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl008RunReason}}

	for i := 0; i < 2; i++ {
		if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
			t.Fatalf("run %d: code=%d, want %d", i, code, exitError)
		}
	}
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("final run: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "continuation budget exhausted") || strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want a bounded continuation exhaustion, not a human boundary", stdout)
	}
}
