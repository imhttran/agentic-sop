package ollamaagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Shared capability orchestration.
//
// This file owns the two lifecycle engines that more than one capability runs:
// the document-producing two-phase engine (PLAN, REVIEW) and the mutating
// three-phase engine (IMPLEMENT, FIX). Capability-specific bounds, labels, and
// instruction text do NOT live here: each engine is parameterized by the values
// its capability file supplies (plan.go, review.go) or by the centralized policy
// (policy.go). harness.go keeps only dispatch plus the common infrastructure.

// --- Two-phase engine (document-producing capabilities) ---
const maxSynthesisCorrections = 2

// twoPhase bounds one document-producing capability's loop: a bounded read-only
// discovery phase, then a tool-free synthesis phase that must produce the
// document. PLAN (plan.go) and REVIEW (review.go) share this orchestration and
// differ only in these values. A nudgeAfter of 0 disables the soft wrap-up
// nudge.
type twoPhase struct {
	discoveryTurns int
	synthesisTurns int
	// nudgeAfter is the discovery-turn count at which the model is told,
	// softly, to wrap up inspection; nothing is disabled by the nudge.
	nudgeAfter  int
	nudge       string
	instruction string
	correction  string
	// discoverLabel and synthLabel name the phases for the trace and
	// diagnostics (PLAN: DISCOVERY/SYNTHESIS; REVIEW: INSPECT/SYNTHESIZE).
	discoverLabel string
	synthLabel    string
}

// executeTwoPhase runs the bounded discovery → tool-free synthesis loop shared by
// the document-producing capabilities (PLAN and REVIEW). Discovery permits at most
// the capability's discovery budget of turns (each at most one tool) with the
// centralized read-only tool policy. When discovery is exhausted without a final
// response, tools are withdrawn and the model is told to synthesize; synthesis
// permits at most the capability's synthesis budget of model turns. A final
// response ends the invocation immediately, in either phase, so early completion
// is preserved.
//
// All phase/counter state below belongs to this single invocation; none of it is
// stored on the shared Harness.
func (h *Harness) executeTwoPhase(ctx context.Context, req agent.Request, tp twoPhase) (string, error) {
	policy := PolicyFor(req.Capability) // read-only tools
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req, policy)},
		{Role: "user", Content: userPrompt(req)},
	}

	var (
		synthesis        bool // the synthesis phase has begun
		discovery        int  // discovery turns taken (each executes at most one tool)
		discovered       int  // discovery tools actually executed
		synth            int  // actual synthesis turns taken
		synthCorrections int  // denied synthesis tool requests
		nudged           bool // the soft wrap-up nudge has been sent
	)

	for {
		if synthesis && synth >= tp.synthesisTurns {
			h.trace.Record(TraceRecord{
				Capability:  string(req.Capability),
				Phase:       tp.synthLabel,
				Iteration:   synth,
				Termination: terminationSynthesis,
			})
			return "", h.synthesisLimitError(req, tp, discovered, synth)
		}

		raw, calls, err := h.chat(ctx, messages)
		if err != nil {
			return "", err
		}
		name, args, isTool, final, err := turnToolCall(raw, calls)
		if err != nil && !errors.Is(err, errNarrate) {
			return "", err
		}

		switch {
		case err != nil: // narration: no tool call and no final object
			if !synthesis {
				discovery++
				h.recordTwoPhaseTurn(req, tp.discoverLabel, discovery, "narrate", "")
				messages = append(messages,
					chatMessage{Role: "assistant", Content: raw},
					chatMessage{Role: "user", Content: turnReminder},
				)
				if discovery >= tp.discoveryTurns {
					synthesis = true
					messages = append(messages, chatMessage{Role: "user", Content: tp.instruction})
					h.recordSynthesisTransition(req, tp)
				}
			} else {
				synth++
				h.recordTwoPhaseTurn(req, tp.synthLabel, synth, "narrate", "")
				messages = append(messages,
					chatMessage{Role: "assistant", Content: raw},
					chatMessage{Role: "user", Content: turnReminder},
				)
			}

		case !isTool: // a final response ends the invocation immediately
			label, index := tp.discoverLabel, discovery+1
			if synthesis {
				label, index = tp.synthLabel, synth+1
			}
			h.recordTwoPhaseTurn(req, label, index, "final", "")
			encoded, mErr := json.Marshal(final)
			if mErr != nil {
				return "", fmt.Errorf("encode final response: %w", mErr)
			}
			return string(encoded), nil

		case synthesis: // no tools during synthesis: deny and correct
			synthCorrections++

			h.recordTwoPhaseTurn(
				req,
				tp.synthLabel,
				synthCorrections,
				name,
				toolharness.SummarizeRequest(name, args),
			)

			h.tools.RecordDenied(
				name,
				args,
				"tools are unavailable during synthesis",
			)

			if synthCorrections > maxSynthesisCorrections {
				return "", fmt.Errorf(
					"the Ollama agent %s repeatedly requested tools during %s (model=%s, synthesis_corrections=%d, termination=synthesis_correction_limit)",
					req.Capability,
					strings.ToLower(tp.synthLabel),
					h.cfg.Model,
					synthCorrections,
				)
			}

			messages = append(
				messages,
				chatMessage{
					Role:    "assistant",
					Content: assistantEcho(name, args, raw),
				},
				chatMessage{
					Role:    "user",
					Content: tp.correction,
				},
			)

		default: // discovery
			discovery++
			summary := toolharness.SummarizeRequest(name, args)
			var detail string
			if !policy.Allows(name) {
				denied := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
				h.tools.RecordDenied(name, args, denied)
				h.recordTwoPhaseTurn(req, tp.discoverLabel, discovery, name, summary)
				detail = toolResultMessage(name, "", errors.New(denied))
			} else {
				result, toolErr := h.tools.Run(ctx, name, args)
				discovered++
				recordToolActivity(ctx, name, args)
				h.recordTwoPhaseTurn(req, tp.discoverLabel, discovery, name, summary)
				detail = toolResultMessage(name, result, toolErr)
			}
			// The soft wrap-up nudge rides on the tool result so the transcript
			// keeps alternating assistant/user turns; it disables nothing.
			if tp.nudgeAfter > 0 && !nudged && discovery >= tp.nudgeAfter {
				nudged = true
				if detail != "" {
					detail += "\n\n" + tp.nudge
				} else {
					detail = tp.nudge
				}
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: detail},
			)
			if discovery >= tp.discoveryTurns {
				synthesis = true
				messages = append(messages, chatMessage{Role: "user", Content: tp.instruction})
				h.recordSynthesisTransition(req, tp)
			}
		}
	}
}

