package ollamaagent

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// REVIEW's inspect-to-synthesize lifecycle: bounded read-only INSPECTION of the
// change under review, then tool-free SYNTHESIS of the verdict. REVIEW-specific
// bounds, labels, a soft inspection nudge, and instructions live here; the
// executor they parameterize is the shared two-phase orchestration in
// orchestrate.go.
//
// The inspection is deliberately short: the change under review arrives in the
// request, so tools only confirm what the diff does not say. The soft nudge
// (after reviewInspectNudgeAfter inspections) tells a still-inspecting model to
// wrap up without withdrawing anything; crossing reviewInspectTurns enters
// synthesis, where tools are disabled and at most reviewSynthesizeTurns model
// turns may produce the structured review result.
var reviewTwoPhase = twoPhase{
	discoveryTurns: reviewInspectTurns,
	synthesisTurns: reviewSynthesizeTurns,
	nudgeAfter:     reviewInspectNudgeAfter,
	nudge: `You have already inspected the essential context.

Finish inspection now and return the required structured REVIEW response,
unless one more specific, necessary check remains.`,
	discoverLabel: "INSPECT",
	synthLabel:    "SYNTHESIZE",
	instruction: `Inspection is complete.
Do not request any more tools.
Using only the repository context already gathered, produce the required REVIEW response now.
Do not continue exploring.`,
	correction: `Repository inspection is complete.
No additional tools are available.
Produce the required REVIEW response using the context already gathered.`,
}

// executeReview runs REVIEW's inspect → synthesize lifecycle and returns the
// structured review result. REVIEW stays read-only end to end: the policy
// allows no mutation tools, and synthesis disables tools entirely.
func (h *Harness) executeReview(ctx context.Context, req agent.Request) (string, error) {
	return h.executeTwoPhase(ctx, req, reviewTwoPhase)
}
