package ollamaagent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Structured execution outcome handling for the IMPLEMENT/FIX command path.
//
// The model reports the outcome, but SOP already owns the structured-outcome
// vocabulary (agent.Outcome with completed/needs_human/failed). This file does
// not introduce a second vocabulary: it recognizes the same wire shape SOP's
// command provider parses (outcomeWire) and, for a completed outcome, reconciles
// the model's changes_expected claim with the repository change the harness
// actually observed, so a run cannot claim to have changed the repository when it
// did not (nor deny a change it made).

// reconcileOutcome grounds a completed IMPLEMENT/FIX outcome in observable
// repository reality. The model's reported changes_expected is compared with
// whether the repository working tree actually changed during the run; the
// observed truth wins, and a disagreement is recorded on the harness so it can be
// surfaced rather than silently accepted.
//
// A non-completed outcome (needs_human, failed) carries no changes_expected and
// is returned unchanged. Content that is not a recognized outcome is returned
// unchanged, so a legacy prose or document response is never rewritten. When the
// working tree cannot be inspected, the model's claim is left as reported.
func (h *Harness) reconcileOutcome(ctx context.Context, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || trimmed[0] != '{' {
		return content
	}
	var wire outcomeWire
	if err := json.Unmarshal([]byte(trimmed), &wire); err != nil {
		return content
	}
	switch wire.Status {
	case string(agent.OutcomeCompleted), string(agent.OutcomeNeedsHuman), string(agent.OutcomeFailed):
	default:
		return content
	}

	// Only a completed outcome asserts a repository change; needs_human and
	// failed outcomes are left exactly as the model reported them.
	if wire.Status != string(agent.OutcomeCompleted) {
		return content
	}

	observed, err := h.tools.WorkingTreeChanged(ctx)
	if err != nil {
		return content
	}
	reported := true
	if wire.ChangesExpected != nil {
		reported = *wire.ChangesExpected
	}
	wire.ChangesExpected = &observed

	h.mismatch = reported != observed
	if h.mismatch {
		// Keep the model's claim visible without changing SOP's outcome vocabulary:
		// the note explains why changes_expected was overridden by observation.
		wire.Summary = appendMismatchNote(wire.Summary, reported, observed)
	}

	data, err := json.Marshal(wire)
	if err != nil {
		return content
	}
	return string(data)
}

// retryNoChangeFailure rewrites a model-reported `failed` outcome into a
// retryable `needs_human` one when the working tree did not change. A mutating
// capability that changed nothing did not actually attempt the work, so a retry
// is warranted (bounded by max_attempts) rather than a hard failure — the same
// classification the harness gives its own no-change exhaustion. A `failed`
// outcome with an observed change is a real failure and is left as reported; so is
// a non-failed outcome or one that cannot be inspected.
func (h *Harness) retryNoChangeFailure(ctx context.Context, content string) string {
	var wire outcomeWire
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &wire); err != nil {
		return content
	}
	if wire.Status != string(agent.OutcomeFailed) {
		return content
	}
	changed, err := h.tools.WorkingTreeChanged(ctx)
	if err != nil || changed {
		return content
	}
	wire.Status = string(agent.OutcomeNeedsHuman)
	const note = "no repository change was made; retrying"
	if strings.TrimSpace(wire.Reason) == "" {
		wire.Reason = note
	} else {
		wire.Reason = strings.TrimSpace(wire.Reason) + " [" + note + "]"
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return content
	}
	return string(data)
}

// appendMismatchNote records the disagreement between the model's claim and the
// observed repository change in the summary, so it is surfaced to the operator
// instead of being silently corrected.
func appendMismatchNote(summary string, reported, observed bool) string {
	changed := "was not changed"
	if observed {
		changed = "was changed"
	}
	note := "changes_expected reconciled to repository reality: the agent reported " +
		strconv.FormatBool(reported) + " but the repository " + changed
	if strings.TrimSpace(summary) == "" {
		return note
	}
	return strings.TrimSpace(summary) + " [" + note + "]"
}
