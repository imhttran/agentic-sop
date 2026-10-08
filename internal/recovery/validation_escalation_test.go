package recovery

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

// TestMissingValidationDoesNotEscalate proves a missing required validation set is
// never escalated to a larger model and never replanned: the bounded fix loop cannot
// repair a configuration gap, and a stronger model would repeat the same failure
// under unchanged configuration. The recovery policy must not spend a class on it.
func TestMissingValidationDoesNotEscalate(t *testing.T) {
	ev := Evidence{
		Kind:        failure.ValidationNotConfigured,
		Disposition: failure.Block,
		Class:       model.ClassSmall,
		Attempt:     1,
	}
	if implementationFailure(ev) {
		t.Fatalf("a missing validation set must not be an implementation failure: %+v", ev)
	}
	for _, p := range []Policy{
		{Enabled: true, MaxEscalations: 2},
		{Enabled: true, Replan: true, MaxReplans: 2, MaxEscalations: 2},
	} {
		dec := Decide(p, ev)
		if dec.Action == ActionEscalate || dec.Action == ActionReplan {
			t.Fatalf("a missing validation set must not escalate or replan: policy=%+v decision=%+v", p, dec)
		}
	}
}
