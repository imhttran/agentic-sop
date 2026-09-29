package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/store"
)

// budgetExhaustedReason mirrors the real CTRL002 outcome: the agent exhausted its
// tool budget during discovery, made no repository change, and described the
// credential/secret work it never reached. The incidental human-boundary words
// must not turn a resumable run into a human decision.
const budgetExhaustedReason = "No repository change could be performed in this invocation: " +
	"the tool budget was exhausted by required discovery (reading internal/sopclient/activity.go, " +
	"run.go, service.go, types.go, store.go, tests, web handlers/server, cmd, templates, README) " +
	"before any write tool could be executed. The fixes (structural credential/secret/JWT/URL-credential " +
	"redaction) have not been applied to the working tree. [no repository change was made; retrying]"

// budgetGraph seeds a one-task graph plan and returns its directory.
func budgetGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	return dir
}

// TestRunBudgetExhaustionWithHumanKeywordsContinues proves test 1: discovery that
// exhausts its budget before any mutation is CONTINUE (requeued), even when the
// agent's explanation mentions words the classifier would otherwise read as a
// human boundary.
func TestRunBudgetExhaustionWithHumanKeywordsContinues(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "CONTINUE") {
		t.Errorf("stdout = %q, want the CONTINUE disposition", stdout)
	}
	if !strings.Contains(stdout, "run S001: CONTINUE") {
		t.Errorf("stdout = %q, want the run reported as CONTINUE, not a human boundary", stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN fully absent for a resumable run", stdout)
	}
	if !strings.Contains(stdout, "S001 CONTINUE (requeued)") {
		t.Errorf("stdout = %q, want S001 requeued as CONTINUE", stdout)
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

	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.Continue || cls.Kind != failure.IncompleteImplementation {
		t.Errorf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", cls)
	}
}

// TestRunBudgetContinuationEventuallyCompletes proves test 2: a run that continues
// after budget exhaustion can complete normally on a later invocation.
func TestRunBudgetContinuationEventuallyCompletes(t *testing.T) {
	dir := budgetGraph(t)

	// Attempt 1: budget exhausted before any mutation -> CONTINUE (requeued).
	a1 := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a1, "run"); code != exitError {
		t.Fatalf("first run: code=%d, want %d", code, exitError)
	}

	// Attempt 2: the agent implements the task and claims the change.
	a2 := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a2, "run")
	if code != exitOK {
		t.Fatalf("second run: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "S001 LOCAL_DONE") {
		t.Errorf("stdout = %q, want S001 LOCAL_DONE", stdout)
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
	if got.Status != domain.LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE", got.Status)
	}
}

// TestRunBudgetContinuationIsBounded proves test 3: repeated continuation reaches
// the configured continuation ceiling (max_continuations, default 4) and terminates
// cleanly as a bounded automation failure rather than looping.
func TestRunBudgetContinuationIsBounded(t *testing.T) {
	dir := budgetGraph(t)

	// Each attempt reports a distinct budget-exhausted reason so it makes
	// "progress"; the bound is the continuation budget (max_continuations, default
	// 4), separate from and not consuming the retry budget.
	for i := 0; i < 4; i++ {
		a := outcomeAgent{outcome: &agent.Outcome{
			Status: agent.OutcomeNeedsHuman,
			Reason: budgetExhaustedReason + fmt.Sprintf(" (attempt %d)", i),
		}}
		if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
			t.Fatalf("run %d: code=%d, want %d", i, code, exitError)
		}
	}

	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason + " (final)"}}
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("final run: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "continuation budget exhausted") {
		t.Errorf("stdout = %q, want the bounded continuation budget message", stdout)
	}
	// Repeated no-mutation continuation ends in a terminal stuck state, not a
	// human boundary.
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, a bounded continuation exhaustion must not be reported as NEEDS_HUMAN", stdout)
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
	if got.Status != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED after the continuation budget is spent", got.Status)
	}
}

