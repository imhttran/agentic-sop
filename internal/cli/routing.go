package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/adaptiveroute"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// Deterministic per-task model routing (Phase 3.5).
//
// The router (internal/router) maps typed evidence - the task's structure and the
// early-JEV checkpoints' typed evidence - to one of the configured model classes,
// and the model layer resolves that class to a concrete provider/model. This file
// is the seam that computes the decision, records it, and (only) selects the model
// for the task's implementation agent.
//
// It carries no lifecycle authority: routing never approves, blocks, transitions
// state, or bypasses a gate. It only selects which configured model runs the
// bounded implementation work. A manual --model-class override always wins over
// the automatic router, so an operator's explicit choice is never silently
// replaced (Phase 3.5 6).

// taskRouting is the per-task routing decision: the resolved class selection plus
// the deterministic provenance (source, reasons, the typed signals used, and which
// checkpoints informed it). It is the SOLE source of truth for the model choice:
// it is derived in memory by routingForTask and never reconstructed from the
// persisted routing.json. Nothing reads the artifact back to drive a decision.
//
// ExecutionTarget is the resolved EXECUTION target: the routing class carried
// over unchanged plus the actual provider/model/locality that will run and the
// typed execution source (primary, or availability-fallback). It keeps the
// distinction between the routing DECISION and the executing model explicit, so
// persisted evidence never rewrites a SMALL routing decision to MEDIUM merely
// because a SMALL availability fallback executed.
type taskRouting struct {
	Class           model.Class
	Source          runpkg.RoutingSource
	Reasons         []string
	Selection       model.Selection
	ExecutionTarget model.ExecutionTarget
	Signals         router.Signals
	Checkpoints     []string
}

// routingForTask computes the per-task routing decision. It returns ok=false when
// no routing applies (automatic routing is disabled and no manual override).
//
// A manual --model-class override wins: it is recorded as a manual_override
// decision but never re-derived by the router.
func routingForTask(cfg config.Config, d deps, spec *taskfile.Spec, tri, pre earlyGateResult) (taskRouting, bool, error) {
	// A recovery-driven escalated attempt outranks both the router and a manual
	// override: the recovery policy (internal/recovery) already decided this
	// attempt's class, and escalation is never offered while an override pins the
	// class (see escalatable). Its provenance is recorded as the escalation source,
	// never as a router or operator decision.
	if d.attempt != nil {
		return taskRouting{
			Class:           d.attempt.Decision.ToClass,
			Source:          runpkg.RoutingSourceEscalation,
			Reasons:         []string{d.attempt.Decision.Reason},
			Selection:       d.attempt.Selection,
			ExecutionTarget: model.ExecutionTargetForPrimary(d.attempt.Selection),
		}, true, nil
	}
	if strings.TrimSpace(d.modelClass) != "" {
		if d.routing.Active {
			// A manual override pins the CLASS; it does not pin the model within it,
			// so the local-first fallback applies to an explicitly chosen class too.
			res, applied := applyLocalFallback(cfg, d, d.routing)
			return taskRouting{
				Class:           res.Selection.Class,
				Source:          runpkg.RoutingSourceManual,
				Reasons:         routingReasons([]string{model.RoutingReasonManual}, res.Selection),
				Selection:       res.Selection,
				ExecutionTarget: executionTargetFor(res, applied),
			}, true, nil
		}
		return taskRouting{}, false, nil
	}
	if !d.routingEnabled {
		return taskRouting{}, false, nil
	}

	task := router.TaskSignals{
		AcceptanceCriteria: len(spec.AcceptanceCriteria),
		Dependencies:       len(spec.Dependencies),
	}
	sig := router.SignalsFrom(nil, task)
	var checkpoints []string
	for _, g := range []struct {
		name   string
		result earlyGateResult
	}{{"task_triage", tri}, {"pre_execution", pre}} {
		if ev := evidenceFrom(g.result); ev != nil {
			sig = sig.Merge(router.SignalsFrom(ev, task))
			checkpoints = append(checkpoints, g.name)
		}
	}

	dec := router.Decide(sig)
	// CTX-011 adaptive routing: adapt the deterministic router's class using accumulated
	// evaluation evidence. The harness chooses the class; a model never chooses itself.
	// With no evidence (or too little) the baseline is unchanged, so this is a no-op until
	// enough evidence accrues.
	if adapted := adaptiveroute.Route(adaptiveroute.Input{
		Baseline:   dec.Class,
		Capability: agent.Implement,
		Evidence:   routingEvidence(routingDir(d)),
	}); adapted.Class != dec.Class {
		dec.Class = adapted.Class
		dec.Reasons = append(dec.Reasons, adapted.Reason+" ("+adapted.Evidence+")")
	}
	res, err := model.Resolve(model.Inputs{
		Config:       cfg.Models,
		Lookup:       os.Getenv,
		RoutedClass:  dec.Class,
		RoutedReason: router.ReasonsText(dec.Reasons),
	})
	if err != nil {
		return taskRouting{}, false, err
	}
	if !res.Active {
		return taskRouting{}, false, nil
	}
	res, applied := applyLocalFallback(cfg, d, res)
	return taskRouting{
		Class:           dec.Class,
		Source:          runpkg.RoutingSourcePolicy,
		Reasons:         routingReasons(dec.Reasons, res.Selection),
		Selection:       res.Selection,
		ExecutionTarget: executionTargetFor(res, applied),
		Signals:         sig,
		Checkpoints:     checkpoints,
	}, true, nil
}

