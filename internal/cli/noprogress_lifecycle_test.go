package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

const noProgressReason = "IMPLEMENT_NO_PROGRESS: the Ollama agent IMPLEMENT made no repository progress after 5 consecutive stale iterations (iterations=16, repository_mutations=0, changed_files=0, tool_calls=16, termination=no_progress); a retry may succeed"

// TestRunGraphNoProgressBlocksWithoutApproval proves a bounded no-progress stop
// blocks the task for operator intervention and never creates a human approval:
// there is no approve/decline decision to offer.
func TestRunGraphNoProgressBlocksWithoutApproval(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	// The harness maps a no-progress stop to a needs_human outcome; use that real
	// status so the test covers the path that previously parked at WAITING_FOR_HUMAN.
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "sop approval") || strings.Contains(stdout, "sop approve") || strings.Contains(stdout, "sop decline") {
		t.Errorf("no-progress must not offer an approval decision:\n%s", stdout)
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
		t.Errorf("task status = %s, want BLOCKED", got.Status)
	}
	if got.BlockedReason != domain.NO_PROGRESS {
		t.Errorf("blocked reason = %s, want NO_PROGRESS", got.BlockedReason)
	}
	// No approval was recorded for the run.
	if code, out, _ := runCLI(t, dir, "approvals", "--json"); code == exitOK && strings.Contains(out, "S001") {
		t.Errorf("an approval was created for a no-progress task:\n%s", out)
	}
}
