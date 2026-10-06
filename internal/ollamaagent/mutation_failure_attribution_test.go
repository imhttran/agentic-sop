package ollamaagent

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestFailedMutationIsReportedAsFailureNotObservationGap pins the diagnostic
// split that hid the CLOSE-004 cause: an operation that never executed is reported
// as a failed mutation, while "verification_unavailable" stays reserved for an
// executed operation whose effect could not be independently verified. Neither is
// credited as progress.
func TestFailedMutationIsReportedAsFailureNotObservationGap(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(capability), func(t *testing.T) {
			dir := t.TempDir()
			// delete_file on a missing path: the tool fails, so nothing executed.
			_, srv := newFakeOllama(t,
				`{"tool":"delete_file","args":{"path":"missing.txt"}}`,
				`{"status":"completed","summary":"done","changes_expected":false}`)
			h := New(testConfig(srv.URL), dir)
			req := implementRequest()
			req.Capability = capability
			ev := &mutationEvidence{}
			_, _ = h.ExecuteWithEvidence(context.Background(), req, ev)

			if ev.observed {
				t.Fatalf("a failed mutation was credited: %+v", ev)
			}
			var failed, gap bool
			for _, rec := range h.TraceRecords() {
				switch rec.Detail {
				case "mutation_failed":
					failed = true
				case "verification_unavailable":
					gap = true
				}
			}
			if !failed || gap {
				t.Fatalf("trace=%+v, want the failure attributed as a failure, not an observation gap", h.TraceRecords())
			}
		})
	}
}