// TestRunBudgetContinuationCarriesPreviousContext proves the checkpoint: the next
// invocation after a budget-exhaustion CONTINUE receives the previous attempt's
// outcome as context, so it resumes instead of repeating the whole discovery.
func TestRunBudgetContinuationCarriesPreviousContext(t *testing.T) {
	dir := budgetGraph(t)

	first := &recordingAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", first, "run"); code != exitError {
		t.Fatalf("first run: code=%d, want %d", code, exitError)
	}

	second := &recordingAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}
	_, _, _ = runInjectedCLI(t, dir, "diff\n", second, "run")
	if len(second.inputs) == 0 {
		t.Fatal("no implement request recorded on the continued attempt")
	}
	input := second.inputs[0]
	if !strings.Contains(input, "Previous attempt") || !strings.Contains(input, "tool budget was exhausted") {
		t.Errorf("continuation context missing the previous attempt:\n%s", input)
	}
}

// TestRunGenuineAmbiguityStillNeedsHuman proves test 4: a genuine unresolved
// decision still stops at the human boundary (the fix does not weaken it).
func TestRunGenuineAmbiguityStillNeedsHuman(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "the requirements conflict and no authoritative contract resolves them",
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.NeedsHuman {
		t.Errorf("classification = %+v, want NEEDS_HUMAN", cls)
	}
}

// gitCommitAll stages everything and commits it, for building a repo whose
// intentional state is tracked.
func gitCommitAll(t *testing.T, dir, message string) {
	t.Helper()
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=SOP Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=SOP Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("add", "-A")
	run("commit", "-q", "-m", message)
}

// TestRunContinuationPreservesPreexistingDirtyTree proves test 5: a CONTINUE
// requeue preserves pre-existing user work and never runs a destructive Git
// operation.
func TestRunContinuationPreservesPreexistingDirtyTree(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	assertRecoveryPreserved(t, dir, "S001")
}

// TestRunContinuationPreservesUserOwnedConfig proves test 6: an intentional,
// user-owned change to a tracked SOP config file is preserved across a CONTINUE
// requeue, never attributed to the task and never reverted.
func TestRunContinuationPreservesUserOwnedConfig(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)

	// Commit a baseline config, then make the intentional user edit on top of it.
	writeConfig(t, dir, "project:\n  name: x\nagent:\n  provider: command\nvalidation:\n  build:\n    - \"true\"\n")
	gitCommitAll(t, dir, "track sop config")
	const userConfig = "project:\n  name: x\nagent:\n  provider: ollama\n  model: deepseek-v4.1-flash:cloud\nvalidation:\n  build:\n    - \"true\"\n"
	writeConfig(t, dir, userConfig)

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)

	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}

	if got, err := os.ReadFile(filepath.Join(dir, stateDirName, "config.yaml")); err != nil || string(got) != userConfig {
		t.Errorf("config.yaml = %q, %v; the user-owned change was not preserved", got, err)
	}
	assertRecoveryPreserved(t, dir, "S001")
}

// TestRunDoesNotRevertUnrelatedChangeOnAgentRequest proves test 7: when the agent
// asks SOP to revert an unrelated, pre-existing user change, SOP neither performs
// the reversion nor discards the change.
func TestRunDoesNotRevertUnrelatedChangeOnAgentRequest(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	const userConfig = "project:\n  name: x\nagent:\n  provider: ollama\nvalidation:\n  build:\n    - \"true\"\n"
	writeConfig(t, dir, userConfig)
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)

	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "please revert the unrelated .agent-sdlc/config.yaml provider change before continuing",
	}}
	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}

	if got, err := os.ReadFile(filepath.Join(dir, stateDirName, "config.yaml")); err != nil || string(got) != userConfig {
		t.Errorf("config.yaml = %q, %v; SOP reverted a user-owned change", got, err)
	}
	assertRecoveryPreserved(t, dir, "S001")
}
