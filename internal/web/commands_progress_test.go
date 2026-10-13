package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"sop-controller/internal/sopclient"
)

func waitCommandState(t *testing.T, cr *CommandRunner, project, verb string) CommandState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := cr.Status(project, verb); ok && !st.Running() {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	st, _ := cr.Status(project, verb)
	t.Fatalf("command did not finish: %+v", st)
	return CommandState{}
}

// TestCommandRunnerStreamsLiveOutput proves a running command's output is visible
// before it finishes (DP-010), and the final output is retained.
func TestCommandRunnerStreamsLiveOutput(t *testing.T) {
	cr := NewCommandRunner(2 * time.Second)
	started, err := cr.Start("p", "start", func(ctx context.Context) (string, error) {
		w := sopclient.ProgressWriter(ctx)
		if w == nil {
			return "", context.Canceled
		}
		w.Write([]byte("phase-1\n"))
		time.Sleep(30 * time.Millisecond)
		w.Write([]byte("phase-2\n"))
		return "phase-1\nphase-2\ndone\n", nil
	})
	if err != nil || !started {
		t.Fatalf("start: started=%v err=%v", started, err)
	}
	sawPartial := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := cr.Status("p", "start"); ok && strings.Contains(st.Output, "phase-1") {
			sawPartial = true
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !sawPartial {
		t.Error("live output was not visible while the command ran")
	}
	final := waitCommandState(t, cr, "p", "start")
	if final.State != "done" || !strings.Contains(final.Output, "done") {
		t.Errorf("final = %+v, want done with full output", final)
	}
}

// TestCommandRunnerBlockedState proves a start/run that SOP parks at a gate is
// reported as "blocked", not a failure or a conflict (DP-010).
func TestCommandRunnerBlockedState(t *testing.T) {
	cr := NewCommandRunner(2 * time.Second)
	if _, err := cr.Start("p", "start", func(ctx context.Context) (string, error) {
		return "run output", &blockedError{msg: "gate open"}
	}); err != nil {
		t.Fatal(err)
	}
	st := waitCommandState(t, cr, "p", "start")
	if st.State != "blocked" || st.Conflict {
		t.Errorf("state = %+v, want blocked without conflict", st)
	}
	if !strings.Contains(st.Error, "gate open") {
		t.Errorf("blocked message missing: %q", st.Error)
	}
}

// TestCommandRunnerFailedState proves a plain error stays "error".
func TestCommandRunnerFailedState(t *testing.T) {
	cr := NewCommandRunner(2 * time.Second)
	if _, err := cr.Start("p", "validate", func(ctx context.Context) (string, error) {
		return "boom", context.DeadlineExceeded
	}); err != nil {
		t.Fatal(err)
	}
	if st := waitCommandState(t, cr, "p", "validate"); st.State != "error" {
		t.Errorf("state = %+v, want error", st)
	}
}

// TestBoundOutputKeepsTail proves the retained output is bounded and keeps the most
// recent bytes (the tail).
func TestBoundOutputKeepsTail(t *testing.T) {
	long := strings.Repeat("a", 100) + "TAIL"
	got := boundOutput(long, 10)
	if !strings.HasSuffix(got, "TAIL") {
		t.Errorf("boundOutput must keep the tail: %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("boundOutput must mark truncation: %q", got)
	}
}
