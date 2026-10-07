package ollamaagent

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
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

	// consecutiveNoProgress counts stale turns. Successful novel discovery -- a
	// first-seen inspection or a first-seen successful non-mutating command --
	// resets it only within the initial discovery window; mutation resets it
	// separately.
	consecutiveNoProgress int

	// discoverySeen contains successful discovery identities (inspections and
	// non-mutating commands) credited during this invocation. It is independent of
	// attempted-path checkpoints and mutation evidence, and grows only within
	// implementNowAfter model turns.
	discoverySeen map[inspectionIdentity]bool

	// repositoryMutations counts the successful controlled mutations this invocation
	// performed. It is the authoritative progress signal and is reported in the
	// no-progress diagnostic; a narrative claim never increments it.
	repositoryMutations int

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
	st.consecutiveNoProgress = 0
	st.repositoryMutations++
}

// observeDiscovery credits a first-seen successful discovery action -- a file
// inspection or a non-mutating command -- only within the initial model-turn
// window. Narration, failures, denied calls, and repeats cannot extend that window.
func (st *executionState) observeDiscovery(iteration int, root, name string, args map[string]any, result string, err error) bool {
	if st.mutationObserved || iteration > implementNowAfter {
		return false
	}
	identity, ok := discoveryIdentity(root, name, args, result, err)
	if !ok || st.discoverySeen[identity] {
		return false
	}
	if st.discoverySeen == nil {
		st.discoverySeen = make(map[inspectionIdentity]bool)
	}
	st.discoverySeen[identity] = true
	return true
}

// stalled stops unmutated runs after five stale turns. Discovery may reset the
// streak but never sets mutationObserved. After mutation, the existing CHANGE /
// finalization lifecycle and repetition guard continue to govern execution.
func (st *executionState) stalled(mutated, discovered bool, staleLimit int) bool {
	if mutated || discovered {
		st.consecutiveNoProgress = 0
		return false
	}
	st.consecutiveNoProgress++
	return !st.mutationObserved && st.consecutiveNoProgress >= staleLimit
}

// mutationConvergenceRequired reports whether the bounded discovery window has
// closed with no observed mutation, so non-mutating repository tools must now be
// denied and the run must converge on a mutation attempt or a truthful terminal
// outcome.
//
// The enforcement opens strictly BEFORE the stale bound. An unmutated run is
// terminated by the stale guard at implementNowAfter + staleIterations turns, so
// an enforcement that fired only once the stale streak had already reached
// staleIterations could never affect execution (the loop returns first). Firing as
// soon as the discovery window closes (iteration > implementNowAfter) leaves the
// model the remaining turns up to the stale bound in which to make the change --
// mutation tools stay available -- or to return a truthful needs_human/failed
// outcome; a run that still keeps requesting non-mutating tools is stopped by the
// unchanged stale guard. Derived entirely from the existing window: no new counter,
// no new configurable threshold, no budget knob.
func (st *executionState) mutationConvergenceRequired(iteration int) bool {
	if st.mutationObserved {
		return false
	}
	// The enforcement opens only after the bounded discovery window has closed
	// (iteration > implementNowAfter) AND the run has carried its non-mutating
	// activity one turn further (a stale turn has accrued). This threshold is
	// strictly below the stale bound (staleIterations), so the denial runs before
	// the stale guard terminates the stage and the model still has turns in which
	// to make the change or return a truthful outcome. A first post-window turn
	// that immediately mutates is therefore never denied.
	if iteration <= implementNowAfter {
		return false
	}
	return st.consecutiveNoProgress >= 1
}

// nonMutatingRepositoryTool reports whether name is a repository tool that does
// not change the repository: a file inspection or an inspection-class command.
// It reuses the existing controlledMutation/commandMutates classification rather
// than hard-coding a second mutation list, so the enforcement set stays derived.
func nonMutatingRepositoryTool(name string, args map[string]any) bool {
	if controlledMutation(name, args) {
		return false
	}
	switch name {
	case toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles:
		return true
	case toolharness.ToolRunCommand:
		command, _ := args["command"].(string)
		return !commandMutates(command)
	}
	return false
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