// recordTwoPhaseTurn appends one safe trace entry for a two-phase turn.
func (h *Harness) recordTwoPhaseTurn(req agent.Request, phase string, index int, tool, request string) {
	h.trace.Record(TraceRecord{
		Capability: string(req.Capability),
		Phase:      phase,
		Iteration:  index,
		Tool:       tool,
		Request:    request,
		Progress:   progressOK,
	})
}

// recordSynthesisTransition marks the discovery-to-synthesis transition in the trace.
func (h *Harness) recordSynthesisTransition(req agent.Request, tp twoPhase) {
	h.trace.Record(TraceRecord{Capability: string(req.Capability), Event: "→ " + tp.synthLabel})
}

// synthesisLimitError reports that a document-producing capability exhausted its
// tool-free synthesis turns without producing a document. It is deliberately
// phase-specific rather than the generic iteration-limit message.
func (h *Harness) synthesisLimitError(req agent.Request, tp twoPhase, discovered, synth int) error {
	return fmt.Errorf("the Ollama agent %s failed during %s (model=%s, %s=%d, %s=%d, termination=%s)",
		req.Capability, strings.ToLower(tp.synthLabel), h.cfg.Model,
		strings.ToLower(tp.discoverLabel)+"_tool_calls", discovered,
		strings.ToLower(tp.synthLabel)+"_turns", synth, terminationSynthesis)
}

// --- Phased engine (mutating capabilities: IMPLEMENT and FIX) ---

