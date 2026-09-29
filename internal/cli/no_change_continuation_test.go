package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// No-change continuation lifecycle tests (CTRL006 regression).
//
// A productive invocation that exhausts its bounded budget on repository
// discovery without mutating is a resumable continuation, not a human decision.
// SOP reports CONTINUE and requeues the task through the existing bounded
// recovery path; only a genuine approval/safety/ambiguity boundary is
// NEEDS_HUMAN. These tests pin that end to end through the CLI.

// noChangeFixReason mirrors the real CTRL006 FIX outcome: the invocation spent
// its budget on valid discovery and made no repository change.
const noChangeFixReason = "the Ollama agent FIX made no repository change after 24 iterations " +
	"(model=deepseek-v4.1-flash:cloud, tool_calls=22, termination=no_change, last_action=\"search_files\"); a retry may succeed"

// fixNoChangeAgent completes IMPLEMENT but reports a no-change, needs_human FIX
// outcome, so a run reaches the fix loop exactly as the CTRL006 dogfood run did.
type fixNoChangeAgent struct{ fixReason string }

func (a fixNoChangeAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		return agent.Response{Content: "done", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}, nil
	case agent.Fix:
		return agent.Response{Content: "done", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: a.fixReason}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// failingValidationGraph seeds a one-task graph whose configured build fails, so
// the lifecycle enters the bounded fix loop instead of finishing after IMPLEMENT.
func failingValidationGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n")
	return dir
}

// TestRunFixNoChangeContinuationReportsContinue is requirement 1 for the FIX
// path: productive discovery plus no mutation is CONTINUE, never NEEDS_HUMAN, and
// the run is not parked at the human boundary.
func TestRunFixNoChangeContinuationReportsContinue(t *testing.T) {
	dir := failingValidationGraph(t)
	a := fixNoChangeAgent{fixReason: noChangeFixReason}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, NEEDS_HUMAN must not be reported for a resumable no-change run", stdout)
	}
	for _, want := range []string{
		"run S001: CONTINUE",
		"classification: CONTINUE",
		"S001 CONTINUE (requeued)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}

	// The run stage is a non-pass, non-human stage: SOP will continue on its own.
	if stage, ok := runpkg.Load(dir, "S001"); !ok || stage == runpkg.WaitingForHuman {
		t.Errorf("run stage = %q (ok=%t), want a non-human continuation stage", stage, ok)
	}

	// The task was requeued (bounded), not blocked and not parked for a human.
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

// TestRunContinuationCheckpointReachesNextInvocation is requirement 2: the
// continuation reuses the recorded checkpoint, the next invocation mutates, and
// the task then completes.
func TestRunContinuationCheckpointReachesNextInvocation(t *testing.T) {
	dir := budgetGraph(t)

	first := &recordingAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noChangeFixReason}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", first, "run"); code != exitError {
		t.Fatalf("first run: code=%d, want %d", code, exitError)
	}

	second := &recordingAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", second, "run")
	if code != exitOK {
		t.Fatalf("second run: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "S001 LOCAL_DONE") {
		t.Errorf("stdout = %q, want S001 LOCAL_DONE", stdout)
	}
	if len(second.inputs) == 0 {
		t.Fatal("no implement request recorded on the continued attempt")
	}
	// The checkpoint (the previous no-change outcome) reaches the next invocation,
	// so it resumes instead of repeating discovery from zero.
	if input := second.inputs[0]; !strings.Contains(input, "Previous attempt") || !strings.Contains(input, "made no repository change") {
		t.Errorf("continuation context missing the checkpoint:\n%s", input)
	}
}

// TestRunTransientProviderFailureStillRetries is requirement 10: a provider
// failure is still a bounded RETRY, and it is not turned into a human boundary.
func TestRunTransientProviderFailureStillRetries(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "ollama returned an empty response"}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "classification: RETRY") || !strings.Contains(stdout, "S001 RETRY (requeued)") {
		t.Errorf("stdout = %q, want a bounded RETRY requeue", stdout)
	}
}

// TestRunCompletedMutatingImplementationStillPasses is requirement 8: an ordinary
// successful implementation is unchanged by the continuation handling.
func TestRunCompletedMutatingImplementationStillPasses(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/a.go b/a.go\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "S001 LOCAL_DONE") {
		t.Errorf("stdout = %q, want S001 LOCAL_DONE", stdout)
	}
	if strings.Contains(stdout, "CONTINUE") {
		t.Errorf("stdout = %q, a completed implementation must not be reported as a continuation", stdout)
	}
}
