package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// gateCountingAgent records implementation calls so a blocked gate is proven to
// stop work before the model, not merely to return an error after it ran.
type gateCountingAgent struct {
	dir            string
	implementCalls int
}

func (a *gateCountingAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement, agent.Fix:
		a.implementCalls++
		if err := os.WriteFile(filepath.Join(a.dir, "impl.txt"), []byte("done\n"), 0o644); err != nil {
			return agent.Response{}, err
		}
		return agent.Response{Content: `{"status":"completed","summary":"done","changes_expected":true}`}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func seedApprovalStatus(t *testing.T, dir, taskID string, status domain.ApprovalStatus) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	req := domain.ApprovalRequest{
		ID:          "APR-1",
		TaskID:      taskID,
		Kind:        domain.ApprovalNeedsHuman,
		Target:      taskID,
		Reason:      "required acceptance evidence is missing or failed",
		Stage:       string(runpkg.WaitingForHuman),
		RequestedAt: mustTime(t, "2026-10-18T00:00:00Z"),
		Status:      status,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "approval.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	out, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// approveIfPending records an approval when the task has a PENDING request, so a
// test that intentionally exercises post-boundary recovery can move past the
// approval gate without weakening it.
func approveIfPending(t *testing.T, dir, taskID string) {
	t.Helper()
	req, ok := runpkg.At(runpkg.Dir(dir, taskID)).Approval()
	if !ok || req.Status != domain.ApprovalPending {
		return
	}
	if code, _, stderr := runCLI(t, dir, "approve", taskID, "--by", "operator"); code != exitOK {
		t.Fatalf("approve %s: code=%d stderr=%s", taskID, code, stderr)
	}
}

// TestPendingApprovalBlocksThenApprovedRunExecutes proves the production path:
// a PENDING request stops execution before any model call, an explicit approval
// is required, and the approved rerun executes without editing state or resetting
// the continuation budget.
func TestPendingApprovalBlocksThenApprovedRunExecutes(t *testing.T) {
	dir := budgetGraph(t)
	seedApprovalStatus(t, dir, "S001", domain.ApprovalPending)
	if err := runpkg.At(runpkg.Dir(dir, "S001")).RecordContinuations(1); err != nil {
		t.Fatal(err)
	}
	a := &gateCountingAgent{dir: dir}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/impl.txt b/impl.txt\n--- /dev/null\n+++ b/impl.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
	if code == exitOK {
		t.Fatalf("a pending approval executed: stdout=%s stderr=%s", stdout, stderr)
	}
	if a.implementCalls != 0 {
		t.Fatalf("pending approval called the implementation model %d time(s)", a.implementCalls)
	}
	if !strings.Contains(stdout, "WAITING_FOR_APPROVAL") || !strings.Contains(stdout, "sop approve S001") {
		t.Fatalf("stdout does not name the pending gate:\n%s", stdout)
	}
	if got := runpkg.At(runpkg.Dir(dir, "S001")).ReadContinuations(); got != 1 {
		t.Fatalf("continuations = %d after a blocked gate, want 1", got)
	}

	if code, _, errOut := runCLI(t, dir, "approve", "S001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, errOut)
	}

	code, stdout, stderr = runInjectedCLI(t, dir, "diff --git a/impl.txt b/impl.txt\n--- /dev/null\n+++ b/impl.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
	if code != exitOK {
		t.Fatalf("approved rerun: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.implementCalls == 0 {
		t.Fatal("the approved rerun did not execute the implementation")
	}
	if got := runpkg.At(runpkg.Dir(dir, "S001")).ReadContinuations(); got != 1 {
		t.Fatalf("continuations = %d after approved recovery, want 1 (no reset)", got)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.LOCAL_DONE {
		t.Fatalf("task = %s, want LOCAL_DONE", task.Status)
	}
}

// TestDeclinedApprovalBlocksSecondRun proves a decline is a decision, not a
// missing one: the task does not become runnable merely because it is re-run.
func TestDeclinedApprovalBlocksSecondRun(t *testing.T) {
	dir := budgetGraph(t)
	seedApprovalStatus(t, dir, "S001", domain.ApprovalDeclined)
	a := &gateCountingAgent{dir: dir}

	for i := 0; i < 2; i++ {
		code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/impl.txt b/impl.txt\n--- /dev/null\n+++ b/impl.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
		if code == exitOK {
			t.Fatalf("run %d executed a declined approval: stdout=%s stderr=%s", i+1, stdout, stderr)
		}
		if a.implementCalls != 0 {
			t.Fatalf("run %d called the implementation model %d time(s)", i+1, a.implementCalls)
		}
		if !strings.Contains(stderr, "approval declined") {
			t.Fatalf("run %d stderr does not name the decline:\n%s", i+1, stderr)
		}
	}
}
