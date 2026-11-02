package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/acp"
)

// The full session/update translation and delivery behavior is ACP-004's
// deterministic acceptance coverage in internal/acp (TestProgress). This file
// proves the host wiring only: runACP's server gets a usable progress source,
// the source fabricates nothing for a project with no recorded job, and the
// persisted job-record state mapping is the closed set ACP-004 pins.

// TestACPProgressSourceIsWired proves the source runACP passes to
// acp.WithProgress exists and can read, so `sop acp` streams progress.
func TestACPProgressSourceIsWired(t *testing.T) {
	src := acpProgress(defaultWorkEnv())
	if src == nil || src.Progress == nil {
		t.Fatal("acpProgress returned no usable source; runACP would serve without acp.WithProgress")
	}
}

// TestACPProgressSourceDoesNotFabricate proves a project with no recorded job
// reports nothing: no frame material, no invented state.
func TestACPProgressSourceDoesNotFabricate(t *testing.T) {
	src := acpProgress(defaultWorkEnv())
	snap, ok, err := src.Progress(t.TempDir(), "job-absent")
	if err == nil && ok {
		t.Fatalf("a project with no recorded jobs reported %+v; the progress source must not fabricate progress", snap)
	}
	if ok && snap.JobID != "job-absent" {
		t.Fatalf("progress reported job %q for a read of job-absent", snap.JobID)
	}
}

// TestACPProgressSourceMapsPersistedStates pins the job-record state mapping:
// the derived record set is closed, and anything else is the ambiguous ended
// case — never an invented state.
func TestACPProgressSourceMapsPersistedStates(t *testing.T) {
	cases := map[string]string{
		"running":   acp.ProgressStateRunning,
		"completed": acp.ProgressStateCompleted,
		"failed":    acp.ProgressStateFailed,
		"ended":     acp.ProgressStateEnded,
	}
	for record, want := range cases {
		if got := mapWorkState(record); got != want {
			t.Errorf("mapWorkState(%q) = %q, want %q", record, got, want)
		}
	}
	if got := mapWorkState("mystery"); got != acp.ProgressStateEnded {
		t.Errorf("mapWorkState(mystery) = %q, want the ambiguous %q", got, acp.ProgressStateEnded)
	}
}
