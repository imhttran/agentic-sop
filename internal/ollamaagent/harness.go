package ollamaagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Run reads one SOP command-agent request from in, executes it against the
// configured Ollama model, and writes the response content to out.
//
// A harness failure is reported as a structured "failed" outcome for a
// capability that expects one (IMPLEMENT, FIX, DESIGN_TESTS, DIAGNOSE_FAILURE).
// For PLAN and REVIEW, which expect their own JSON schema, it returns an error so
// SOP records an agent failure instead of a malformed document.
func Run(ctx context.Context, in io.Reader, out, errOut io.Writer, getwd func() (string, error)) error {
	var req agent.Request
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}
	if err := req.Validate(); err != nil {
		return err
	}

	cfg, err := ConfigFromEnv()
	if err != nil {
		return err
	}
	root, err := getwd()
	if err != nil {
		return fmt.Errorf("determine repository root: %w", err)
	}

	h := New(cfg, root)
	content, err := h.Execute(ctx, req)
	// Surface the audit trail even when the run fails: a failed task still has
	// tool activity worth reviewing, and a durable sink is chosen by the
	// operator (SOP_TOOL_AUDIT_LOG), never SOP's state database.
	h.FlushAudit(errOut)
	if err != nil {
		// A failed run is exactly when the turn-by-turn history matters, so the
		// safe per-turn trace is written before the outcome is reported.
		h.FlushTrace(errOut)
		if wantsOutcome(req.Capability) {
			fmt.Fprintf(errOut, "sop-ollama-agent: %v\n", err)
			status := agent.OutcomeFailed
			var incomplete *changeIncompleteError
			if errors.As(err, &incomplete) {
				// The agent never changed the repository: retry, don't block.
				status = agent.OutcomeNeedsHuman
			}
			return writeJSON(out, outcomeWire{Status: string(status), Reason: err.Error()})
		}
		return err
	}
	// Ground a mutating capability's completed outcome in the repository change the
	// harness actually observed, so a run cannot claim a change it did not make (nor
	// deny one it did). The model's own claim is preserved in the summary note.
	if req.Capability == agent.Implement || req.Capability == agent.Fix {
		content = h.reconcileOutcome(ctx, content)
		if h.mismatch {
			fmt.Fprintf(errOut, "sop-ollama-agent: changes_expected disagreed with the observed repository change; reconciled\n")
		}
	}
	// A model that reports a failure or a human boundary still produced a turn
	// history worth keeping, so its trace is written as well — otherwise only
	// infrastructure errors, and not the failures the model chose to report, are
	// diagnosable.
	if wantsOutcome(req.Capability) && nonCompletedOutcome(content) {
		h.FlushTrace(errOut)
	}
	if _, err := io.WriteString(out, content); err != nil {
		return err
	}
	return nil
}

// nonCompletedOutcome reports whether content is a structured outcome that is not
// "completed" (the failed or needs_human report a model chose to return).
func nonCompletedOutcome(content string) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var w outcomeWire
	if err := json.Unmarshal([]byte(trimmed), &w); err != nil {
		return false
	}
	return w.Status == string(agent.OutcomeFailed) || w.Status == string(agent.OutcomeNeedsHuman)
}

// wantsOutcome reports whether a capability's expected response is a structured
// execution outcome rather than a schema document (PLAN, REVIEW).
func wantsOutcome(c agent.Capability) bool {
	switch c {
	case agent.Plan, agent.Review:
		return false
	default:
		return true
	}
}

// outcomeWire is the command-agent outcome as SOP's command provider parses it.
// The field tags match that wire format exactly.
type outcomeWire struct {
	Status          string `json:"status"`
	Summary         string `json:"summary,omitempty"`
	Reason          string `json:"reason,omitempty"`
	ChangesExpected *bool  `json:"changes_expected,omitempty"`
}

// Harness executes one SOP request against Ollama using controlled tools.
type Harness struct {
	cfg    Config
	client *ollamaClient
	tools  *toolharness.Harness
	audit  *toolharness.AuditLog
	trace  *TraceLog

	// tracePath is an optional durable sink (SOP_OLLAMA_TRACE_LOG) the failed-run
	// trace is appended to, so it survives the command provider discarding stderr.
	tracePath string

	// mismatch records whether reconcileOutcome overrode the model's
	// changes_expected claim with the repository change actually observed.
	mismatch bool
}

// New returns a Harness working inside root. All repository access goes through
// the shared toolharness, so this agent and any other caller share one policy:
// the state database is refused for every tool, destructive Git is denied, and
// every tool call is audited.
func New(cfg Config, root string) *Harness {
	audit := toolharness.NewAuditLog(defaultMaxAuditRecords)
	var sink toolharness.Auditor = audit
	if path := strings.TrimSpace(lookupEnv(envToolAuditLog)); path != "" {
		sink = toolharness.NewMultiAuditor(audit, toolharness.NewFileAuditor(path))
	}
	tools := toolharness.New(root, toolharness.Config{
		CommandTimeout: cfg.CommandTimeout,
		MaxOutputBytes: cfg.MaxOutputBytes,
	}, sink)
	return &Harness{
		cfg:       cfg,
		client:    newOllamaClient(cfg),
		tools:     tools,
		audit:     audit,
		trace:     newTraceLog(defaultMaxTraceRecords),
		tracePath: strings.TrimSpace(lookupEnv(envToolTraceLog)),
	}
}

