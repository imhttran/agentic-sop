package ollamaagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
			return writeJSON(out, outcomeWire{Status: string(agent.OutcomeFailed), Reason: err.Error()})
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
	if _, err := io.WriteString(out, content); err != nil {
		return err
	}
	return nil
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
		cfg:    cfg,
		client: newOllamaClient(cfg),
		tools:  tools,
		audit:  audit,
		trace:  newTraceLog(defaultMaxTraceRecords),
	}
}

// AuditRecords returns the in-memory audit trail for this run, for review and
// troubleshooting.
func (h *Harness) AuditRecords() []toolharness.AuditRecord { return h.audit.Records() }

// TraceRecords returns the in-memory per-turn trace for this run, for review and
// troubleshooting. It never contains model prompts, file contents, or secrets.
func (h *Harness) TraceRecords() []TraceRecord { return h.trace.Records() }

// FlushTrace writes a compact per-turn trace to w. It is called on a failed run so
// the turn history that led to the failure is visible without reading a sink.
func (h *Harness) FlushTrace(w io.Writer) { h.trace.Flush(w) }

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
// synthesis loop; every other capability runs the generic bounded loop.
func (h *Harness) Execute(ctx context.Context, req agent.Request) (string, error) {
	if req.Capability == agent.Plan {
		return h.executePlan(ctx, req)
	}
	return h.executeLoop(ctx, req)
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
	terminationIteration  = "iteration_limit"
	terminationNoProgress = "no_progress"
	terminationSynthesis  = "synthesis_limit"
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

	planSynthesisTransitionEvent = "→ SYNTHESIS"
)

// executePlan runs the two-phase PLAN loop. Discovery permits at most
// planDiscoveryTurns turns (so at most that many tool executions) with the
// centralized read-only tool policy. When discovery is exhausted without a final
// response, tools are withdrawn and the model is told to synthesize; synthesis
// permits at most planSynthesisTurns model turns. A final response ends the
// invocation immediately, in either phase, so early completion is preserved.
func (h *Harness) executePlan(ctx context.Context, req agent.Request) (string, error) {
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
		if phase == phaseSynthesis && synthesis >= planSynthesisTurns {
			h.trace.Record(TraceRecord{
				Capability:  string(req.Capability),
				Phase:       phaseSynthesis.label(),
				Iteration:   synthesis,
				Termination: terminationSynthesis,
			})
			return "", h.planSynthesisLimitError(req, discovered, synthesis)
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
				h.recordPlanTurn(req, phase, discovery, "narrate", "")
				messages = append(messages,
					chatMessage{Role: "assistant", Content: raw},
					chatMessage{Role: "user", Content: turnReminder},
				)
				if discovery >= planDiscoveryTurns {
					phase = phaseSynthesis
					messages = append(messages, chatMessage{Role: "user", Content: planSynthesisInstruction})
					h.recordPlanTransition(req)
				}
			} else {
				synthesis++
				h.recordPlanTurn(req, phase, synthesis, "narrate", "")
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
			h.recordPlanTurn(req, phase, index, "final", "")
			encoded, mErr := json.Marshal(final)
			if mErr != nil {
				return "", fmt.Errorf("encode final response: %w", mErr)
			}
			return string(encoded), nil

		case phase == phaseSynthesis: // no tools during synthesis: deny and correct
			synthesis++
			h.recordPlanTurn(req, phase, synthesis, name, toolharness.SummarizeRequest(name, args))
			h.tools.RecordDenied(name, args, "tools are unavailable during PLAN synthesis")
			messages = append(messages,
				chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
				chatMessage{Role: "user", Content: planSynthesisCorrection},
			)

		default: // discovery
			discovery++
			summary := toolharness.SummarizeRequest(name, args)
			if !policy.Allows(name) {
				detail := fmt.Sprintf("tool %q is not available for %s; allowed tools: %s", name, req.Capability, describeTools(policy))
				h.tools.RecordDenied(name, args, detail)
				h.recordPlanTurn(req, phase, discovery, name, summary)
				messages = append(messages,
					chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
					chatMessage{Role: "user", Content: toolResultMessage(name, "", errors.New(detail))},
				)
			} else {
				result, toolErr := h.tools.Run(ctx, name, args)
				discovered++
				h.recordPlanTurn(req, phase, discovery, name, summary)
				messages = append(messages,
					chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
					chatMessage{Role: "user", Content: toolResultMessage(name, result, toolErr)},
				)
			}
			if discovery >= planDiscoveryTurns {
				phase = phaseSynthesis
				messages = append(messages, chatMessage{Role: "user", Content: planSynthesisInstruction})
				h.recordPlanTransition(req)
			}
		}
	}
}

// recordPlanTurn appends one safe trace entry for a PLAN turn.
func (h *Harness) recordPlanTurn(req agent.Request, phase planPhase, index int, tool, request string) {
	h.trace.Record(TraceRecord{
		Capability: string(req.Capability),
		Phase:      phase.label(),
		Iteration:  index,
		Tool:       tool,
		Request:    request,
		Progress:   progressOK,
	})
}

// recordPlanTransition marks the discovery-to-synthesis transition in the trace.
func (h *Harness) recordPlanTransition(req agent.Request) {
	h.trace.Record(TraceRecord{Capability: string(req.Capability), Event: planSynthesisTransitionEvent})
}

// planSynthesisLimitError reports that PLAN exhausted its tool-free synthesis
// turns without producing a document. It is deliberately phase-specific rather
// than the generic iteration-limit message.
func (h *Harness) planSynthesisLimitError(req agent.Request, discovered, synthesis int) error {
	return fmt.Errorf("Ollama agent %s failed during synthesis (model=%s, discovery_tool_calls=%d, synthesis_turns=%d, termination=%s)",
		req.Capability, h.cfg.Model, discovered, synthesis, terminationSynthesis)
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
