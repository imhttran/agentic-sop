package cli

import (
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/runmetrics"
)

// TestScheduledLifecycleRecordsCountsAndProvenance proves the real scheduled
// lifecycle writes counts.json (not just the unit-level RecordReport helper), with
// the five kinds separate and the candidate binary's checksum recorded.
func TestScheduledLifecycleRecordsCountsAndProvenance(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	planWithMapping(t, dir, "go vet ./...", []planner.AcceptanceMapping{{Criterion: 1, Check: 1}})
	task := reconcilePlanTask(t, dir)

	if code, out := runScheduledAcceptance(t, dir, task); code != exitOK {
		t.Fatalf("first run: code=%d out=%s", code, out)
	}
	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	counts := runmetrics.LoadCounts(runDir)
	if c := counts[runmetrics.CountInvocations]; !c.Known || c.Value != 1 {
		t.Errorf("invocations = %+v, want one recorded invocation", c)
	}
	if c := counts[runmetrics.CountAttempts]; !c.Known || c.Value < 1 {
		t.Errorf("attempts = %+v, want at least one recorded attempt", c)
	}
	if c := counts[runmetrics.CountContinuations]; !c.Known || c.Value != 0 {
		t.Errorf("continuations = %+v, want a recorded zero", c)
	}
	if c := counts[runmetrics.CountModelCalls]; !c.Known || c.Value < 1 {
		t.Errorf("model calls = %+v, want at least one", c)
	}
	if c := counts[runmetrics.CountApprovals]; !c.Known || c.Value != 0 {
		t.Errorf("approvals = %+v, want a recorded zero", c)
	}
	if c := counts[runmetrics.CountDirectInterventions]; !c.Known || c.Value != 0 {
		t.Errorf("direct interventions = %+v, want a recorded zero", c)
	}
	prov, ok := runmetrics.LoadBinaryProvenance(runDir)
	if !ok || prov.SHA256 == "" || prov.Path == "" {
		t.Fatalf("candidate binary provenance not recorded: %+v ok=%v", prov, ok)
	}

	// A second real lifecycle is a second invocation and a new attempt; the
	// absolute totals must advance without re-reading a seeded value as a delta.
	if code, out := runScheduledAcceptance(t, dir, task); code != exitOK {
		t.Fatalf("second run: code=%d out=%s", code, out)
	}
	counts = runmetrics.LoadCounts(runDir)
	if c := counts[runmetrics.CountInvocations]; c.Value != 2 {
		t.Errorf("invocations after restart = %+v, want 2", c)
	}
	if c := counts[runmetrics.CountAttempts]; c.Value != 2 {
		t.Errorf("attempts after restart = %+v, want 2", c)
	}
}