// AuditRecords returns the in-memory audit trail for this run, for review and
// troubleshooting.
func (h *Harness) AuditRecords() []toolharness.AuditRecord { return h.audit.Records() }

// TraceRecords returns the in-memory per-turn trace for this run, for review and
// troubleshooting. It never contains model prompts, file contents, or secrets.
func (h *Harness) TraceRecords() []TraceRecord { return h.trace.Records() }

// FlushTrace writes a compact per-turn trace to w. It is called on a failed run so
// the turn history that led to the failure is visible without reading a sink. When
// SOP_OLLAMA_TRACE_LOG names a file, the trace is also appended there, so it
// survives the command provider discarding the harness's stderr.
func (h *Harness) FlushTrace(w io.Writer) {
	h.trace.Flush(w)
	if h.tracePath == "" {
		return
	}
	f, err := os.OpenFile(h.tracePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(w, "sop-ollama-agent: cannot open %s: %v\n", h.tracePath, err)
		return
	}
	defer f.Close()
	h.trace.Flush(f)
}

// FlushAudit writes a one-line audit summary to w so an operator sees what was
// executed and denied without reading a separate file. A nil or empty trail
// writes nothing.
func (h *Harness) FlushAudit(w io.Writer) {
	records := h.audit.Records()
	if len(records) == 0 {
		return
	}
	allowed, denied := 0, 0
	for _, r := range records {
		if r.Action == toolharness.ActionDeny {
			denied++
		} else {
			allowed++
		}
	}
	fmt.Fprintf(w, "sop-ollama-agent: tools: %d calls, %d allowed, %d denied\n", len(records), allowed, denied)
}

// Execute runs the request's capability and returns the model's final JSON object
// as canonical JSON, ready for SOP to parse. PLAN runs a two-phase discovery →
// synthesis loop; the phased mutating capabilities (IMPLEMENT and FIX) share the
// three-phase discover → change → finalize loop; every other capability runs the
// generic bounded loop.
func (h *Harness) Execute(ctx context.Context, req agent.Request) (string, error) {
	switch req.Capability {
	case agent.Plan, agent.Review:
		return h.executeTwoPhase(ctx, req)
	case agent.Implement, agent.Fix:
		return h.executePhased(ctx, req)
	default:
		return h.executeLoop(ctx, req)
	}
}

// executeLoop is the generic single-phase tool loop. It is bounded by the
// capability's policy — an iteration budget and an allowed tool set — and detects
// a model that repeats non-progressing turns, gives it one recovery instruction,
// and then stops with a diagnostic rather than burning the whole budget.
func (h *Harness) executeLoop(ctx context.Context, req agent.Request) (string, error) {
	policy := PolicyFor(req.Capability)
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req, policy)},
		{Role: "user", Content: userPrompt(req)},
	}

	var (
		progress  turnProgress
		toolCalls int
		lastTool  string
		lastReq   string
	)
	for iteration := 1; iteration <= policy.MaxIterations; iteration++ {
		raw, calls, err := h.chat(ctx, messages)
		if err != nil {
			return "", err
		}

		// A turn expresses a tool call either as a JSON object in content (the
		// harness protocol) or as a native Ollama tool call with empty content.
		// Content wins when both are present so a final answer is never lost.
		name, args, isTool, final, err := turnToolCall(raw, calls)
		if err != nil {
			if !errors.Is(err, errNarrate) {
				return "", err
			}
			// The model narrated without emitting a tool call or the final object.
			// Nudge it to continue; repeated narration is itself a no-progress
			// signal, so it cannot consume the whole budget silently.
			recovery, terminate := progress.observe(narrationFingerprint(req.Capability))
			h.recordTurn(req, iteration, "narrate", "", &progress, terminate)
			if terminate {
				return "", h.noProgressError(req, iteration, "narrate", "")
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: raw},
				chatMessage{Role: "user", Content: nudgeText(recovery)},
			)
			continue
		}
		if !isTool {
			encoded, err := json.Marshal(final)
			if err != nil {
				return "", fmt.Errorf("encode final response: %w", err)
			}
			return string(encoded), nil
		}

		// Capability policy: a read-only capability must not mutate the repository.
		// The refusal is reported to the model (so it can choose an allowed tool),
		// audited as a denial, and counted as a turn for progress detection.
		if !policy.Allows(name) {
			detail := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
			lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)
			h.tools.RecordDenied(name, args, detail)
			recovery, terminate := progress.observe(actionFingerprint(name, args, detail, nil))
			h.recordTurn(req, iteration, name, lastReq, &progress, terminate)
			if terminate {
				return "", h.noProgressError(req, iteration, lastTool, lastReq)
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: toolResultMessage(name, "", errors.New(detail)) + recoverySuffix(recovery)},
			)
			continue
		}

		if toolCalls >= h.cfg.MaxToolCalls {
			return "", fmt.Errorf("tool-call limit reached (%d tool calls); the model did not finish", h.cfg.MaxToolCalls)
		}
		toolCalls++

		result, toolErr := h.tools.Run(ctx, name, args)
		lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)
		recovery, terminate := progress.observe(actionFingerprint(name, args, result, toolErr))
		h.recordTurn(req, iteration, name, lastReq, &progress, terminate)
		if terminate {
			return "", h.noProgressError(req, iteration, lastTool, lastReq)
		}
		messages = append(messages,
			chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
			chatMessage{Role: "user", Content: toolResultMessage(name, result, toolErr) + recoverySuffix(recovery)},
		)
	}
	return "", h.limitError(req, policy, lastTool, lastReq)
}

