package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// CTRL011 regression: an invocation IMPLEMENTING approval functionality — which
// exhausted its bounded turn budget after productive discovery and made no
// repository change — is a bounded continuation, not APPROVAL_REQUIRED. The
// approval vocabulary is the subject of the work, not a request that the current
// execution be authorized. A genuine approval request (explicit authorization
// prose) still blocks.

// ctrl011CLIReason models the CTRL011 dogfood outcome: sufficient discovery, no
// mutation, deterministic remaining implementation, and approval-heavy prose.
const ctrl011CLIReason = "No repository change was made this invocation: the bounded turn budget was exhausted by required discovery " +
	"before any file could be written. Discovery for the Human Approval Controls task is complete. sopclient already exposes " +
	"Client.ApproveTask returning ErrOperationUnsupported and OpApproveTask is StatusUnsupported; SOP exposes no `sop approve` " +
	"operation, so the controller must not simulate approval. Remaining implementation is deterministic: add internal/sopclient/approval.go, " +
	"render an approve/decline control only for a NEEDS_HUMAN disposition or a WAITING_FOR_HUMAN (StageWaitingForHuman) stage, " +
	"implement a non-mutating decline button, document that approval is unsupported, and add tests for the approval gate. " +
	"A later bounded invocation should perform these edits."

// TestRunApprovalImplementationDiscoveryContinues is the CTRL011 dogfood
// regression: productive approval-feature discovery with no mutation is CONTINUE /
// INCOMPLETE_IMPLEMENTATION with LOW risk and AUTO_CONTINUE under BALANCED — never
// APPROVAL_REQUIRED / NEEDS_HUMAN.
func TestRunApprovalImplementationDiscoveryContinues(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl011CLIReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "classification: NEEDS_HUMAN") || strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
		t.Errorf("implementing approval controls must not require a human:\n%s", stdout)
	}
	for _, want := range []string{"run S001: CONTINUE", "classification: CONTINUE", "risk=LOW", "decision=AUTO_CONTINUE", "S001 CONTINUE (requeued)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}

	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.Continue || cls.Kind != failure.IncompleteImplementation {
		t.Errorf("recorded classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", cls)
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

// TestRunApprovalDiscoveryCheckpointReachesNextInvocation proves the checkpoint is
// preserved: the approval-feature discovery that stopped for a bounded continuation
// is handed to the next invocation, which then implements and completes.
func TestRunApprovalDiscoveryCheckpointReachesNextInvocation(t *testing.T) {
	dir := budgetGraph(t)

	first := &recordingAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl011CLIReason}}
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
	// The next invocation begins from the checkpoint instead of rediscovering: the
	// previous attempt's discovery summary reaches it.
	if input := second.inputs[0]; !strings.Contains(input, "Previous attempt") || !strings.Contains(input, "bounded turn budget") {
		t.Errorf("continuation context missing the checkpoint:\n%s", input)
	}
}

// ctrl012CLIReason models the CTRL012 dogfood outcome (see the classifier's
// ctrl012Reason): a partially implemented plan-reconciliation/approval feature.
const ctrl012CLIReason = "Incomplete: I rewrote internal/sopclient/changed_task.go and internal/sopclient/boundary.go (flipped OpAcceptChangedTask " +
	"to StatusSupported). However the blocking finding also requires web handlers/routes/templates surfacing the changed-task list and per-task approval " +
	"controls, and I did not update internal/sopclient/boundary_test.go, whose assertions still expect these ops to be unsupported. As left, " +
	"go test ./internal/sopclient/... fails and the acceptance criteria 'each changed task requires explicit approval' are still not wired into the UI. " +
	"More work is required to finish: update boundary_test.go, add per-task accept routes/handlers, enforce reconcile-before-mutation ordering, and add tests."

