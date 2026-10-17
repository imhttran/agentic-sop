package ollamaagent

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// A completion claim that asserts success without the already-satisfied object: the
// shape the observed NEXT-002 run produced.
const plainCompletion = `{"status":"completed","summary":"the requested implementation already exists"}`

// A false claim: it cites a path the harness never observed being read.
const invalidSatisfiedClaim = `{"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"Add returns the sum","paths":["never_read.go"]}]}}`

// A real mutation (content that differs from the committed fixture), so a run that
// changes the repository cannot request existing work.
const writeAdd = `{"tool":"write_file","args":{"path":"add.go","content":"package add\n\n// changed by this invocation\nfunc Add(a, b int) int { return a + b }\n"}}`

func committedFixture(t *testing.T) (string, agent.Request) {
	t.Helper()
	dir, req := satisfactionFixture(t)
	gitInit(t, dir)
	commitAll(t, dir, "existing implementation")
	req.Capability = agent.Implement
	// The CLI drops descriptive (non docs/reports) deliverables before the harness
	// (taskReportDeliverables), so an existing-work task arrives with none.
	req.Deliverables = nil
	return dir, req
}

func requestedCount(h *Harness) int {
	n := 0
	for _, r := range h.TraceRecords() {
		if r.Event == "existing-work verification requested" {
			n++
		}
	}
	return n
}

// TestExistingWorkVerificationClosesCommittedWork: an invocation that reports completion
// with no attempt-local change is asked once for the existing already-satisfied object, and
// the existing verifier closes the task with no mutation credited.
func TestExistingWorkVerificationClosesCommittedWork(t *testing.T) {
	dir, req := committedFixture(t)
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, plainCompletion, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}

	if _, _, _ = h.CompleteWithEvidence(context.Background(), req, ev); !ev.alreadySatisfied {
		t.Fatal("committed existing work was not verified")
	}
	if got := requestedCount(h); got != 1 {
		t.Fatalf("existing-work requests = %d, want exactly 1", got)
	}
}

// TestExistingWorkRequestNotMadeAfterMutation: a run that changed the repository is never
// offered the existing-work route, so an attempt-local mutation cannot be laundered into a
// zero-mutation completion.
func TestExistingWorkRequestNotMadeAfterMutation(t *testing.T) {
	dir, req := committedFixture(t)
	_, srv := newFakeOllama(t, writeAdd, plainCompletion)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)

	ev := &mutationEvidence{}
	_, _, _ = h.CompleteWithEvidence(context.Background(), req, ev)
	if ev.alreadySatisfied {
		t.Fatal("a mutated invocation must not complete as existing work")
	}
	if got := requestedCount(h); got != 0 {
		t.Fatalf("existing-work requests = %d, want 0 after a mutation", got)
	}
}

// TestExistingWorkRequestIsBoundedOnce: a second plain completion does not request again.
func TestExistingWorkRequestIsBoundedOnce(t *testing.T) {
	dir, req := committedFixture(t)
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, plainCompletion, plainCompletion)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}

	_, _, _ = h.CompleteWithEvidence(context.Background(), req, ev)
	if ev.alreadySatisfied {
		t.Fatal("an unverified repeat must not be accepted")
	}
	if got := requestedCount(h); got != 1 {
		t.Fatalf("existing-work requests = %d, want exactly 1 (bounded)", got)
	}
}

// TestFalseCompletionClaimStillFailsClosed: after the bounded request, a false claim is
// rejected by the existing verifier and is never accepted.
func TestFalseCompletionClaimStillFailsClosed(t *testing.T) {
	dir, req := committedFixture(t)
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, plainCompletion, invalidSatisfiedClaim, invalidSatisfiedClaim)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}

	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("a false claim must fail closed: err=%v evidence=%+v", err, ev)
	}
}

// TestNEXT002ScenarioClosesWithNoNewChanges reproduces the NEXT-002 closure shape: prose
// deliverables (dropped by the CLI), an implementation already committed, criteria that name
// the evidence files, and no attempt-local change. It must close through the existing
// already-satisfied verification with no new implementation and no mutation.
func TestNEXT002ScenarioClosesWithNoNewChanges(t *testing.T) {
	dir, req := committedFixture(t)
	req.AcceptanceCriteria = []string{"Add returns the sum"}
	req.ValidationCommands = []string{"go test ./..."}
	_, srv := newFakeOllama(t, readAdd, readAddTest, validateAdd, plainCompletion, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)

	ev := &mutationEvidence{}
	if _, _, _ = h.CompleteWithEvidence(context.Background(), req, ev); !ev.alreadySatisfied {
		t.Fatal("NEXT-002 shape did not close")
	}
}

// TestExistingWorkRequestReachability pins the guard: the bounded request is offered only
// for a completion claim, with no attempt-local mutation, a captured baseline, no bounded
// request already used, and a turn still available inside the existing ceiling.
func TestExistingWorkRequestReachability(t *testing.T) {
	completed := map[string]any{"status": string(agent.OutcomeCompleted)}
	claim := map[string]any{"status": string(agent.OutcomeCompleted), "completion": agent.AlreadySatisfied}

	cases := []struct {
		name  string
		proof *satisfactionProof
		st    *executionState
		final map[string]any
		iter  int
		maxIt int
		want  bool
	}{
		{"eligible", satisfProof("fp"), &executionState{}, completed, 3, 32, true},
		{"no baseline (contract unavailable)", satisfProof(""), &executionState{}, completed, 3, 32, false},
		{"mutation observed", satisfProof("fp"), &executionState{mutationObserved: true}, completed, 3, 32, false},
		{"not a completion claim", satisfProof("fp"), &executionState{}, map[string]any{"status": string(agent.OutcomeFailed)}, 3, 32, false},
		{"already the claim object", satisfProof("fp"), &executionState{}, claim, 3, 32, false},
		{"no turn remains", satisfProof("fp"), &executionState{}, completed, 32, 32, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.proof.canRequestExistingWork(c.final, c.st, c.iter, c.maxIt); got != c.want {
				t.Fatalf("canRequestExistingWork = %v, want %v", got, c.want)
			}
		})
	}

	p := satisfProof("fp")
	p.existingWorkRequested = true
	if p.canRequestExistingWork(completed, &executionState{}, 1, 32) {
		t.Fatal("the request must be bounded to once")
	}
	q := satisfProof("fp")
	q.evidenceCorrectionUsed = true
	if q.canRequestExistingWork(completed, &executionState{}, 1, 32) {
		t.Fatal("the existing fail-closed path after a used evidence correction must be preserved")
	}
}