// noProgressThreshold is how many genuinely identical consecutive turns are
// tolerated before the model is told to stop repeating itself; one more
// repetition after that instruction terminates the run early.
const noProgressThreshold = 3

// Termination reasons, reported in both the error and the trace.
const (
	terminationIteration    = "iteration_limit"
	terminationNoProgress   = "no_progress"
	terminationSynthesis    = "synthesis_limit"
	terminationFinalization = "finalization_limit"
)

// turnProgress tracks consecutive identical turns so the loop can tell a
// productive exploration from a model stuck repeating itself.
type turnProgress struct {
	prev     string
	repeated int
	recovery bool
}

// observe folds one turn's fingerprint into the run-length state. A fingerprint
// that differs from the previous turn is progress and resets the run length. It
// returns whether the recovery instruction should be sent this turn, and whether
// the model kept repeating itself after that instruction and must be stopped.
func (p *turnProgress) observe(fp string) (sendRecovery, terminate bool) {
	if fp != "" && fp == p.prev {
		p.repeated++
	} else {
		p.repeated = 1
	}
	p.prev = fp
	if p.repeated < noProgressThreshold {
		return false, false
	}
	if p.recovery {
		return false, true
	}
	p.recovery = true
	return true, false
}

// label reports the progress word for a trace entry.
func (p *turnProgress) label() string {
	if p.repeated > 1 {
		return progressRepeat
	}
	return progressOK
}

// recordTurn appends one safe, secret-free trace entry for a model turn.
func (h *Harness) recordTurn(req agent.Request, iteration int, action, request string, p *turnProgress, terminate bool) {
	rec := TraceRecord{
		Capability: string(req.Capability),
		Iteration:  iteration,
		Tool:       action,
		Request:    request,
		Progress:   p.label(),
		Recovery:   p.recovery,
	}
	if terminate {
		rec.Termination = terminationNoProgress
	}
	h.trace.Record(rec)
}

