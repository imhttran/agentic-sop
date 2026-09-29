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
// the model's changes_expected claim with the repository change the invocation
// actually produced, so a run cannot claim to have changed the repository when it
// did not (nor deny one it did).
//
// The primary execution signal is the invocation's own mutation evidence: did
// this invocation perform a successful controlled mutation? The reconciliation
// evidence is the working tree's divergence from the invocation's baseline
// fingerprint (ChangedSince), which can confirm or contradict the mutation
// signal. Observed reality overrides the model's claim in every case.
//
// The reference point is the invocation's own baseline fingerprint (captured
// before the capability ran), not HEAD: in a pre-dirty working tree, changes
// that existed before the invocation are not attributed to it. The baseline, the
// mutation evidence, and the mismatch flag are all local to one Complete call —
// nothing is stored on the Harness, so one invocation cannot contaminate the
// next.
//
// IMPLEMENT/FIX results MUST be emitted as structured outcomes; prose responses
// are wrapped in a completed outcome so SOP can act on them deterministically.

// reconcileOutcome grounds a completed IMPLEMENT/FIX outcome in observable
// repository reality. The invocation's mutation evidence is the primary signal;
// whether the working tree changed from the invocation baseline is reconciliation
// evidence that confirms or contradicts it. The observed truth wins over the
// model's reported changes_expected, and a disagreement is reported (not silently
// accepted) through both the summary note and the returned flag.
//
// A non-completed outcome (needs_human, failed) carries no changes_expected and
// is returned unchanged. Content that is not a recognized outcome is returned
// unchanged, so a legacy prose or document response is never rewritten. When the
// working tree cannot be inspected AND there is no mutation evidence — no
// baseline was taken, or Git fails — the model's claim is left as reported.
func (h *Harness) reconcileOutcome(ctx context.Context, baseline string, ev *mutationEvidence, content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || trimmed[0] != '{' {
		return content, false
	}
	var wire outcomeWire
	if err := json.Unmarshal([]byte(trimmed), &wire); err != nil {
		return content, false
	}
	switch wire.Status {
	case string(agent.OutcomeCompleted), string(agent.OutcomeNeedsHuman), string(agent.OutcomeFailed):
	default:
		return content, false
	}

	// Only a completed outcome asserts a repository change; needs_human and
	// failed outcomes are left exactly as the model reported them.
	if wire.Status != string(agent.OutcomeCompleted) {
		return content, false
	}

	observed, known := observedRepositoryChange(ctx, h, baseline, ev)
	if !known {
		// No observed signal at all (no baseline and no mutation evidence): leave
		// the model's claim untouched rather than inventing reality.
		return content, false
	}

	reported := true
	if wire.ChangesExpected != nil {
		reported = *wire.ChangesExpected
	}
	wire.ChangesExpected = &observed

	mismatch := reported != observed
	if mismatch {
		// Keep the model's claim visible without changing SOP's outcome vocabulary:
		// the note explains why changes_expected was overridden by observation.
		wire.Summary = appendMismatchNote(wire.Summary, reported, observed)
	}

	data, err := json.Marshal(wire)
	if err != nil {
		return content, mismatch
	}
	return string(data), mismatch
}

// observedRepositoryChange reports whether this invocation changed the repository,
// and whether that can be determined at all. Mutation evidence is the primary
// signal: a successful controlled mutation is a change, regardless of what the
// working tree looks like. The working tree's divergence from the invocation
// baseline is reconciliation evidence: it confirms a mutation, or reveals a
// change the model made without a classified mutation, but it can never
// contradict a positive mutation signal. When there is no mutation evidence and
// the working tree cannot be inspected (no baseline, or Git fails), the result is
// unknown, so the model's claim is left untouched.
func observedRepositoryChange(ctx context.Context, h *Harness, baseline string, ev *mutationEvidence) (observed, known bool) {
	if ev != nil && ev.observed {
		return true, true
	}
	if baseline == "" {
		return false, false
	}
	changed, err := h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		return false, false
	}
	return changed, true
}

// retryNoChangeFailure rewrites a model-reported `failed` outcome into a
// retryable `needs_human` one when the working tree did not change during this
// invocation. A mutating capability that changed nothing did not actually
// attempt the work, so a retry is warranted (bounded by max_attempts) rather
// than a hard failure — the same classification the harness gives its own
// no-change exhaustion. A `failed` outcome with an observed change is a real
// failure and is left as reported; so is a non-failed outcome, one that cannot
// be reconciled to a baseline, or one whose working tree cannot be inspected.
func (h *Harness) retryNoChangeFailure(ctx context.Context, baseline string, ev *mutationEvidence, content string) string {
	var wire outcomeWire
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &wire); err != nil {
		return content
	}
	if wire.Status != string(agent.OutcomeFailed) {
		return content
	}
	observed, known := observedRepositoryChange(ctx, h, baseline, ev)
	if !known || observed {
		return content
	}
	wire.Status = string(agent.OutcomeNeedsHuman)
	// This note is a deterministic, machine-recognizable marker: failure.Classify
	// keys on it (see harnessIncompleteMarkers) so a no-change outcome is classified
	// CONTINUE even when the model's own explanation mentions a human-boundary word.
	// Keep the two in sync.
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

// ensureStructuredOutcome ensures IMPLEMENT/FIX results are emitted as structured
// outcomes by wrapping prose responses in a completed outcome. This enforces that
// SOP receives a deterministic JSON shape it can parse and act on, and that
// changes_expected is derived from observed reality (invocation mutation evidence
// confirmed by the working-tree change since the baseline), not model claims.
func (h *Harness) ensureStructuredOutcome(ctx context.Context, baseline string, ev *mutationEvidence, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		trimmed = "{}"
	}

	// If the content is already a recognized structured outcome, return it
	// (after reconciliation, which was already done upstream).
	if trimmed[0] == '{' {
		var wire outcomeWire
		if err := json.Unmarshal([]byte(trimmed), &wire); err == nil {
			switch wire.Status {
			case string(agent.OutcomeCompleted), string(agent.OutcomeNeedsHuman), string(agent.OutcomeFailed):
				return content
			}
		}
	}

	// Prose response: wrap it in a completed outcome.
	// The summary captures what the model reported; changes_expected is set
	// from the repository change observed during this invocation so the harness
	// report is always truthful. When the repository cannot be inspected and there
	// is no mutation evidence, conservatively assume a change was intended (the
	// stricter choice, and the default when the model omits the field).
	observed, known := observedRepositoryChange(ctx, h, baseline, ev)
	if !known {
		observed = true
	}

	wire := outcomeWire{
		Status:          string(agent.OutcomeCompleted),
		Summary:         trimmed,
		ChangesExpected: &observed,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		// Should never happen for a simple struct with basic types.
		// Fallback: emit the prose as the summary of a completed outcome.
		wire.Summary = content
		data, _ = json.Marshal(wire)
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
