package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/failure"
)

// TestDispositionForActionCoversEveryGovernedAction confirms, against the
// production mapping, that the governed decision path can express all six
// failure dispositions the A/B corpus scores. It records the reachability
// finding rather than assuming that a declared vocabulary is complete.
func TestDispositionForActionCoversEveryGovernedAction(t *testing.T) {
	cases := []struct {
		action autonomy.Action
		want   failure.Disposition
	}{
		{autonomy.ActionAutoRetry, failure.Retry},
		{autonomy.ActionAutoContinue, failure.Continue},
		{autonomy.ActionAutoFix, failure.AutoFix},
		{autonomy.ActionAutoReconcile, failure.Replan},
		{autonomy.ActionTerminal, failure.Block},
		{autonomy.ActionHumanApproval, failure.NeedsHuman},
	}
	seen := map[failure.Disposition]bool{}
	for _, c := range cases {
		if got := dispositionForAction(c.action); got != string(c.want) {
			t.Errorf("dispositionForAction(%q) = %q, want %q", c.action, got, c.want)
		}
		seen[c.want] = true
	}
	if len(seen) != 6 {
		t.Fatalf("governed mapping reaches %d dispositions, want all 6: %v", len(seen), seen)
	}
}