// actionFingerprint is a deterministic identity for one executed tool action. It
// includes the result, so a repeated action that now returns different content (a
// file changed, a test started passing) is treated as progress rather than a
// stuck loop. It hashes arguments and result, so the trace never carries content.
func actionFingerprint(name string, args map[string]any, result string, err error) string {
	sum := sha256.New()
	io.WriteString(sum, name)
	sum.Write([]byte{0})
	// encoding/json sorts map keys, so the argument encoding is deterministic.
	if b, merr := json.Marshal(args); merr == nil {
		sum.Write(b)
	}
	sum.Write([]byte{0})
	if err != nil {
		io.WriteString(sum, "error:"+err.Error())
	} else {
		io.WriteString(sum, "ok:"+result)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// narrationFingerprint identifies a narration turn (no tool and no final object)
// for progress detection.
func narrationFingerprint(c agent.Capability) string { return "narrate:" + string(c) }

// nudgeText is the user message sent after a narration turn: normally a reminder
// to reply in the one-object protocol; when recovery is warranted, the stronger
// instruction to stop repeating and finish.
func nudgeText(recovery bool) string {
	if recovery {
		return progressReminder
	}
	return turnReminder
}

// recoverySuffix appends the recovery instruction to a tool result, so it stays in
// the same user turn rather than producing two consecutive user messages.
func recoverySuffix(recovery bool) string {
	if !recovery {
		return ""
	}
	return "\n\n" + progressReminder
}

// limitError reports that a capability exhausted its iteration budget without
// returning a final response.
func (h *Harness) limitError(req agent.Request, policy CapabilityPolicy, lastTool, lastRequest string) error {
	return fmt.Errorf("Ollama agent %s did not complete after %d iterations (termination=%s, model=%s%s)",
		req.Capability, policy.MaxIterations, terminationIteration, h.cfg.Model, actionSuffix(lastTool, lastRequest))
}

// noProgressError reports that a capability kept repeating a non-progressing
// action after it was told to conclude.
func (h *Harness) noProgressError(req agent.Request, iteration int, action, request string) error {
	return fmt.Errorf("Ollama agent %s stopped after %d iterations; the model repeated a non-progressing action (termination=%s, model=%s, repeated_action=%q)",
		req.Capability, iteration, terminationNoProgress, h.cfg.Model, strings.TrimSpace(action+" "+request))
}

// actionSuffix renders the last action for a diagnostic, or "" when none ran.
func actionSuffix(tool, request string) string {
	if tool == "" {
		return ""
	}
	return fmt.Sprintf(", last_action=%q", strings.TrimSpace(tool+" "+request))
}

// planPhase is one phase of a PLAN invocation: bounded read-only discovery, then
// a tool-free synthesis. Phase state is scoped to a single executePlan call; it
// is not SOP workflow state.
type planPhase int

const (
	phaseDiscovery planPhase = iota
	phaseSynthesis
)

// label renders the phase for the trace and diagnostics.
func (p planPhase) label() string {
	if p == phaseSynthesis {
		return "SYNTHESIS"
	}
	return "DISCOVERY"
}

// PLAN phase instructions. When discovery is exhausted the model is told
// exploration is over; a tool request during synthesis is corrected, not executed.
// twoPhase bounds one document-producing capability's loop: a bounded read-only
// discovery, then a tool-free synthesis that must produce the document. PLAN and
// REVIEW share the machinery and differ only in these values.
type twoPhase struct {
	discoveryTurns int
	synthesisTurns int
	instruction    string
	correction     string
}

// twoPhaseFor returns the two-phase bounds and synthesis wording for a capability.
func twoPhaseFor(c agent.Capability) twoPhase {
	if c == agent.Review {
		return twoPhase{
			discoveryTurns: reviewDiscoveryTurns,
			synthesisTurns: reviewSynthesisTurns,
			instruction:    reviewSynthesisInstruction,
			correction:     reviewSynthesisCorrection,
		}
	}
	return twoPhase{
		discoveryTurns: planDiscoveryTurns,
		synthesisTurns: planSynthesisTurns,
		instruction:    planSynthesisInstruction,
		correction:     planSynthesisCorrection,
	}
}

const (
	planSynthesisInstruction = `Exploration is complete.
Do not request any more tools.
Using only the repository context already gathered, produce the required PLAN response now.
Do not continue exploring.
Do not implement anything.
Return the exact structured PLAN response expected by SOP.`

	planSynthesisCorrection = `Repository discovery is complete.
No additional tools are available.
Produce the required PLAN response using the context already gathered.`

	reviewSynthesisInstruction = `Exploration is complete.
Do not request any more tools.
Using only the repository context already gathered, produce the required REVIEW response now.
Do not continue exploring.`

	reviewSynthesisCorrection = `Repository discovery is complete.
No additional tools are available.
Produce the required REVIEW response using the context already gathered.`

	synthesisTransitionEvent = "→ SYNTHESIS"
)

// executeTwoPhase runs the bounded discovery → tool-free synthesis loop shared by
// the document-producing capabilities (PLAN and REVIEW). Discovery permits at most
// the capability's discovery budget of turns (each at most one tool) with the
// centralized read-only tool policy. When discovery is exhausted without a final
// response, tools are withdrawn and the model is told to synthesize; synthesis
// permits at most the capability's synthesis budget of model turns. A final
// response ends the invocation immediately, in either phase, so early completion
// is preserved.
func (h *Harness) executeTwoPhase(ctx context.Context, req agent.Request) (string, error) {
	tp := twoPhaseFor(req.Capability)
	policy := PolicyFor(req.Capability) // read-only tools
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req, policy)},
		{Role: "user", Content: userPrompt(req)},
	}

	var (
		phase      = phaseDiscovery
		discovery  int // discovery turns taken (each executes at most one tool)
		discovered int // discovery tools actually executed
		synthesis  int // synthesis turns taken
	)

	for {
		if phase == phaseSynthesis && synthesis >= tp.synthesisTurns {
			h.trace.Record(TraceRecord{
				Capability:  string(req.Capability),
				Phase:       phaseSynthesis.label(),
				Iteration:   synthesis,
				Termination: terminationSynthesis,
			})
			return "", h.synthesisLimitError(req, discovered, synthesis)
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
			if phase == phaseDiscovery {
				discovery++
				h.recordTwoPhaseTurn(req, phase, discovery, "narrate", "")
				messages = append(messages,
					chatMessage{Role: "assistant", Content: raw},
					chatMessage{Role: "user", Content: turnReminder},
				)
				if discovery >= tp.discoveryTurns {
					phase = phaseSynthesis
					messages = append(messages, chatMessage{Role: "user", Content: tp.instruction})
					h.recordSynthesisTransition(req)
				}
			} else {
				synthesis++
				h.recordTwoPhaseTurn(req, phase, synthesis, "narrate", "")
				messages = append(messages,
					chatMessage{Role: "assistant", Content: raw},
					chatMessage{Role: "user", Content: turnReminder},
				)
			}

		case !isTool: // a final response ends the invocation immediately
			index := discovery + 1
			if phase == phaseSynthesis {
				index = synthesis + 1
			}
			h.recordTwoPhaseTurn(req, phase, index, "final", "")
			encoded, mErr := json.Marshal(final)
			if mErr != nil {
				return "", fmt.Errorf("encode final response: %w", mErr)
			}
			return string(encoded), nil

		case phase == phaseSynthesis: // no tools during synthesis: deny and correct
			synthesis++
			h.recordTwoPhaseTurn(req, phase, synthesis, name, toolharness.SummarizeRequest(name, args))
			h.tools.RecordDenied(name, args, "tools are unavailable during synthesis")
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: tp.correction},
			)

		default: // discovery
			discovery++
			summary := toolharness.SummarizeRequest(name, args)
			if !policy.Allows(name) {
				detail := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
				h.tools.RecordDenied(name, args, detail)
				h.recordTwoPhaseTurn(req, phase, discovery, name, summary)
				messages = append(messages,
					chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
					chatMessage{Role: "user", Content: toolResultMessage(name, "", errors.New(detail))},
				)
			} else {
				result, toolErr := h.tools.Run(ctx, name, args)
				discovered++
				h.recordTwoPhaseTurn(req, phase, discovery, name, summary)
				messages = append(messages,
					chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
					chatMessage{Role: "user", Content: toolResultMessage(name, result, toolErr)},
				)
			}
			if discovery >= tp.discoveryTurns {
				phase = phaseSynthesis
				messages = append(messages, chatMessage{Role: "user", Content: tp.instruction})
				h.recordSynthesisTransition(req)
			}
		}
	}
}

