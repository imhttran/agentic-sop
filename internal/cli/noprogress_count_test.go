package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/recovery"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// TestPriorNoProgressRecoveriesCountsOnlyCheckpointResumes proves an unrelated
// no-change journal entry (for example a refused retry recorded by an earlier
// build) does not spend the single checkpoint recovery.
func TestPriorNoProgressRecoveriesCountsOnlyCheckpointResumes(t *testing.T) {
	rn, err := runpkg.New(t.TempDir(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	unrelated := recovery.Recovery{Attempt: 1, Class: recovery.ImplementationNoProgress, Action: recovery.ActionHuman, Reason: recovery.ReasonNoMaterialChange}
	if err := recordRecovery(rn, unrelated); err != nil {
		t.Fatal(err)
	}
	if got := priorNoProgressRecoveries(rn); got != 0 {
		t.Fatalf("unrelated entry counted: got %d, want 0", got)
	}
	resume := recovery.Recovery{Attempt: 2, Class: recovery.ImplementationNoProgress, Action: recovery.ActionRetrySame, Reason: recovery.ReasonResumeCheckpoint, Change: recovery.ChangeResumeCheckpoint}
	if err := recordRecovery(rn, resume); err != nil {
		t.Fatal(err)
	}
	if got := priorNoProgressRecoveries(rn); got != 1 {
		t.Fatalf("checkpoint resume: got %d, want 1", got)
	}
}