// executionTargetFor records the resolved EXECUTION target for a routing result.
// It classifies the result as primary or availability-fallback from the EXPLICIT
// decision the caller made: `applied` is true only when applyLocalFallback
// replaced the selection with the class's configured availability fallback.
// Taking the decision as an argument (rather than inferring it from an unrelated
// Selection field) keeps the provenance explicit and cannot silently degrade if
// another code path later reuses a source value. It performs no probe.
//
// The routing CLASS is carried over UNCHANGED whether or not the fallback
// applied: a SMALL decision stays SMALL even when its SMALL cloud fallback runs.
func executionTargetFor(res model.Result, applied bool) model.ExecutionTarget {
	if applied && res.LocalFallback != nil {
		return model.ExecutionTargetForFallback(res.Selection, *res.LocalFallback)
	}
	return model.ExecutionTargetForPrimary(res.Selection)
}

// routingReasons returns a routing decision's reasons, with the local-fallback
// phrase appended when the class's configured runtime fallback supplied the
// selection. The appended phrase is a fixed constant, so the reason list stays
// deterministic and is never model-generated prose.
func routingReasons(reasons []string, sel model.Selection) []string {
	if sel.Source != model.SourceCloudFallback {
		return reasons
	}
	out := make([]string, 0, len(reasons)+1)
	out = append(out, reasons...)
	return append(out, model.ReasonLocalFallback)
}

// applyTaskRouting computes the routing decision for the task and, when it
// applies, replaces the implementation agent with one built for the selected
// class. It prints a concise routing block - the class, the resolved model, the
// source, the reasons, and the typed evidence - and returns the (possibly
// unchanged) agent, the decision, and any error. Nothing is printed when no
// routing applies.
//
// It is called immediately before implementation, after the pre-execution
// checkpoint, so the model is chosen from the freshest evidence and no expensive
// agent work precedes the decision. Building the agent for the selected class can
// fail (for example an unknown provider); that failure is surfaced rather than
// silently falling back to a different model (Phase 3.5 10).
//
// When providers.validate is opted in, the FINAL per-task selection - not the
// run-level default - is validated here, before any agent work, so a routed model
// the runtime cannot serve stops the task instead of failing mid-execution. The
// validation is read-only and never substitutes a provider or model.
//
// An escalated attempt (Phase 5) is the exception: its agent is already built,
// validated, and guarded by the recovery loop (runAttempts), so this only records
// the attempt's class and reason. See escalation.go.
func applyTaskRouting(ctx context.Context, cfg config.Config, d deps, spec *taskfile.Spec, tri, pre earlyGateResult, a agent.Agent, stdout io.Writer) (agent.Agent, *taskRouting, error) {
	tr, ok, err := routingForTask(cfg, d, spec, tri, pre)
	if err != nil {
		return a, nil, err
	}
	if !ok {
		// Routing selected no class, so THIS agent is the one that will execute the
		// implementation. Guard it here, at the final-selection seam, rather than only at
		// construction: with routing enabled the default agent is a fallback, and
		// rejecting it before routing would refuse work whose routed class could have
		// executed (Phase 5.4 36). Guarding here keeps the invariant that the agent
		// which actually runs supports IMPLEMENT, whether it is routed or default.
		if err := guardCapability(a, agent.Implement); err != nil {
			return a, nil, err
		}
		return a, nil, nil
	}
	if d.attempt != nil {
		// An escalated attempt's agent was already built, validated, and guarded by
		// the recovery loop (runAttempts), so the selected model already equals the
		// executing model. Rebuilding it here would construct a second agent for the
		// same class; the routing decision is still recorded so the attempt's class
		// and reason stay auditable.
		return a, &tr, nil
	}
	// Validate the final routed selection before it runs. This is the selection that
	// will execute the task, so it is the one provider validation must see.
	if err := validateSelection(ctx, cfg, tr.Selection); err != nil {
		return a, nil, fmt.Errorf("class %s selection: %w", tr.Class, err)
	}
	if d.newAgent == nil {
		// Fail closed (Phase 5 14): routing selected a different model, but without
		// an agent factory SOP cannot build it. Silently leaving the previous agent
		// in place would execute a model other than the one routing selected.
		return a, nil, fmt.Errorf("model routing: no agent factory is available to build the %s model (%s/%s)", tr.Class, tr.Selection.Provider, tr.Selection.Model)
	}
	ta, err := d.newAgent(cfg.Agent.Harness, tr.Selection.Provider, tr.Selection.Model)
	if err != nil {
		return a, nil, fmt.Errorf("model routing: build agent for class %s: %w", tr.Class, err)
	}
	if err := guardCapability(ta, agent.Implement); err != nil {
		return a, nil, err
	}
	// The operator-facing routing block: class, resolved model, source, reasons,
	// and the typed evidence the router used. It is rendered from the in-memory
	// decision (the sole source of truth) and is emitted only when routing applied.
	fmt.Fprint(stdout, routingClassLine(routingDocFor(&tr))+"\n")
	return ta, &tr, nil
}