// recordTwoPhaseTurn appends one safe trace entry for a two-phase turn.
func (h *Harness) recordTwoPhaseTurn(req agent.Request, phase planPhase, index int, tool, request string) {
	h.trace.Record(TraceRecord{
		Capability: string(req.Capability),
		Phase:      phase.label(),
		Iteration:  index,
		Tool:       tool,
		Request:    request,
		Progress:   progressOK,
	})
}

// recordSynthesisTransition marks the discovery-to-synthesis transition in the trace.
func (h *Harness) recordSynthesisTransition(req agent.Request) {
	h.trace.Record(TraceRecord{Capability: string(req.Capability), Event: synthesisTransitionEvent})
}

// synthesisLimitError reports that a document-producing capability exhausted its
// tool-free synthesis turns without producing a document. It is deliberately
// phase-specific rather than the generic iteration-limit message.
func (h *Harness) synthesisLimitError(req agent.Request, discovered, synthesis int) error {
	return fmt.Errorf("Ollama agent %s failed during synthesis (model=%s, discovery_tool_calls=%d, synthesis_turns=%d, termination=%s)",
		req.Capability, h.cfg.Model, discovered, synthesis, terminationSynthesis)
}

// IMPLEMENT and FIX share a three-phase loop: bounded DISCOVERY (read the
// repository just enough to make the change), CHANGE (make the change and
// targeted checks), then FINALIZE (no repository tools; return the structured SOP
// outcome). Phase state is scoped to a single executePhased call — it is not SOP
// workflow state and is never persisted.
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

// IMPLEMENT phase bounds. They are interaction ceilings, not targets: a
// productive run finishes well before them. The finalize threshold sits below the
// capability's hard ceiling (maxIterationsImplement) so the normal completion
// mechanism is finalization, not exhausting the iteration budget.
const (
	// implementNudgeAfter is how many tool interactions of discovery are allowed
	// before the model is nudged to begin implementing. It is a soft transition:
	// the read/search tools stay available.
	implementNudgeAfter = 6
	// implementFinalizeAfter is the tool-interaction count at which a mutated
	// invocation becomes eligible to finalize. Crossing it without a mutation does
	// not finalize: a threshold is not evidence that the work is done.
	implementFinalizeAfter = 18
	// implementCompletionWindow is how many non-mutating tool interactions after a
	// mutation indicate the model has stopped writing and is wrapping up. It is
	// what makes finalization safe: a model still writing resets the count, so it is
	// never finalized mid-change.
	implementCompletionWindow = 2
	// implementLateStageAfter is the late-stage decision point for a run that has
	// not changed the repository: it gets one final instruction to implement or
	// report truthfully. A mutated run is never forced to finalize here — only once
	// it stops writing — so a productive implementation is not cut off.
	implementLateStageAfter = 22
	// implementFinalizeTurns is how many model turns are allowed in FINALIZE before
	// the invocation fails with a finalization diagnostic.
	implementFinalizeTurns = 2
)

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

	implementNowInstruction = `You have gathered enough repository context, but you have not yet made the
required repository change.

Stop broad exploration and implement the requested change now.

Repository tools remain available for implementation.

Use additional reads only when directly necessary to make the change.

Do not return a completed outcome until the required change has
actually been performed.

Once the implementation is complete, return the required structured outcome.
SOP will perform independent validation afterward.`

	implementFinalInstruction = `You have not yet performed the required repository change.

Do not continue repository exploration.

Either:
- perform the required implementation now using the available tools, or
- return a truthful structured outcome (needs_human or failed) explaining why
  the implementation could not be performed.

Do not claim completion without making the required change. Do not invent
repository changes. SOP will independently validate the repository.`

	implementChangeEvent   = "→ CHANGE"
	implementFinalizeEvent = "→ FINALIZE"
	implementContinueEvent = "→ CHANGE_CONTINUE"
)