// ctrl012PlanDoc is a plan whose task TITLE and acceptance criteria use approval and
// reconciliation vocabulary: implementing the feature is ordinary work, not a
// request that the current run be authorized.
const ctrl012PlanDoc = "# Implementation Plan\n\n## Project\n\nSOP Controller\n\n## Summary\n\nAdd plan reconciliation controls.\n\n" +
	"## S001 — Add Plan Reconciliation and Approval Controls\n\nAdd per-task approval and accept-changed controls for changed executed tasks.\n\n" +
	"### Dependencies\n\nNone\n\n### Deliverables\n\n- changed-task presentation\n\n" +
	"### Acceptance Criteria\n\n- all changed executed tasks are reported before mutation\n- each changed task requires explicit approval\n"

// TestRunPlanReconciliationControlsContinues is the CTRL012 dogfood regression: a
// partially implemented reconciliation/approval feature — title, acceptance
// criteria, and summary saturated with approval vocabulary — is CONTINUE /
// INCOMPLETE_IMPLEMENTATION with LOW risk and AUTO_CONTINUE under BALANCED, never
// APPROVAL_REQUIRED / NEEDS_HUMAN.
func TestRunPlanReconciliationControlsContinues(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), ctrl012PlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl012CLIReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "classification: NEEDS_HUMAN") || strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
		t.Errorf("implementing reconciliation/approval controls must not require a human:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=AUTO_CONTINUE") {
		t.Errorf("stdout = %q, want AUTO_CONTINUE under BALANCED", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind == failure.ApprovalRequired || cls.Disposition == failure.NeedsHuman {
		t.Errorf("classification = %+v, want a non-human continuation", cls)
	}
}

// TestRunWaitingForHumanStageStaysHuman proves a task whose persisted run stage is
// WAITING_FOR_HUMAN stays at a human gate across invocations: the stage is a
// structured current-run signal, independent of the agent's prose.
func TestRunWaitingForHumanStageStaysHuman(t *testing.T) {
	dir := budgetGraph(t)
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SetStage(runpkg.WaitingForHuman); err != nil {
		t.Fatal(err)
	}

	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the implementation is incomplete"}}
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN for a WAITING_FOR_HUMAN run", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind != failure.ApprovalRequired || cls.Disposition != failure.NeedsHuman {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", cls)
	}
}

// TestRunActiveApprovalRequestStillBlocks is the separate regression: a CURRENT
// run with an active (unresolved) approval request is a genuine human boundary.
// APPROVAL_REQUIRED is STRUCTURED ONLY — it comes from SOP's own current-run
// lifecycle state, never from the agent's summary prose.
func TestRunActiveApprovalRequestStillBlocks(t *testing.T) {
	dir := budgetGraph(t)
	// SOP recorded an approval request for this task (as the lifecycle does when it
	// parks a task at a genuine gate). It is unresolved, so the task stays at the
	// gate.
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID:          "S001-gate",
		TaskID:      "S001",
		Kind:        domain.ApprovalNeedsHuman,
		Target:      "S001",
		Reason:      "conflicting requirements",
		RequestedAt: time.Now().UTC(),
		Status:      domain.ApprovalPending,
	}); err != nil {
		t.Fatal(err)
	}

	// Even an ordinary, incomplete-sounding summary stays at the gate: the prose is
	// irrelevant, the structured request is authoritative.
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the implementation is incomplete"}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN for an active approval request", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind != failure.ApprovalRequired || cls.Disposition != failure.NeedsHuman {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", cls)
	}
}

// TestRunApprovalProseAloneContinues proves authorization-sounding agent prose is
// NOT an approval gate: with no structured current-run approval state, the
// invocation is a bounded continuation.
func TestRunApprovalProseAloneContinues(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the required operation needs human authorization to proceed"}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "classification: NEEDS_HUMAN") || strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
		t.Errorf("authorization prose must not require a human:\n%s", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind == failure.ApprovalRequired || cls.Disposition == failure.NeedsHuman {
		t.Errorf("classification = %+v, want a non-human continuation (prose is not a gate)", cls)
	}
}
