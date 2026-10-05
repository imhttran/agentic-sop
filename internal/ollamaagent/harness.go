package ollamaagent

import (
	"context"
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
	h.ShowRuntimeVisibility(errOut)
	content, reconciled, err := h.Complete(ctx, req)
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
			var stalled *noProgressError
			if errors.As(err, &incomplete) || errors.As(err, &stalled) {
				// The agent never made repository progress: retry, don't block.
				status = agent.OutcomeNeedsHuman
			}
			return writeJSON(out, outcomeWire{Status: string(status), Reason: err.Error()})
		}
		return err
	}
	if reconciled {
		fmt.Fprintf(errOut, "sop-ollama-agent: changes_expected disagreed with the observed repository change; reconciled\n")
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

// Complete runs one full capability invocation: it captures the working-tree
// baseline (for a mutating capability), runs the capability's lifecycle, and
// grounds a mutating capability's result in the repository change actually
// observed during this invocation. reconciled reports whether the model's
// changes_expected claim disagreed with the observed reality and was overridden.
//
// Execution evidence is invocation-scoped: one mutationEvidence accumulator is
// created here and passed into the capability's lifecycle, the working-tree
// baseline is captured when the invocation starts, and nothing is carried on the
// Harness from one Complete call into the next. Mutation evidence (a successful
// controlled mutation performed by this invocation) is the primary execution
// signal; the working-tree change since the baseline (ChangedSince) is
// reconciliation evidence that can confirm or contradict it.
func (h *Harness) Complete(ctx context.Context, req agent.Request) (content string, reconciled bool, err error) {
	return h.CompleteWithEvidence(ctx, req, &mutationEvidence{})
}

// CompleteWithEvidence is Complete with a caller-supplied mutation-evidence
// accumulator. The caller owns ev and can read the exact repository paths the
// invocation changed (ev.mutationPaths) after Complete returns, so a caller can
// attribute changes to a task from observed mutations rather than inferring them
// from the working-tree diff. Behavior is otherwise identical to Complete.
func (h *Harness) CompleteWithEvidence(ctx context.Context, req agent.Request, ev *mutationEvidence) (content string, reconciled bool, err error) {
	mutating := req.Capability == agent.Implement || req.Capability == agent.Fix

	if ev == nil {
		ev = &mutationEvidence{}
	}

	baseline := ""
	if mutating {
		// Pre-dirty reconciliation: the baseline fingerprint (not HEAD) is the
		// reference, so pre-existing working-tree changes are never attributed to
		// this invocation. Failure to fingerprint degrades to the model's claim
		// rather than blocking the run.
		baseline, _ = h.tools.WorkingTreeFingerprint(ctx)
	}

	content, err = h.ExecuteWithEvidence(ctx, req, ev)
	if err != nil {
		return "", false, err
	}
	if !mutating {
		return content, false, nil
	}

	// Ground a mutating capability's completed outcome in observed reality: the
	// invocation's own mutation evidence is the primary execution signal, and the
	// working-tree change since the baseline is reconciliation evidence. The
	// model's own claim is preserved in the summary note when it disagrees.
	content, reconciled = h.reconcileOutcome(ctx, baseline, ev, content)
	// A failure that changed nothing did not attempt the work: surface it as a
	// retryable boundary so SOP requeues instead of blocking.
	content = h.retryNoChangeFailure(ctx, baseline, ev, content)
	// IMPLEMENT/FIX results must be structured outcomes: wrap prose responses
	// in a completed outcome with changes_expected derived from observed reality.
	content = h.ensureStructuredOutcome(ctx, baseline, ev, content)
	return content, reconciled, nil
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
	Status              string                    `json:"status"`
	Summary             string                    `json:"summary,omitempty"`
	Reason              string                    `json:"reason,omitempty"`
	ChangesExpected     *bool                     `json:"changes_expected,omitempty"`
	Completion          string                    `json:"completion,omitempty"`
	Evidence            *agent.CompletionEvidence `json:"evidence,omitempty"`
	RepositoryMutations *int                      `json:"repository_mutations,omitempty"`
}

// Harness executes one SOP request against Ollama using controlled tools. It
// holds no per-invocation state: every field is configuration, the client, the
// audited tool surface, or a bounded diagnostic log written during execution and
// flushed by the caller. Invocation-scoped state (phase, mutation evidence,
// no-progress detection, the working-tree baseline) lives inside a single
// Complete/Execute call, so one invocation cannot contaminate another.
type Harness struct {
	cfg    Config
	client *ollamaClient
	tools  *toolharness.Harness
	audit  *toolharness.AuditLog
	trace  *TraceLog

	// tracePath is an optional durable sink (SOP_OLLAMA_TRACE_LOG) the failed-run
	// trace is appended to, so it survives the command provider discarding stderr.
	tracePath string
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
		Roots:          cfg.Roots,
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

// ShowRuntimeVisibility writes the execution stack (harness, provider, model, source) to w.
func (h *Harness) ShowRuntimeVisibility(w io.Writer) {
	source := configSource(agent.EnvOllamaModel)
	fmt.Fprintf(w, "Harness: tool\n")
	fmt.Fprintf(w, "Provider: ollama\n")
	fmt.Fprintf(w, "Model: %s\n", h.cfg.Model)
	fmt.Fprintf(w, "Provider source: %s\n", source)
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

// Execute runs one capability lifecycle and returns the model's final JSON object
// as canonical JSON, ready for SOP to parse. Each capability's runner is
// isolated in its own file: PLAN (plan.go), REVIEW (review.go), IMPLEMENT
// (implement.go), FIX (fix.go); the shared engines they parameterize live in
// orchestrate.go, and the generic bounded loop below serves the remaining
// capabilities. Execute performs no outcome grounding: reconciliation against the
// repository is Complete's job, because it needs the baseline captured before the
// invocation ran. It records mutation evidence into a fresh accumulator, so
// callers that only invoke Execute (tests, exploratory use) still get an accurate
// signal for that invocation.
func (h *Harness) Execute(ctx context.Context, req agent.Request) (string, error) {
	return h.ExecuteWithEvidence(ctx, req, &mutationEvidence{})
}

// ExecuteWithEvidence runs one capability lifecycle, recording successful
// controlled mutations into ev. The accumulator is owned by the caller (one
// Complete call), so mutation evidence is scoped to one invocation and never
// stored on the Harness. A nil ev disables recording (the lifecycle still runs).
func (h *Harness) ExecuteWithEvidence(ctx context.Context, req agent.Request, ev *mutationEvidence) (string, error) {
	if ev == nil {
		ev = &mutationEvidence{}
	}
	switch req.Capability {
	case agent.Plan:
		return h.executePlan(ctx, req)
	case agent.Review:
		return h.executeReview(ctx, req)
	case agent.Implement:
		return h.executeImplement(ctx, req, ev)
	case agent.Fix:
		return h.executeFix(ctx, req, ev)
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
		{Role: "system", Content: systemPrompt(req, policy, h.tools.Root(), h.cfg.Roots)},
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

// limitError reports that a capability exhausted its iteration budget without
// returning a final response.
func (h *Harness) limitError(req agent.Request, policy CapabilityPolicy, lastTool, lastRequest string) error {
	return fmt.Errorf("the Ollama agent %s did not complete after %d iterations (termination=%s, model=%s%s)",
		req.Capability, policy.MaxIterations, terminationIteration, h.cfg.Model, actionSuffix(lastTool, lastRequest))
}

// noProgressError reports that a capability kept repeating a non-progressing
// action after it was told to conclude.
func (h *Harness) noProgressError(req agent.Request, iteration int, action, request string) error {
	return fmt.Errorf("the Ollama agent %s stopped after %d iterations; the model repeated a non-progressing action (termination=%s, model=%s, repeated_action=%q)",
		req.Capability, iteration, terminationNoProgress, h.cfg.Model, strings.TrimSpace(action+" "+request))
}

// chat asks the model for the next turn. Transient provider failures (an empty
// turn, HTTP 429/5xx, a timeout, a connection failure) are retried at the provider
// boundary (see ollamaClient.chat). The harness does not retry here, so one
// logical turn makes at most one bounded provider-retry sequence and a transient
// failure cannot compound across two retry layers.
func (h *Harness) chat(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	return h.client.chat(ctx, messages)
}