// implementState is the invocation-scoped phase state of one phased run
// (IMPLEMENT or FIX). It is never stored on the shared Harness, so one invocation
// cannot leak mutation or phase state into another.
type implementState struct {
	phase               implementPhase
	mutated             bool // a controlled mutation succeeded during this invocation
	interactions        int  // tool interactions executed this invocation
	sinceMutation       int  // tool interactions since the last successful mutation
	nudged              bool // the discovery nudge has been sent
	implementInstructed bool // the implement-now instruction has been sent
	finalizeTurns       int  // model turns consumed in FINALIZE
}

// finalizeEligible reports whether the phased loop may withdraw its tools.
// Withdrawing requires an observed mutation — a count alone is not evidence the
// work is done — and that the model has stopped mutating: a model that is still
// writing has not finished, and finalizing it would refuse the very write it
// still needs. A run that never changes the repository has no writer to wait for,
// so it is finalized at the late stage: the model is pushed to conclude (a write
// is still honoured and resumes CHANGE) rather than exploring to the ceiling.
func (st implementState) finalizeEligible() bool {
	if st.interactions < implementFinalizeAfter {
		return false
	}
	if st.mutated {
		return st.sinceMutation >= implementCompletionWindow
	}
	return st.interactions >= implementLateStageAfter
}

// isMutationTool reports whether a successful call to name changes the
// repository, which moves IMPLEMENT from discovery into change.
func isMutationTool(name string) bool {
	return name == toolharness.ToolWriteFile || name == toolharness.ToolCreateFile
}