// IMPLEMENT and FIX share a three-phase loop: bounded DISCOVERY (read the
// repository just enough to make the change), CHANGE (make the change and
// targeted checks), then FINALIZE (no repository tools; return the structured SOP
// outcome). Phase state is scoped to a single invocation — it is not SOP
// workflow state and is never persisted.
//
// Capability-specific policy text and instructions live in the capability's own
// file (implement.go); FIX enters the same engine through its own seam (fix.go).
// The bounds themselves are centralized in policy.go.
type implementPhase int

const (
	implDiscover implementPhase = iota
	implChange
	implFinalize
)

// label renders the phase for the trace and diagnostics.
func (p implementPhase) label() string {
	switch p {
	case implChange:
		return "CHANGE"
	case implFinalize:
		return "FINALIZE"
	default:
		return "DISCOVER"
	}
}

// executePhased runs the three-phase loop shared by the phased mutating
// capabilities (IMPLEMENT and FIX). Discovery and change may use the controlled
// tools; once the model has changed the repository and then stopped writing for
// the completion window those tools are withdrawn and the model must return the
// structured outcome. A model that is still writing keeps its tools —
// finalization must not truncate a multi-file change — while FINALIZE itself is
// terminal: every tool request there, mutation included, is denied. A threshold
// crossed without a mutation does not finalize: the model is told to implement
// and keeps its tools. That steering recurs on every interaction past
// implementNowAfter (a one-shot instruction can be outrun), and becomes closing
// just before the late-stage cutoff, so an unmutated run is warned that the
// invocation is about to end while it can still write. A final response ends the
// invocation in any phase, so early completion is preserved, and the capability's
// MaxIterations stays the hard safety ceiling.
//
// A mutated run is force-finalized once it reaches the capability's
// ForceFinalizeAfter, evaluated before the next model turn so the cutoff does not
// depend on the shape of the turn that crosses it: a model that keeps writing (or
// narrating) around the threshold still stops mutating and is left a full
// FinalizeTurns window before the ceiling. A mutated run that exhausts the ceiling
// inside FINALIZE is reported as a finalization failure, never the generic
// iteration limit.
//
// All mutation observation, phase, counters, no-progress fingerprints, and
// finalization state live in a fresh invocation-scoped executionState created
// here, so one invocation cannot contaminate another even though the Harness is
// reused. Each successful controlled mutation is also recorded in ev — the
// invocation's mutation evidence, owned by the caller's Complete call — so the
// caller can ground changes_expected on observed reality rather than on a model
// claim.
//
// The no-progress guard permits novel successful inspection through the existing
// implement-now threshold, then requires mutation within five stale turns. It
// counts all model turns toward the discovery window, so narration and denials
// cannot extend it. Repetitive exploration can terminate sooner.
func (h *Harness) executePhased(ctx context.Context, req agent.Request, ev *mutationEvidence) (string, error) {
	policy := PolicyFor(req.Capability) // all tools; IMPLEMENT may mutate
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req, policy)},
		{Role: "user", Content: userPrompt(req)},
	}

	st := newExecutionState()
	var (
		lastTool string
		lastReq  string
	)

	for iteration := 1; iteration <= policy.MaxIterations; iteration++ {
		// FINALIZE has a small turn allowance: a model that keeps asking for tools
		// after implementation is complete is stopped with a phase-specific reason
		// rather than the generic iteration limit.
		if st.phase == implFinalize && st.finalization.turns >= policy.FinalizeTurns {
			h.trace.Record(TraceRecord{
				Capability:  string(req.Capability),
				Phase:       st.phase.label(),
				Iteration:   iteration,
				Termination: terminationFinalization,
			})
			if !st.mutationObserved {
				// Unreachable for a zero-mutation run: the no-progress guard (stalled) stops it
				// long before FINALIZE's FinalizeTurns allowance is spent, and entering FINALIZE
				// without a mutation needs the late-stage point (implementLateStageAfter), which
				// such a run never reaches. Kept for the mutated path and as a defensive
				// fallback if maxNoProgressIterations is configured above the late-stage point.
				return "", h.noChangeError(req, policy, st, lastTool, lastReq)
			}
			return "", h.finalizeLimitError(req, st, lastTool, lastReq)
		}

		// Force-finalization reserves the closing turns for a tool-free outcome.
		// Once a mutated run reaches the force threshold it enters FINALIZE before
		// its next model turn, whatever shape the turn that crossed the threshold had.
		// Evaluating it here — not only after a tool call — is what makes the cutoff
		// independent of a narration (or a denied request) landing on the threshold:
		// a continuously writing FIX can no longer consume the ceiling in CHANGE, and
		// FINALIZE always keeps a full FinalizeTurns allowance before the ceiling.
		if st.phase != implFinalize && policy.ForceFinalizeAfter > 0 &&
			st.mutationObserved && iteration >= policy.ForceFinalizeAfter {
			st.phase = implFinalize
			st.finalization.entered = true
			recordFinalizeActivity(ctx)
			h.recordImplementEvent(req, implementFinalizeEvent, "")
			messages = append(messages, chatMessage{Role: "user", Content: implementFinalizeInstruction})
		}

		raw, calls, err := h.chat(ctx, messages)
		if err != nil {
			return "", err
		}
		name, args, isTool, final, err := turnToolCall(raw, calls)
		if err != nil {
			if !errors.Is(err, errNarrate) {
				return "", err
			}
			if st.phase == implFinalize {
				st.finalization.turns++
			}
			recovery, repeatStop := st.progress.observe(narrationFingerprint(req.Capability))
			progressStop := st.stalled(false, false)
			terminate := repeatStop || progressStop
			h.recordImplementTurn(req, st.phase, iteration, "narrate", "", st.progress.label(), recovery, terminate)
			if terminate {
				if progressStop {
					return "", h.implementNoProgressError(req, st, ev, iteration, "narrate", "")
				}
				return "", h.noProgressError(req, iteration, "narrate", "")
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: raw},
				chatMessage{Role: "user", Content: nudgeText(recovery)},
			)
			continue
		}
		if !isTool {
			h.recordImplementTurn(req, st.phase, iteration, "outcome", "", progressOK, false, false)
			encoded, err := json.Marshal(final)
			if err != nil {
				return "", fmt.Errorf("encode final response: %w", err)
			}
			return string(encoded), nil
		}

		// FINALIZE is terminal for this invocation. Repository work is complete and
		// all tool requests are denied, including mutation tools. This preserves a
		// bounded tool-free window in which the model must return SOP_OUTCOME.
		if st.phase == implFinalize {
			st.finalization.turns++
			lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)

			h.tools.RecordDenied(
				name,
				args,
				"tools are unavailable during IMPLEMENT finalization",
			)

			h.recordImplementTurn(
				req,
				st.phase,
				iteration,
				name,
				lastReq,
				progressDenied,
				false,
				false,
			)

			messages = append(
				messages,
				chatMessage{
					Role:    "assistant",
					Content: assistantEcho(name, args, raw),
				},
				chatMessage{
					Role:    "user",
					Content: implementFinalizeCorrection,
				},
			)

			continue
		}
		// Capability policy: keep the invariant that a capability only reaches the
		// tools its policy allows, even though IMPLEMENT currently allows them all.
		if !policy.Allows(name) {
			detail := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
			lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)
			h.tools.RecordDenied(name, args, detail)
			recovery, repeatStop := st.progress.observe(actionFingerprint(name, args, detail, nil))
			progressStop := st.stalled(false, false)
			terminate := repeatStop || progressStop
			h.recordImplementTurn(req, st.phase, iteration, name, lastReq, st.progress.label(), recovery, terminate)
			if terminate {
				if progressStop {
					return "", h.implementNoProgressError(req, st, ev, iteration, lastTool, lastReq)
				}
				return "", h.noProgressError(req, iteration, lastTool, lastReq)
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: toolResultMessage(name, "", errors.New(detail)) + recoverySuffix(recovery)},
			)
			continue
		}

		if st.counters.toolCalls >= h.cfg.MaxToolCalls {
			return "", fmt.Errorf("tool-call limit reached (%d tool calls); the model did not finish", h.cfg.MaxToolCalls)
		}
		st.counters.toolCalls++

		candidate := controlledMutation(name, args)
		var result string
		var toolErr error
		var observation toolharness.MutationObservation
		if candidate {
			result, observation, toolErr = h.tools.RunObservedMutation(ctx, name, args)
			if !observation.Verified {
				h.recordImplementEvent(req, "mutation observation", "verification_unavailable")
				result += "\nmutation verification unavailable; no mutation progress credited"
			} else if observation.Changed && !observation.Succeeded {
				h.recordImplementEvent(req, "mutation observation", "failed_operation_repository_changed")
				result += "\nrepository state changed during failed operation; no successful mutation progress credited"
			}
		} else {
			result, toolErr = h.tools.Run(ctx, name, args)
		}
		lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)

		// An inspection is remembered in the invocation's continuation checkpoint,
		// so a run that stops short of changing the repository can hand the next
		// invocation the paths it already looked at. A failed read is recorded too:
		// the intent to inspect that path is the useful signal, and the recorded
		// value is the path alone.
		if path, ok := checkpointPath(name, args); ok {
			st.recordInspected(path)
		}

		// A successful, independently verified mutation is the execution evidence: it
		// moves the invocation out of discovery and grounds changes_expected. A
		// failed or denied write never counts, and neither does a read, a search,
		// an inspection, or a successful operation that left repository state unchanged.
		justMutated := candidate && toolErr == nil && observation.Succeeded && observation.Verified && observation.Changed
		// CHANGE activity is later consumed as task change evidence. No-op,
		// failed, or unverified file attempts remain audited but must not emit it.
		if !candidate || justMutated || name == toolharness.ToolRunCommand {
			recordToolActivity(ctx, name, args)
		}
		if justMutated {
			st.observeMutation()
			st.counters.interactions++
			if ev != nil {
				ev.record(name)
				if path, ok := mutationPath(name, args); ok {
					ev.recordPath(path)
				}
			}
		} else {
			st.countNonMutatingInteraction()
		}

		recovery, repeatStop := st.progress.observe(actionFingerprint(name, args, result, toolErr))
		discovered := st.observeDiscovery(iteration, h.tools.Root(), name, args, result, toolErr)
		progressStop := st.stalled(justMutated, discovered)
		terminate := repeatStop || progressStop
		if justMutated && st.phase == implDiscover {
			st.phase = implChange
			h.recordImplementEvent(req, implementChangeEvent, "")
		}
		h.recordImplementTurn(req, st.phase, iteration, name, lastReq, st.progress.label(), recovery, terminate)
		if terminate {
			if progressStop {
				return "", h.implementNoProgressError(req, st, ev, iteration, lastTool, lastReq)
			}
			return "", h.noProgressError(req, iteration, lastTool, lastReq)
		}

		// Phase guidance rides on the tool result so turns keep alternating.
		var advice []string
		if recovery {
			advice = append(advice, progressReminder)
		}
		switch {
		case justMutated:
			advice = append(advice, implementChangeInstruction)
		case st.phase == implDiscover && !st.finalization.nudged && st.counters.interactions >= implementNudgeAfter:
			// Successful bounded discovery can reach this nudge without mutation.
			st.finalization.nudged = true
			advice = append(advice, implementNudge)
		}
		// Finalization is mutation-aware: crossing the threshold alone is not enough.
		// Without a mutation the tools stay enabled and the model is pushed to
		// implement, so a run is never finalized before it has changed anything.
		if st.phase != implFinalize {
			switch {
			case st.finalizeEligible(iteration, policy):
				st.phase = implFinalize
				st.finalization.entered = true
				recordFinalizeActivity(ctx)
				if st.mutationObserved {
					advice = append(advice, implementFinalizeInstruction)
				} else {
					// Unreachable: finalizeEligible without a mutation needs the late-stage point
					// (implementLateStageAfter), which the no-progress guard stops an unmutated run
					// before. Kept as the documented intent.
					advice = append(advice, implementFinalInstruction)
				}
				h.recordImplementEvent(req, implementFinalizeEvent, "")
			case !st.mutationObserved && st.counters.interactions >= implementNowAfter:
				// Bounded discovery can reach implement-now; subsequent inspections no
				// longer earn discovery credit and tools remain available for mutation.
				// An unmutated run is steered on EVERY interaction past the implement-now
				// threshold, not once. A single instruction is easy to outrun: the model
				// reads on and the instruction is many turns stale by the time the run
				// finalizes. Re-stating it keeps the steering at most one turn old, and the
				// interactions immediately before the late-stage cutoff become closing, so
				// the model is told the consequence of not writing while it can still
				// write. The transition is still recorded once, as the phase event, so the
				// trace is unchanged.
				if st.counters.interactions >= implementClosingAfter {
					advice = append(advice, implementClosingInstruction)
				} else {
					advice = append(advice, implementNowInstruction)
				}
				if !st.finalization.implementInstructed {
					st.finalization.implementInstructed = true
					h.recordImplementEvent(req, implementContinueEvent, "implementation required before finalization")
				}
			}
		}

		content := toolResultMessage(name, result, toolErr)
		for _, a := range advice {
			content += "\n\n" + a
		}
		messages = append(messages,
			chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
			chatMessage{Role: "user", Content: content},
		)
	}
	if !st.mutationObserved {
		// Normally the discovery window plus stale bound stops an unmutated run
		// first. A configured ceiling below that total still uses this fallback.
		return "", h.noChangeError(req, policy, st, lastTool, lastReq)
	}
	// A mutated run that reached the ceiling still inside FINALIZE never returned
	// its outcome: that is a finalization failure, not the generic iteration limit,
	// even when FINALIZE was entered late (for example by a mutation that only
	// arrived in the closing turns).
	if st.phase == implFinalize {
		return "", h.finalizeLimitError(req, st, lastTool, lastReq)
	}
	return "", h.implementExhaustedError(req, policy, st, lastTool, lastReq)
}

