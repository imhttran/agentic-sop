package ollamaagent

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// PLAN's two-phase lifecycle: bounded read-only DISCOVERY, then tool-free
// SYNTHESIS. PLAN-specific bounds, labels, and instructions live here; the
// executor they parameterize is the shared two-phase orchestration in
// orchestrate.go. Discovery has no soft nudge: the capability already produces a
// document from request context, and its discovery budget is tight.
var planTwoPhase = twoPhase{
	discoveryTurns: planDiscoveryTurns,
	synthesisTurns: planSynthesisTurns,
	discoverLabel:  "DISCOVERY",
	synthLabel:     "SYNTHESIS",
	instruction: `Exploration is complete.
Do not request any more tools.
Using only the repository context already gathered, produce the required PLAN response now.
Do not continue exploring.
Do not implement anything.
Include the capability inventory (a status and the evidence for every capability the
work depends on) and any assumptions behind the plan.
Never report a missing or unknown capability as if it already exists.
Return the exact structured PLAN response expected by SOP.`,
	correction: `Repository discovery is complete.
No additional tools are available.
Produce the required PLAN response using the context already gathered.`,
}

// executePlan runs PLAN's discovery → synthesis lifecycle and returns the plan
// document. It never mutates the repository: the read-only tool policy is
// enforced by the two-phase executor in addition to this capability's prompt.
func (h *Harness) executePlan(ctx context.Context, req agent.Request) (string, error) {
	return h.executeTwoPhase(ctx, req, planTwoPhase)
}