// executePhased runs the three-phase loop shared by the phased mutating
// capabilities (IMPLEMENT and FIX). Discovery and change may use the controlled
// tools; once the model has changed the repository and then stopped writing for
// the completion window those tools are withdrawn and the model must return the
// structured outcome. A model that is still writing keeps its tools —
// finalization must not truncate a multi-file change — and a mutation requested
// during FINALIZE resumes CHANGE. A threshold crossed without a mutation does not
// finalize: the model is told to implement and keeps its tools. A final response
// ends the invocation in any phase, so early completion is preserved, and the
// capability's MaxIterations stays the hard safety ceiling.
func (h *Harness) executePhased(ctx context.Context, req agent.Request) (string, error) {
	policy := PolicyFor(req.Capability) // all tools; IMPLEMENT may mutate
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req, policy)},
		{Role: "user", Content: userPrompt(req)},
	}

	st := &implementState{phase: implDiscover}
	var (
		progress  turnProgress
		toolCalls int
		lastTool  string
		lastReq   string
	)

	for iteration := 1; iteration <= policy.MaxIterations; iteration++ {
		// FINALIZE has a small turn allowance: a model that keeps asking for tools
		// after implementation is complete is stopped with a phase-specific reason
		// rather than the generic iteration limit.
		if st.phase == implFinalize && st.finalizeTurns >= implementFinalizeTurns {
			h.trace.Record(TraceRecord{
				Capability:  string(req.Capability),
				Phase:       st.phase.label(),
				Iteration:   iteration,
				Termination: terminationFinalization,
			})
			if !st.mutated {
				return "", h.noChangeError(req, policy, st, lastTool, lastReq)
			}
			return "", h.finalizeLimitError(req, st, lastTool, lastReq)
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
				st.finalizeTurns++
			}
			recovery, terminate := progress.observe(narrationFingerprint(req.Capability))
			h.recordImplementTurn(req, st.phase, iteration, "narrate", "", progress.label(), recovery, terminate)
			if terminate {
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

		// No repository tools during FINALIZE: refuse, correct, and count the turn
		// against the finalization allowance rather than executing it. A mutation is
		// the exception: a model that still needs to write has not finished, so honour
		// the write and resume CHANGE instead of refusing it. FINALIZE stops
		// exploration; it must not truncate a multi-file change mid-write.
		if st.phase == implFinalize {
			if !isMutationTool(name) || !policy.Allows(name) {
				st.finalizeTurns++
				lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)
				h.tools.RecordDenied(name, args, "tools are unavailable during IMPLEMENT finalization")
				h.recordImplementTurn(req, st.phase, iteration, name, lastReq, progressDenied, false, false)
				messages = append(messages,
					chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
					chatMessage{Role: "user", Content: implementFinalizeCorrection},
				)
				continue
			}
			st.phase = implChange
			st.finalizeTurns = 0
			h.recordImplementEvent(req, implementChangeEvent, "resumed for a further mutation")
		}

		// Capability policy: keep the invariant that a capability only reaches the
		// tools its policy allows, even though IMPLEMENT currently allows them all.
		if !policy.Allows(name) {
			detail := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
			lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)
			h.tools.RecordDenied(name, args, detail)
			recovery, terminate := progress.observe(actionFingerprint(name, args, detail, nil))
			h.recordImplementTurn(req, st.phase, iteration, name, lastReq, progress.label(), recovery, terminate)
			if terminate {
				return "", h.noProgressError(req, iteration, lastTool, lastReq)
			}
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: toolResultMessage(name, "", errors.New(detail)) + recoverySuffix(recovery)},
			)
			continue
		}

		if toolCalls >= h.cfg.MaxToolCalls {
			return "", fmt.Errorf("tool-call limit reached (%d tool calls); the model did not finish", h.cfg.MaxToolCalls)
		}
		toolCalls++
		st.interactions++

		result, toolErr := h.tools.Run(ctx, name, args)
		lastTool, lastReq = name, toolharness.SummarizeRequest(name, args)

		// A successful controlled mutation is what moves the invocation out of
		// discovery; a failed or denied write never counts.
		justMutated := toolErr == nil && isMutationTool(name)
		if justMutated {
			st.mutated = true
			st.sinceMutation = 0
		} else if st.mutated {
			st.sinceMutation++
		}

		recovery, terminate := progress.observe(actionFingerprint(name, args, result, toolErr))
		if justMutated && st.phase == implDiscover {
			st.phase = implChange
			h.recordImplementEvent(req, implementChangeEvent, "")
		}
		h.recordImplementTurn(req, st.phase, iteration, name, lastReq, progress.label(), recovery, terminate)
		if terminate {
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
		case st.phase == implDiscover && !st.nudged && st.interactions >= implementNudgeAfter:
			st.nudged = true
			advice = append(advice, implementNudge)
		}
		// Finalization is mutation-aware: crossing the threshold alone is not enough.
		// Without a mutation the tools stay enabled and the model is pushed to
		// implement, so a run is never finalized before it has changed anything.
		if st.phase != implFinalize {
			switch {
			case st.finalizeEligible():
				st.phase = implFinalize
				if st.mutated {
					advice = append(advice, implementFinalizeInstruction)
				} else {
					advice = append(advice, implementFinalInstruction)
				}
				h.recordImplementEvent(req, implementFinalizeEvent, "")
			case !st.mutated && st.interactions >= implementFinalizeAfter && !st.implementInstructed:
				st.implementInstructed = true
				advice = append(advice, implementNowInstruction)
				h.recordImplementEvent(req, implementContinueEvent, "implementation required before finalization")
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
	if !st.mutated {
		return "", h.noChangeError(req, policy, st, lastTool, lastReq)
	}
	return "", h.implementExhaustedError(req, policy, st, lastTool, lastReq)
}

// recordImplementTurn appends one safe trace entry for an IMPLEMENT turn.
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
// repository.
const terminationNoChange = "no_change"

// changeIncompleteError reports that a phased mutating capability ended without
// changing the repository: the agent never acted, so the right response is to try
// again (a retryable human boundary), not to record the work as failed.
type changeIncompleteError struct{ msg string }

func (e *changeIncompleteError) Error() string { return e.msg }

// noChangeError builds the retryable "made no repository change" diagnostic.
func (h *Harness) noChangeError(req agent.Request, policy CapabilityPolicy, st *implementState, lastTool, lastRequest string) error {
	return &changeIncompleteError{fmt.Sprintf("Ollama agent %s made no repository change after %d iterations (model=%s, tool_calls=%d, termination=%s%s); a retry may succeed",
		req.Capability, policy.MaxIterations, h.cfg.Model, st.interactions, terminationNoChange, actionSuffix(lastTool, lastRequest))}
}

// implementExhaustedError reports that IMPLEMENT consumed its hard iteration
// ceiling without finalizing. It records whether a mutation was observed, so the
// two very different failures — changed nothing, and changed things but would not
// stop — are distinguishable.
func (h *Harness) implementExhaustedError(req agent.Request, policy CapabilityPolicy, st *implementState, lastTool, lastRequest string) error {
	return fmt.Errorf("Ollama agent %s did not complete after %d iterations (model=%s, mutation_observed=%t, tool_calls=%d, termination=%s%s)",
		req.Capability, policy.MaxIterations, h.cfg.Model, st.mutated, st.interactions, terminationIteration, actionSuffix(lastTool, lastRequest))
}

// finalizeLimitError reports that IMPLEMENT kept asking for tools after its
// finalization allowance without returning an outcome. It is deliberately
// phase-specific rather than the generic iteration-limit message.
func (h *Harness) finalizeLimitError(req agent.Request, st *implementState, lastTool, lastRequest string) error {
	return fmt.Errorf("Ollama agent %s did not finalize (model=%s, mutation_observed=%t, tool_calls=%d, finalization_turns=%d, termination=%s%s)",
		req.Capability, h.cfg.Model, st.mutated, st.interactions, st.finalizeTurns, terminationFinalization, actionSuffix(lastTool, lastRequest))
}

// maxEmptyRetries bounds how many times an empty model turn is re-requested
// before it is a failure.
const maxEmptyRetries = 2

// chat asks the model for the next turn, re-requesting a bounded number of times
// when the model returns neither content nor a tool call. Other errors are
// returned immediately.
func (h *Harness) chat(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	var (
		content string
		calls   []toolCall
		err     error
	)
	for attempt := 0; attempt <= maxEmptyRetries; attempt++ {
		content, calls, err = h.client.chat(ctx, messages)
		if err == nil {
			return content, calls, nil
		}
		if !errors.Is(err, errEmptyResponse) {
			return "", nil, err
		}
	}
	return "", nil, err
}

// turnReminder is sent when a model turn is neither a tool call nor a final
// object, so the model continues in the one-object-per-turn protocol instead of
// failing the task.
const turnReminder = `Your reply contained no JSON object. Reply now with exactly one JSON object and no prose: either a tool call {"tool": "<name>", "args": {...}} or the required final object.`

// progressReminder is injected once when the model is detected repeating
// non-progressing turns: it tells the model to conclude with what it already has
// instead of continuing optional exploration.
const progressReminder = `You are repeating actions without making progress.

Use the information already gathered.
Do not perform additional optional exploration.
Complete the requested capability now and return the required final response.

SOP will independently validate the repository after you finish.`

// errNarrate marks a model turn that carried no tool call and no final object
// (prose only); the loop nudges the model rather than failing.
var errNarrate = errors.New("model turn was narration, not a tool call or a final object")

// turnToolCall interprets one model turn. It returns a tool call when the turn
// is one, or the final JSON object (a document or an outcome) when it is not.
// Content is preferred over native tool calls so a final answer is never lost;
// a turn with neither returns errNarrate.
func turnToolCall(raw string, calls []toolCall) (name string, args map[string]any, isTool bool, final map[string]any, err error) {
	obj, objErr := firstJSONObject(raw)
	switch {
	case objErr == nil:
		n, a, t, aerr := asToolCall(obj)
		if aerr != nil {
			return "", nil, false, nil, aerr
		}
		if !t {
			return "", nil, false, obj, nil
		}
		return n, a, true, nil, nil
	case len(calls) > 0:
		return calls[0].Name, calls[0].Args, true, nil, nil
	default:
		return "", nil, false, nil, errNarrate
	}
}

// assistantEcho is the assistant message echoed back before a tool result. It is
// the model's own content when it expressed the call there, or a synthesized
// tool-call object when the call came from Ollama's native tool_calls, so the
// conversation stays in the JSON protocol the model was prompted with.
func assistantEcho(name string, args map[string]any, raw string) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	b, err := json.Marshal(map[string]any{"tool": name, "args": args})
	if err != nil {
		return fmt.Sprintf(`{"tool": %q}`, name)
	}
	return string(b)
}