// recordImplementTurn appends one safe trace entry for a phased turn.
func (h *Harness) recordImplementTurn(req agent.Request, phase implementPhase, iteration int, tool, request, progress string, recovery, terminate bool) {
	rec := TraceRecord{
		Capability: string(req.Capability),
		Phase:      phase.label(),
		Iteration:  iteration,
		Tool:       tool,
		Request:    request,
		Progress:   progress,
		Recovery:   recovery,
	}
	if terminate {
		rec.Termination = terminationNoProgress
	}
	h.trace.Record(rec)
}

// recordImplementEvent marks a phase transition or a completion-policy decision in
// the trace. A non-empty detail carries a short, secret-free reason so a run can
// be understood after the fact.
func (h *Harness) recordImplementEvent(req agent.Request, event, detail string) {
	h.trace.Record(TraceRecord{Capability: string(req.Capability), Event: event, Detail: detail})
}

// terminationNoChange marks a phased run that ended without changing the
// repository. With the repository no-progress guard in place this is the fallback
// path: a zero-mutation run is normally stopped earlier with terminationNoProgress
// (see executePhased). It remains reachable only when policy.MaxIterations is
// configured below the discovery window plus stale bound.
const terminationNoChange = "no_change"

// changeIncompleteError reports that a phased mutating capability ended without
// changing the repository: the agent never acted, so the right response is to try
// again (a retryable human boundary), not to record the work as failed. It is now
// the fallback path — a zero-mutation run is normally stopped by the no-progress
// guard with noProgressError instead (see executePhased).
type changeIncompleteError struct{ msg string }

