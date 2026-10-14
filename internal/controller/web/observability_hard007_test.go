package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/sopclient"
)

// HARD007 additions: deterministic observability tests that fill the gaps in
// observability_test.go (human-gate surfacing, no-activity handling, and a
// timeout test kept separate from polling). Every interval here is short and
// bounded, and synchronization is done with channels or bounded polls - never
// a real multi-minute sleep.

// mustSopClient builds a sopclient over the same fixture root the test server
// uses, so a test can read the SOP-persisted projection the web layer renders.
func mustSopClient(t *testing.T, root string) *sopclient.Client {
	t.Helper()
	sop, err := sopclient.New([]string{root}, withApprovals(t, "sop"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sop.Close() })
	return sop
}

// --- Human-gate surfacing -------------------------------------------------

// A SOP human-gate decision is surfaced as a human/approval boundary, not as a
// generic "running" state. It is driven through the existing SOP read surface
// (the task detail's approval projection), which derives the boundary ONLY from
// SOP-persisted evidence. The t3 fixture seeds a WAITING_FOR_HUMAN run stage
// plus a NEEDS_HUMAN classification, so the boundary must be Present and its
// Kind must be a human-gate kind rather than an ambiguous running signal.
func TestHumanGateDecisionSurfacedNotGenericRunning(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")

	sop := mustSopClient(t, root)
	detail, err := sop.Task(context.Background(), id, "t3")
	if err != nil {
		t.Fatalf("Task(t3): %v", err)
	}

	// The human gate must be surfaced as an explicit boundary, not inferred from
	// activity wording, retries, or an attempt count.
	if !detail.Approval.Present {
		t.Fatalf("t3 reports a NEEDS_HUMAN/WAITING_FOR_HUMAN gate; Approval.Present=false would surface it as generic running work")
	}
	switch detail.Approval.Kind {
	case "NEEDS_HUMAN", "WAITING_FOR_HUMAN", "BLOCKED":
		// ok: a SOP-reported human boundary kind
	default:
		t.Fatalf("Approval.Kind = %q, want a human-gate kind, not a generic running state", detail.Approval.Kind)
	}

	// And it must be visible through the existing rendered surface too.
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t3")
	if code != 200 {
		t.Fatalf("task detail: status %d", code)
	}
	if !strings.Contains(body, "NEEDS HUMAN") && !strings.Contains(body, "NEEDS_HUMAN") {
		t.Fatalf("task detail did not surface the human gate; body:\n%s", body)
	}
	if !strings.Contains(body, "Two valid contracts") {
		t.Fatalf("task detail did not surface the SOP-reported gate evidence; body:\n%s", body)
	}

	// A task with NO SOP-reported boundary (t1 is mid-run) must not be surfaced
	// as a human gate - the boundary is read from SOP alone and never inferred.
	mid, err := sop.Task(context.Background(), id, "t1")
	if err != nil {
		t.Fatalf("Task(t1): %v", err)
	}
	if mid.Approval.Present {
		t.Fatalf("t1 has no SOP-reported human boundary; Approval.Present=true fabricates a gate")
	}
}

// --- No-activity handling -------------------------------------------------

// Absence of activity is NOT inferred as failure or blockage. Over a bounded
// quiet interval an idle command stays "running" (a state CommandRunner
// actually defines) with no error and no invented blocked/failed state.
func TestNoActivityDoesNotInferFailureOrBlockage(t *testing.T) {
	r := NewCommandRunner(time.Minute)
	release := make(chan struct{})
	if ok, err := r.Start("proj", "idle", func(ctx context.Context) (string, error) {
		<-release
		return "done", nil
	}); !ok || err != nil {
		t.Fatalf("Start: ok=%v err=%v", ok, err)
	}

	// Observe across a bounded quiet interval. Only the real states are allowed:
	// running | done | error. No blocked/failed state may be invented merely
	// because nothing happened.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		st, ok := r.Status("proj", "idle")
		if !ok {
			t.Fatal("status disappeared during a quiet period")
		}
		switch st.State {
		case "running":
			// truthful: still in flight, nothing to report yet
		case "done", "error":
			t.Fatalf("state = %q during a quiet interval with no activity; no failure/blockage may be inferred", st.State)
		default:
			t.Fatalf("invented state %q; CommandRunner only defines running|done|error", st.State)
		}
		if st.Error != "" {
			t.Fatalf("error = %q during a quiet interval; no failure may be inferred from inactivity", st.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	if got := waitState(t, r, "proj", "idle"); got.State != "done" {
		t.Fatalf("after release: state = %q, want done", got.State)
	}
}

// A genuinely quiet activity surface reports the same truthful read it would
// with events: an empty/quiet window is not a failure signal. This drives the
// existing activity window surface and asserts the read succeeds with no error
// and no fabricated failure event, over a bounded short poll.
func TestQuietActivityWindowIsNotAFailure(t *testing.T) {
	srv, id, _ := newTestServerRoot(t, "sop")

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		code, body := get(t, srv.URL+"/projects/"+id+"/activity/window")
		if code != http.StatusOK {
			t.Fatalf("activity window: status %d, want 200 (a quiet surface is not a failure)", code)
		}
		if body == "" {
			t.Fatal("activity window returned an empty body; the read surface must be truthful, not blank")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// --- Timeout, separate from polling ---------------------------------------

// CommandTimeout is verified separately from any status-polling assertion: a
// short bounded timeout drives the runner to "error" on the context deadline,
// with a blocking command that waits on the context (channel/context
// synchronization, no real multi-minute sleep). This deliberately does not poll
// the status endpoint in its assertion, keeping the timeout signal distinct
// from polling.
func TestCommandTimeoutSeparateFromPolling(t *testing.T) {
	r := NewCommandRunner(50 * time.Millisecond)

	// The command blocks until the runner's own timeout cancels its context.
	fn := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if ok, err := r.Start("proj", "slow", fn); !ok || err != nil {
		t.Fatalf("Start: ok=%v err=%v", ok, err)
	}

	st := waitState(t, r, "proj", "slow")
	if st.State != "error" {
		t.Fatalf("state = %q, want error from the CommandTimeout deadline", st.State)
	}
	if !strings.Contains(st.Error, "deadline exceeded") && !strings.Contains(st.Error, "DeadlineExceeded") {
		t.Fatalf("error = %q, should report the timeout/deadline, not a polling artifact", st.Error)
	}
}