// asToolCall reports whether obj is a tool call and validates its shape. A
// present-but-malformed "tool" field is an explicit error, never a silent final
// answer.
func asToolCall(obj map[string]any) (name string, args map[string]any, isTool bool, err error) {
	raw, present := obj["tool"]
	if !present {
		return "", nil, false, nil
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", nil, false, errors.New(`malformed tool request: "tool" must be a non-empty string`)
	}
	args = map[string]any{}
	if v, present := obj["args"]; present {
		m, ok := v.(map[string]any)
		if !ok {
			return "", nil, false, errors.New(`malformed tool request: "args" must be an object`)
		}
		args = m
	}
	return strings.TrimSpace(s), args, true, nil
}

// firstJSONObject extracts the JSON object at the first "{" in s. With format
// "json" the model emits a bare object, but leading prose is tolerated rather
// than trusted: the object begins at the first brace. If that brace does not open
// a decodable object, the turn has no usable JSON.
func firstJSONObject(s string) (map[string]any, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return nil, errors.New("no JSON object in response")
	}
	if obj, ok := objectAt(s, start); ok {
		return obj, nil
	}
	return nil, errors.New("no JSON object in response")
}

// objectAt parses the balanced JSON object beginning at the "{" at index start,
// reporting whether one was found and decoded.
func objectAt(s string, start int) (map[string]any, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// skip string content
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				var obj map[string]any
				if err := json.Unmarshal([]byte(escapeControlChars(s[start:i+1])), &obj); err != nil {
					return nil, false
				}
				return obj, true
			}
		}
	}
	return nil, false
}

// escapeControlChars escapes raw control characters that appear inside JSON
// strings. A model routinely writes a multi-line string value (a file body) with
// literal newlines and tabs, which strict JSON rejects; escaping them keeps the
// model's intent without a lenient parser.
func escapeControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
			b.WriteByte(c)
		case inString && c == '\\':
			escaped = true
			b.WriteByte(c)
		case c == '"':
			inString = !inString
			b.WriteByte(c)
		case inString && c < 0x20:
			switch c {
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			default:
				fmt.Fprintf(&b, `\u%04x`, c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// writeJSON writes v as a single JSON line.
func writeJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// truncate bounds s to at most n bytes, marking the cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-n)
}