func (e *changeIncompleteError) Error() string { return e.msg }

// noProgressError reports that a phased mutating capability spent consecutive
// iterations without changing the repository and was stopped early, well before
// its iteration ceiling. Like changeIncompleteError it is a resumable
// continuation (retry, do not block), but the diagnostic is specific: the run did
// not stop at the ceiling and did not loop on a repeated action — it made no
// repository progress at all. The model's narrative is never treated as evidence
// of discovery or mutation progress.
type noProgressError struct{ msg string }

func (e *noProgressError) Error() string { return e.msg }

// implementNoProgressError builds the capability_NO_PROGRESS diagnostic (for
// example IMPLEMENT_NO_PROGRESS). It records the observed repository state
// (successful mutations and changed files), the consecutive stale turns
// spent, and the last action, so a stalled run is diagnosable without a model
// self-report. The "a retry may succeed" marker keeps the existing failure
// classifier's retryable-incomplete disposition (CONTINUE).
func (h *Harness) implementNoProgressError(req agent.Request, st *executionState, ev *mutationEvidence, iteration int, lastTool, lastRequest string) error {
	marker := strings.ToUpper(string(req.Capability)) + "_NO_PROGRESS"
	msg := fmt.Sprintf("%s: the Ollama agent %s made no repository progress after %d consecutive stale iterations (provider=ollama, model=%s, iterations=%d, discovery_inspections=%d, repository_mutations=%d, changed_files=%d, tool_calls=%d, termination=%s%s); a retry may succeed",
		marker, req.Capability, st.consecutiveNoProgress, h.cfg.Model, iteration, len(st.discoverySeen), st.repositoryMutations, len(ev.mutationPaths()), st.counters.toolCalls, terminationNoProgress, actionSuffix(lastTool, lastRequest))
	if inspected := st.inspectedSummary(); inspected != "" {
		msg += fmt.Sprintf("\ncontinuation checkpoint (phase=%s, inspected=%s): resume from the intended change rather than repeating repository discovery",
			st.phase.label(), inspected)
	}
	return &noProgressError{msg}
}

