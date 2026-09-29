package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
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

// TestRunExplicitApprovalRequestStillBlocks is the separate regression: prose that
// actually ASKS for authorization to proceed is still a human boundary, proving the
// classifier distinguishes an approval SUBJECT from an approval REQUEST.
func TestRunExplicitApprovalRequestStillBlocks(t *testing.T) {
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the required operation needs human authorization to proceed"}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN for an explicit authorization request", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind != failure.ApprovalRequired || cls.Disposition != failure.NeedsHuman {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", cls)
	}
}
