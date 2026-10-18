package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// TestAcceptanceCargoIndependentOfAgentTool proves the acceptance-check Cargo
// permission admits a supported Cargo command without the agent-tool permission,
// and that the reverse is true (SOP-001).
func TestAcceptanceCargoIndependentOfAgentTool(t *testing.T) {
	root := t.TempDir()

	// Acceptance permission ON: cargo build is authorized.
	if _, err := authorizeAcceptanceCommandWith("cargo build", true, root); err != nil {
		t.Fatalf("acceptance cargo should be authorized when enabled: %v", err)
	}

	// Acceptance permission OFF: cargo build is not authorized, even though the
	// agent-tool permission may be on elsewhere (independence).
	if _, err := authorizeAcceptanceCommandWith("cargo build", false, root); err == nil {
		t.Fatal("acceptance cargo must be refused when the acceptance permission is off")
	}

	// Unsupported forms are refused even when enabled.
	for _, cmd := range []string{"cargo publish", "cargo build --bogus", "cargo build ; rm -rf /"} {
		if _, err := authorizeAcceptanceCommandWith(cmd, true, root); err == nil {
			t.Fatalf("acceptance cargo should reject %q", cmd)
		}
	}

	// A Go command stays authorized regardless of the Cargo permissions.
	if _, err := authorizeAcceptanceCommandWith("go build ./...", false, root); err != nil {
		t.Fatalf("go build should stay authorized: %v", err)
	}

	// The two permissions stay independent through configuration.
	c := config.Config{}
	c.Cargo.AgentTool = boolPtr(true)
	if !c.CargoAgentToolEnabled() || c.CargoAcceptanceCheckEnabled() {
		t.Fatal("agent-tool on must not enable acceptance-check")
	}
}

// TestFailedRequiredAcceptanceBlocksDependentTask proves a failed required
// acceptance check resolves to NEEDS_HUMAN through the existing verdict gate, so a
// dependent task does not execute (SOP-001, reusing the gate).
func TestFailedRequiredAcceptanceBlocksDependentTask(t *testing.T) {
	res := &outcomeArtifact{
		Status: outcome.Partial,
		Criteria: []outcome.CriterionResult{
			{ID: "check-1", Required: true, Met: false},
		},
	}
	cfg := config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}}
	applyLifecycleGateWithWaiver(cfg, nil, res, nil)
	if res.Lifecycle == outcome.LocalDone {
		t.Fatal("a failed required acceptance check must block LOCAL_DONE")
	}
	if res.Lifecycle != outcome.NeedsHuman {
		t.Fatalf("expected NEEDS_HUMAN, got %s", res.Lifecycle)
	}

	// With the gate off the lifecycle is unchanged (LOCAL_DONE), proving the block
	// comes from the existing gate, not a new one.
	off := &outcomeArtifact{Status: outcome.Partial, Criteria: []outcome.CriterionResult{{ID: "check-1", Required: true, Met: false}}}
	applyLifecycleGateWithWaiver(config.Config{}, nil, off, nil)
	if off.Lifecycle != outcome.LocalDone {
		t.Fatalf("gate off must leave LOCAL_DONE, got %s", off.Lifecycle)
	}
}
