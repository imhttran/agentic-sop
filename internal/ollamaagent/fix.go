package ollamaagent

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// FIX is a phased mutating capability with its own seam. It arrives with a
// diagnosis (the failing check or the blocking review findings), so its scope is
// narrower than IMPLEMENT's and its iteration ceiling is lower; the phase
// machine, the mutation-aware completion, and the tool policy engine it runs on
// are IMPLEMENT's (implement.go), parameterized by the FIX policy (policy.go). It
// records successful controlled mutations into ev, the invocation's mutation
// evidence owned by the caller's Complete call.
func (h *Harness) executeFix(ctx context.Context, req agent.Request, ev *mutationEvidence) (string, error) {
	return h.executePhased(ctx, req, ev)
}