// routingDoc is the serializable routing section of the run report.
type routingDoc struct {
	Class           model.Class            `json:"class"`
	Source          string                 `json:"source"`
	Reasons         []string               `json:"reasons,omitempty"`
	Provider        string                 `json:"provider,omitempty"`
	Model           string                 `json:"model,omitempty"`
	Locality        model.Locality         `json:"locality,omitempty"`
	ExecutionSource string                 `json:"execution_source,omitempty"`
	Checkpoints     []string               `json:"checkpoints,omitempty"`
	Signals         *runpkg.RoutingSignals `json:"signals,omitempty"`
}

// routingDocFor builds the report section from a decision, or nil when none.
//
// It projects the IN-MEMORY taskRouting (the sole source of truth) and never
// reads routing.json, so the report cannot be sourced from the persisted artifact.
// A nil result means routing did not apply, and every display surface renders
// nothing for it.
func routingDocFor(tr *taskRouting) *routingDoc {
	if tr == nil {
		return nil
	}
	return &routingDoc{
		Class:           tr.Class,
		Source:          string(tr.Source),
		Reasons:         tr.Reasons,
		Provider:        tr.Selection.Provider,
		Model:           tr.Selection.Model,
		Locality:        tr.Selection.Locality,
		ExecutionSource: executionSourceFor(tr),
		Checkpoints:     tr.Checkpoints,
		Signals:         routingSignalsDoc(tr.Signals),
	}
}

// executionSourceFor returns the typed execution source for a routing decision,
// defaulting to primary when no explicit target was recorded.
func executionSourceFor(tr *taskRouting) string {
	return tr.ExecutionTarget.Source.String()
}

// routingSignalsDoc projects the typed router signals into the persistable shape.
func routingSignalsDoc(s router.Signals) *runpkg.RoutingSignals {
	return &runpkg.RoutingSignals{
		JEVAvailable:       s.JEVAvailable,
		Risk:               string(s.Risk),
		Complexity:         string(s.Complexity),
		Scope:              string(s.Scope),
		CrossCutting:       s.CrossCutting,
		RequiresContext:    s.RequiresContext,
		Confidence:         s.Confidence,
		AcceptanceCriteria: s.AcceptanceCriteria,
		Dependencies:       s.Dependencies,
		FilesAffected:      s.FilesAffected,
	}
}

