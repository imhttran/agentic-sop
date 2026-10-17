package ollamaagent

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// correctionEvent is the trace event the harness records when it offers the single
// correction, so a test can count how many were requested.
const correctionEvent = "ALREADY_SATISFIED evidence incomplete; one correction requested"

// claimNoCommand maps every criterion to files, but never runs the required validation.
// It is correctable (VALIDATION_NOT_EXECUTED) exactly once.
const claimNoCommand = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"Add returns the sum","paths":["./add.go","add_test.go"]}]}}`

// denyCommand is refused by the command policy (git commit is not read-only), so it must
// stay refused even inside the correction phase.
const denyCommand = `{"tool":"run_command","args":{"command":"git commit -m x"}}`

// loopCorrectionFixture builds a satisfaction fixture whose discovery window closes after a
// single credited turn, so every later read-only interaction is only possible through the
// correction phase.
func loopCorrectionFixture(t *testing.T) (string, agent.Request) {
	t.Helper()
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	req.DiscoveryIterations = 1
	return dir, req
}

// TestLoopCorrectionPermitsReadAfterWindow proves the correction phase is real in the
// orchestration loop: after the discovery window has closed and a claim has been rejected, a
// read-only command the run would otherwise refuse is permitted, and the corrected claim is
// then accepted. It exercises the actual harness, loop, and tool policy.
func TestLoopCorrectionPermitsReadAfterWindow(t *testing.T) {
	dir, req := loopCorrectionFixture(t)
	// turn 1 read (credited), turn 2 read (post-window, uncredited, stale turn),
	// turn 3 claim missing the required command -> correction offered,
	// turn 4 the required command (post-window: only the phase authorizes it),
	// turn 5 the corrected claim -> accepted.
	_, srv := newFakeOllama(t, readAdd, readAddTest, claimNoCommand, validateAdd, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	native := NewNativeAgent(cfg, dir, nil)

	response, err := native.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !response.VerifiedAlreadySatisfied {
		t.Fatalf("the post-window correction did not complete: %+v", response)
	}
}

// TestLoopCorrectionKeepsCommandPolicy proves the correction phase does not bypass the command
// policy: a command the policy refuses is still refused inside the phase (audited as a denial),
// and the run cannot reach acceptance through it.
func TestLoopCorrectionKeepsCommandPolicy(t *testing.T) {
	dir, req := loopCorrectionFixture(t)
	// ... turn 3 opens the correction; turn 4 asks for a policy-denied command; turn 5
	// re-claims without the required validation, so the run must stop.
	_, srv := newFakeOllama(t, readAdd, readAddTest, claimNoCommand, denyCommand, claimNoCommand, claimNoCommand)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}

	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("a policy-denied command must not lead to acceptance: err=%v evidence=%+v", err, ev)
	}
	denied := false
	for _, r := range h.AuditRecords() {
		if r.Tool == toolharness.ToolRunCommand && r.Action == toolharness.ActionDeny {
			denied = true
		}
	}
	if !denied {
		t.Fatalf("the command policy denial must be audited: %+v", h.AuditRecords())
	}
}

// TestLoopCorrectionIsOfferedOnlyOnce proves a second invalid claim cannot trigger another
// correction: the harness requests exactly one, then stops for a human.
func TestLoopCorrectionIsOfferedOnlyOnce(t *testing.T) {
	dir, req := loopCorrectionFixture(t)
	// turn 3 opens the correction; turn 4 repeats the same invalid claim.
	_, srv := newFakeOllama(t, readAdd, readAddTest, claimNoCommand, claimNoCommand, claimNoCommand)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}

	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("an uncorrected claim must stop for a human: err=%v evidence=%+v", err, ev)
	}
	corrections := 0
	for _, r := range h.TraceRecords() {
		if r.Event == correctionEvent {
			corrections++
		}
	}
	if corrections != 1 {
		t.Fatalf("corrections requested = %d, want exactly 1", corrections)
	}
}