// noChangeError builds the retryable "made no repository change" diagnostic. It
// carries a compact continuation checkpoint (the phase the invocation stopped in
// and the repository paths it inspected), so the next bounded invocation resumes
// from the context already gathered instead of repeating discovery. Normally
// unreachable for a zero-mutation run (the no-progress guard stops it first);
// implementNoProgressError carries the same checkpoint. Kept for the fallback path
// described on terminationNoChange.
func (h *Harness) noChangeError(req agent.Request, policy CapabilityPolicy, st *executionState, lastTool, lastRequest string) error {
	msg := fmt.Sprintf("the Ollama agent %s made no repository change after %d iterations (model=%s, tool_calls=%d, termination=%s%s); a retry may succeed",
		req.Capability, policy.MaxIterations, h.cfg.Model, st.counters.interactions, terminationNoChange, actionSuffix(lastTool, lastRequest))
	if inspected := st.inspectedSummary(); inspected != "" {
		msg += fmt.Sprintf("\ncontinuation checkpoint (phase=%s, inspected=%s): resume from the intended change rather than repeating repository discovery",
			st.phase.label(), inspected)
	}
	return &changeIncompleteError{msg}
}

// implementExhaustedError reports that a phased run consumed its hard iteration
// ceiling without finalizing. It records whether a mutation was observed, so the
// two very different failures — changed nothing, and changed things but would not
// stop — are distinguishable.
func (h *Harness) implementExhaustedError(req agent.Request, policy CapabilityPolicy, st *executionState, lastTool, lastRequest string) error {
	return fmt.Errorf("the Ollama agent %s did not complete after %d iterations (model=%s, mutation_observed=%t, tool_calls=%d, termination=%s%s)",
		req.Capability, policy.MaxIterations, h.cfg.Model, st.mutationObserved, st.counters.interactions, terminationIteration, actionSuffix(lastTool, lastRequest))
}

// finalizeLimitError reports that a phased run kept asking for tools after its
// finalization allowance without returning an outcome. It is deliberately
// phase-specific rather than the generic iteration-limit message.
func (h *Harness) finalizeLimitError(req agent.Request, st *executionState, lastTool, lastRequest string) error {
	return fmt.Errorf("the Ollama agent %s did not finalize (model=%s, mutation_observed=%t, tool_calls=%d, finalization_turns=%d, termination=%s%s)",
		req.Capability, h.cfg.Model, st.mutationObserved, st.counters.interactions, st.finalization.turns, terminationFinalization, actionSuffix(lastTool, lastRequest))
}