// writeRoutingDecisionArtifact persists the routing decision as the run's
// routing.json.
//
// It is WRITE-ONLY diagnostic evidence: nothing reads it back, and it is never a
// second source of truth - the model class always derives from the in-process
// router.Decide output, and no stage re-drives the decision from this file.
//
// The store writer fails closed on a contract violation (unknown version or
// source) and refuses to persist the artifact; the returned error is surfaced
// explicitly by the caller rather than silently discarded. A contract violation
// never changes the run's decision - the model choice in `tr` is already fixed -
// but it must not be mistaken for success.
//
// The persisted evidence keeps the routing CLASS unchanged and records the
// execution target beside it: the class is the routing DECISION, the provider/
// model/locality are the ACTUAL execution target, and execution_source is the
// typed provenance (primary or availability-fallback). A SMALL routing decision
// is never rewritten to MEDIUM because a SMALL availability fallback executed.
func writeRoutingDecisionArtifact(rn *runpkg.Run, taskID string, tr *taskRouting) error {
	if tr == nil {
		return nil
	}
	art := runpkg.RoutingArtifact{
		Version:         runpkg.RoutingArtifactVersion,
		Task:            taskID,
		Class:           string(tr.Class),
		Source:          tr.Source,
		Reasons:         tr.Reasons,
		Provider:        tr.Selection.Provider,
		Model:           tr.Selection.Model,
		Locality:        string(tr.Selection.Locality),
		ExecutionSource: executionSourceFor(tr),
		Checkpoints:     tr.Checkpoints,
		Signals:         routingSignalsDoc(tr.Signals),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}
	return rn.WriteRoutingArtifact(art)
}

// writeRoutingSummary renders the routing decision in `sop report`, so the class,
// the resolved model, the source, the reasons, and the typed evidence are
// auditable. It renders nothing when no routing decision was recorded, so an
// unrouted run prints no routing block.
func writeRoutingSummary(w io.Writer, r *routingDoc) {
	if r == nil {
		return
	}
	fmt.Fprintln(w, "Model routing:")
	fmt.Fprintf(w, "  %-16s %s\n", "Class:", r.Class)
	fmt.Fprintf(w, "  %-16s %s\n", "Provider:", r.Provider)
	fmt.Fprintf(w, "  %-16s %s\n", "Model:", r.Model)
	fmt.Fprintf(w, "  %-16s %s\n", "Locality:", r.Locality)
	fmt.Fprintf(w, "  %-16s %s\n", "Source:", r.Source)
	if r.ExecutionSource != "" {
		fmt.Fprintf(w, "  %-16s %s\n", "Execution source:", r.ExecutionSource)
	}
	if len(r.Reasons) > 0 {
		fmt.Fprintf(w, "  %-16s %s\n", "Reasons:", router.ReasonsText(r.Reasons))
	}
	if cps := routingCheckpointsText(r.Checkpoints); cps != "" {
		fmt.Fprintf(w, "  %-16s %s\n", "Checkpoints:", cps)
	}
	if ev := routingEvidenceLine(r.Signals); ev != "" {
		fmt.Fprintf(w, "  %-16s %s\n", "Evidence:", ev)
	}
	fmt.Fprintln(w)
}

// evidenceFrom is a small helper that returns a pointer to a copy of the evidence
// only when the checkpoint actually produced usable evidence, so an unavailable or
// failed analysis is never misread as a clean result.
func evidenceFrom(g earlyGateResult) *jev.Evidence {
	if !g.Ran || g.ProviderFailed {
		return nil
	}
	ev := g.Evidence
	return &ev
}

// routingDir resolves the project directory for evidence harvesting.
func routingDir(d deps) string {
	if d.getwd == nil {
		return ""
	}
	dir, err := d.getwd()
	if err != nil {
		return ""
	}
	return dir
}

// routingEvidence harvests deterministic routing evidence from completed runs: the model
// class each run executed at, its capability, and whether it reached a successful
// terminal. It reads only recorded evidence and invents nothing.
func routingEvidence(dir string) []adaptiveroute.Outcome {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, stateDirName, "runs"))
	if err != nil {
		return nil
	}
	var out []adaptiveroute.Outcome
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		out = append(out, outcomesFromTrace(filepath.Join(dir, stateDirName, "runs", entry.Name(), runtrace.FileName))...)
	}
	return out
}

// outcomesFromTrace reads one run trace and yields its routing outcome, when it recorded
// a run at a known model class.
func outcomesFromTrace(path string) []adaptiveroute.Outcome {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var tr runtrace.Trace
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil
	}
	class := model.Class(strings.ToLower(strings.TrimSpace(tr.Execution.ModelClass)))
	if !class.Valid() {
		return nil
	}
	capability := agent.Capability(strings.ToUpper(strings.TrimSpace(tr.Execution.Capability)))
	if capability == "" {
		return nil
	}
	return []adaptiveroute.Outcome{{Class: class, Capability: capability, Success: traceSucceeded(tr)}}
}

// traceSucceeded reports whether a trace reached a successful terminal. Only the PASSED
// stage counts, so a FAILED, BLOCKED, or human-boundary run is never counted as a success.
func traceSucceeded(tr runtrace.Trace) bool {
	return strings.EqualFold(strings.TrimSpace(tr.Termination.Stage), "PASSED")
}
