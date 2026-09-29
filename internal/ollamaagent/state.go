package ollamaagent

import (
	"fmt"
	"strings"
)

// Invocation-scoped execution state.
//
// A Harness is reused across invocations, so every piece of execution evidence
// that describes *one* invocation — mutation observation, phase, counters,
// no-progress fingerprints, and finalization state — must live in a fresh
// executionState created at the start of that invocation. Nothing here is stored
// on the Harness, so invocation A can never contaminate invocation B.
//
// This is deliberately NOT SOP workflow state: nothing is persisted, and the
// Harness never reads or writes .agent-sdlc/state.db.

// executionState is the complete invocation-scoped execution state of one
// capability run. A zero value is a valid, empty state for a new invocation;
// newExecutionState returns a fresh, fully independent instance.
type executionState struct {
	// mutationObserved records the mutation observation of this invocation: a
	// successful controlled mutation was applied to the repository. A failed or
	// denied mutation never sets it.
	mutationObserved bool

	// phase is the invocation's lifecycle phase (for example IMPLEMENT's
	// DISCOVER/CHANGE/FINALIZE).
	phase implementPhase

	// counters are the invocation's interaction counters.
	counters executionCounters

	// progress tracks the invocation's no-progress fingerprints.
	progress turnProgress

	// finalization is the invocation's finalization state.
	finalization finalizationState

	// inspected lists the repository paths this invocation inspected (read,
	// listed, or searched), first-seen, deduplicated, and bounded. It is the
	// deterministic half of a continuation checkpoint: a run that stops short of
	// changing the repository hands the next invocation the paths it already looked
	// at, so the next bounded invocation resumes instead of repeating discovery.
	// It holds paths only — never file contents, prompts, or secrets.
	inspected []string
}

// maxCheckpointFiles bounds the continuation checkpoint, so a long discovery run
// cannot grow the recorded context without limit.
const maxCheckpointFiles = 12

// maxCheckpointShown bounds how many inspected paths the diagnostic renders; the
// remainder is summarized as a count.
const maxCheckpointShown = 5

// executionCounters are the invocation's interaction counters. They measure work
// done in one invocation and are never carried across invocations.
type executionCounters struct {
	// interactions counts tool interactions executed this invocation.
	interactions int
	// toolCalls counts controlled tool calls executed this invocation.
	toolCalls int
	// sinceMutation counts interactions since the last successful mutation. A
	// model still writing keeps this small, so a multi-file change is never
	// finalized mid-write.
	sinceMutation int
}

// finalizationState is the invocation's finalization state: whether finalization
// has been entered, and how many model turns it has consumed.
type finalizationState struct {
	// entered is true once the invocation has withdrawn its tools to finalize.
	entered bool
	// turns counts model turns consumed in FINALIZE.
	turns int
	// nudged is true once the discovery nudge has been sent.
	nudged bool
	// implementInstructed is true once the implement-now instruction has been
	// sent to an unmutated run.
	implementInstructed bool
}

// newExecutionState returns a fresh, empty execution state for one invocation.
// Each call returns an independent value: two states share no mutable references,
// so observing a mutation, advancing a phase, or accumulating a no-progress
// fingerprint in one invocation cannot affect another.
func newExecutionState() *executionState {
	return &executionState{phase: implDiscover}
}

// observeMutation records a successful controlled mutation during this
// invocation. A failed or denied mutation must not be observed, so a caller
// gates this on the tool call succeeding.
func (st *executionState) observeMutation() {
	st.mutationObserved = true
	st.counters.sinceMutation = 0
}

// countNonMutatingInteraction advances the invocation's counters after a tool
// interaction that did not change the repository.
func (st *executionState) countNonMutatingInteraction() {
	st.counters.interactions++
	if st.mutationObserved {
		st.counters.sinceMutation++
	}
}

// recordInspected adds a repository path to the invocation's continuation
// checkpoint. It is first-seen ordered, deduplicated, and bounded, so it stays a
// compact summary rather than a transcript.
func (st *executionState) recordInspected(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, seen := range st.inspected {
		if seen == path {
			return
		}
	}
	if len(st.inspected) >= maxCheckpointFiles {
		return
	}
	st.inspected = append(st.inspected, path)
}

// inspectedSummary renders the checkpoint's inspected paths for the no-change
// diagnostic: the first few, then a count of the rest. It returns "" when nothing
// was inspected, so a run that read no files adds nothing to the message.
func (st *executionState) inspectedSummary() string {
	n := len(st.inspected)
	if n == 0 {
		return ""
	}
	shown := n
	if shown > maxCheckpointShown {
		shown = maxCheckpointShown
	}
	summary := strings.Join(st.inspected[:shown], ", ")
	if n > shown {
		summary += fmt.Sprintf(" (+%d more)", n-shown)
	}
	return summary
}

// finalizeEligible reports whether the phased loop may withdraw its tools.
// Withdrawing requires an observed mutation — a count alone is not evidence the
// work is done — and that the model has stopped mutating: a model that is still
// writing has not finished, and finalizing it would refuse the very write it
// still needs. A run that never changes the repository has no writer to wait for,
// so it is finalized at the late stage.
func (st *executionState) finalizeEligible(iteration int, policy CapabilityPolicy) bool {
	if st.mutationObserved && iteration >= policy.ForceFinalizeAfter {
		return true
	}

	if st.counters.interactions < policy.FinalizeAfter {
		return false
	}

	if st.mutationObserved {
		return st.counters.sinceMutation >= policy.CompletionWindow
	}

	return st.counters.interactions >= policy.LateStageAfter
}
