package ollamaagent

import (
	"context"
	"strings"
	"testing"
)

// TestFinalizeCorrectionDemandsTheOutcomeJSON pins the FINALIZE steering: when a
// mutating run is force-finalized, the model is told that tools are gone and that
// it must return the structured outcome JSON — so a model that keeps calling tools
// is corrected instead of silently burning the finalization allowance.
func TestFinalizeCorrectionDemandsTheOutcomeJSON(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t, fixMutationWrites("fin", implementForceFinalizeAfter+10)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 300

	if _, err := New(cfg, dir).Execute(context.Background(), implementRequest()); err == nil || !strings.Contains(err.Error(), "finalization_limit") {
		t.Fatalf("err = %v, want a finalization_limit termination", err)
	}

	shown := false
	for i := 0; i < fake.count(); i++ {
		if strings.Contains(messageText(fake.request(i)), "outcome JSON") {
			shown = true
			break
		}
	}
	if !shown {
		t.Error("FINALIZE did not deliver the outcome-JSON correction to the model")
	}
}
