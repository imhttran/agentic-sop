package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/recovery"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// TestApplyTaskRoutingFailsClosedWithoutFactory pins the Phase 5.0 / P5.4-001
// invariant: when routing selects a model but SOP has no agent factory to build
// it, the task MUST fail with an actionable error rather than silently keeping the
// previously built agent. Silently continuing would execute a model other than the
// one routing selected, breaking "selected model == executing model".
func TestApplyTaskRoutingFailsClosedWithoutFactory(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")

	cfg := config.Default()
	spec := &taskfile.Spec{ID: "T1", Title: "Add widget"}
	d := deps{routingEnabled: true} // note: no newAgent factory

	prior := routingAgent()
	a, tr, err := applyTaskRouting(context.Background(), cfg, d, spec, earlyGateResult{}, earlyGateResult{}, prior, io.Discard)
	if err == nil {
		t.Fatal("expected a fail-closed error, got nil")
	}
	if !strings.Contains(err.Error(), "no agent factory") {
		t.Fatalf("error should name the missing agent factory: %v", err)
	}
	// Routing selected a class, so the caller must not be handed a decision: the
	// task stops instead of running on the wrong model.
	if tr != nil {
		t.Errorf("no routing decision should be returned on fail-closed: %+v", tr)
	}
	if a != prior {
		t.Errorf("the previous agent must not be replaced on fail-closed")
	}
}

// TestEscalateToFailsClosedWithoutFactory pins the same invariant for bounded
// escalation (Phase 5): an escalated class that cannot be constructed stops the
// escalation rather than silently reusing the previous model.
func TestEscalateToFailsClosedWithoutFactory(t *testing.T) {
	clearModelEnv(t)

	cfg := config.Default()
	d := deps{} // no agent factory
	dec := recovery.Decision{FromClass: model.ClassSmall, ToClass: model.ClassMedium, Reason: recovery.ReasonImplementationFailed}

	a, sel, err := escalateTo(context.Background(), cfg, d, dec)
	if err == nil {
		t.Fatal("expected a fail-closed error, got nil")
	}
	if !strings.Contains(err.Error(), "no agent factory") {
		t.Fatalf("error should name the missing agent factory: %v", err)
	}
	if a != nil {
		t.Errorf("no agent should be built on fail-closed")
	}
	if sel.Model != "" || sel.Provider != "" {
		t.Errorf("no selection should be returned on fail-closed: %+v", sel)
	}
}
