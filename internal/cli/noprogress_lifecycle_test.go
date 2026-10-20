package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/recovery"
	"github.com/imhttran/agentic-sop/internal/store"
)

const noProgressReason = "IMPLEMENT_NO_PROGRESS: the Ollama agent IMPLEMENT made no repository progress after 5 consecutive stale iterations (iterations=16, repository_mutations=0, changed_files=0, tool_calls=16, termination=no_progress); a retry may succeed"

// TestRunGraphNoProgressBlocksWithoutApproval proves a bounded no-progress stop
// gets one automatic checkpoint recovery, then blocks the task for operator
// intervention and never creates a human approval: there is no approve/decline
// decision to offer.
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

	code, firstStdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("first run: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(firstStdout, "requeued") {
		t.Errorf("first no-progress stop must be requeued for one bounded recovery:\n%s", firstStdout)
	}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("second run: code=%d, want %d", code, exitError)
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
		t.Errorf("task status = %s, want BLOCKED after the bounded recovery", got.Status)
	}
	if got.BlockedReason != domain.NO_PROGRESS {
		t.Errorf("blocked reason = %s, want NO_PROGRESS", got.BlockedReason)
	}
	// State preservation: the retry kept the task history and its attempt count;
	// the recovery journal and the single blocker report are durable evidence.
	if got.Attempt < 1 || len(got.Attempts) == 0 {
		t.Errorf("task history was not preserved: attempt=%d attempts=%d", got.Attempt, len(got.Attempts))
	}
	runDir := filepath.Join(dir, stateDirName, "runs", "S001")
	journal, err := os.ReadFile(filepath.Join(runDir, "recovery.jsonl"))
	if err != nil {
		t.Fatalf("recovery journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
	if len(lines) < 2 {
		t.Fatalf("recovery journal = %q, want a retry and a terminal record", journal)
	}
	var recovered recovery.Recovery
	if err := json.Unmarshal([]byte(lines[0]), &recovered); err != nil {
		t.Fatalf("recovery journal is not valid JSON: %v (%s)", err, lines[0])
	}
	if recovered.Class != recovery.ImplementationNoProgress || recovered.Action != recovery.ActionRetrySame {
		t.Errorf("first recovery = %+v, want one implementation_no_progress retry", recovered)
	}
	var terminal recovery.Recovery
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &terminal); err != nil {
		t.Fatalf("terminal recovery journal line is not valid JSON: %v (%s)", err, lines[len(lines)-1])
	}
	if terminal.Action != recovery.ActionHuman {
		t.Errorf("terminal recovery = %+v, want the human/block decision", terminal)
	}
	blocker, err := os.ReadFile(filepath.Join(runDir, "blocker.json"))
	if err != nil {
		t.Fatalf("blocker report: %v", err)
	}
	if !strings.Contains(string(blocker), `"task": "S001"`) || !strings.Contains(string(blocker), "implementation_no_progress") {
		t.Errorf("blocker report does not describe the terminal no-progress state:\n%s", blocker)
	}
	// No approval was recorded for the run.
	if code, out, _ := runCLI(t, dir, "approvals", "--json"); code == exitOK && strings.Contains(out, "S001") {
		t.Errorf("an approval was created for a no-progress task:\n%s", out)
	}
}
