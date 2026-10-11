package ollamaagent

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// IMPLEMENT is a phased mutating capability. Its lifecycle is the shared
// three-phase engine in orchestrate.go (DISCOVERY → CHANGE → FINALIZE); this file
// owns everything IMPLEMENT-specific: the phase instructions and nudges injected
// into the transcript, the phase event markers, and IMPLEMENT's entry point into
// the shared engine. FIX enters the same engine through its own seam (fix.go);
// the bounds themselves are centralized in policy.go.

// IMPLEMENT phase instructions. They are injected as suffixes on a tool result so
// the transcript keeps alternating assistant/user turns.
const (
	implementChangeInstruction = `The requested change has begun.

Focus only on completing the change.

You may inspect relevant files, inspect the diff, or run targeted checks when
necessary. Do not resume broad repository exploration.

Once the requested change is implemented, return the required structured
execution outcome immediately. SOP will perform independent validation after you
return.`

	implementNudge = `You have gathered substantial repository context.

Begin making the requested change now.

Only inspect additional files when they are directly necessary to complete the
implementation. Do not continue broad repository exploration.`

	implementFinalizeInstruction = `Work on the requested change is complete for this invocation.

Do not request additional tools.

Return the required structured execution outcome now using the work and
repository context already available.

SOP will independently run build, test, lint, review, and quality gates after
you return. Do not continue trying to prove the implementation yourself.`

	implementFinalizeCorrection = `Tool execution is complete for this invocation.

No additional tools are available.

Return the required structured execution outcome now. SOP will perform
independent validation.`

	// implementConvergenceCorrection is returned when the bounded discovery window
	// has closed and no mutation has been observed for the stale allowance. The run
	// must now converge: make the requested repository change with the mutation
	// tools that remain available, or return a truthful structured outcome.
	implementConvergenceCorrection = `The bounded discovery window for this invocation has closed and no repository
change has been observed.

Non-mutating repository tools (read_file, list_files, search_files, and
non-mutating run_command) are no longer available. Further inspection is not
progress toward the task, and this invocation cannot continue by reading.

Either make the requested repository change now with the mutation tools that
remain available (write_file, create_file, delete_file, restore_file, or a
mutating run_command), or return a truthful structured execution outcome
(needs_human or failed) explaining what remains and why the change was not made.

Do not claim completion without making the required change. SOP will
independently validate the repository.`

	implementNowInstruction = `You have gathered enough repository context, but you have not yet made the
required repository change.

Stop broad exploration and implement the requested change now.

Repository tools remain available for implementation.

Use additional reads only when directly necessary to make the change.

Do not return a completed outcome until the required change has
actually been performed.

Once the implementation is complete, return the required structured outcome.
SOP will perform independent validation afterward.`

	implementClosingInstruction = `You have not yet made the required repository change.

This invocation is about to end: if you do not make the change now, it ends
without one and SOP continues this task in a later bounded invocation.

Make the requested change now using the tools still available, or return a
truthful structured outcome (needs_human or failed) explaining what remains.

Do not claim completion without making the required change.`

	implementFinalInstruction = `You have not yet performed the required repository change.

This invocation is ending, so no further repository tools are available.

Return a truthful structured outcome now (needs_human or failed) explaining what
remains to be implemented and why the change was not made in this invocation.
SOP will continue this task in a later bounded invocation.

Do not claim completion without making the required change. Do not invent
repository changes. SOP will independently validate the repository.`

	implementChangeEvent   = "→ CHANGE"
	implementFinalizeEvent = "→ FINALIZE"
	implementContinueEvent = "→ CHANGE_CONTINUE"
)

// executeImplement runs IMPLEMENT's discovery → change → finalize lifecycle on
// the shared phased engine, parameterized by the IMPLEMENT policy (policy.go). It
// records successful controlled mutations into ev, the invocation's mutation
// evidence owned by the caller's Complete call.
func (h *Harness) executeImplement(ctx context.Context, req agent.Request, ev *mutationEvidence) (string, error) {
	return h.executePhased(ctx, req, ev)
}
