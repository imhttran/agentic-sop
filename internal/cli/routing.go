package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// Deterministic per-task model routing (Phase 3.5).
//
// The router (internal/router) maps typed evidence — the task's structure and the
// early-JEV checkpoints' typed evidence — to one of the configured model classes,
// and the model layer resolves that class to a concrete provider/model. This file
// is the seam that computes the decision, records it, and (only) selects the model
// for the task's implementation agent.
//
// It carries no lifecycle authority: routing never approves, blocks, transitions
// state, or bypasses a gate. It only selects which configured model runs the
// bounded implementation work. A manual --model-class override always wins over
// the automatic router, so an operator's explicit choice is never silently
// replaced (Phase 3.5 §6).

// taskRouting is the per-task routing decision: the resolved class selection plus
// the deterministic provenance (source, reasons, the typed signals used, and which
// checkpoints informed it). It is the SOLE source of truth for the model choice:
// it is derived in memory by routingForTask and never reconstructed from the
// persisted routing.json. Nothing reads the artifact back to drive a decision.
type taskRouting struct {
	Class       model.Class
	Source      runpkg.RoutingSource
	Reasons     []string
	Selection   model.Selection
	Signals     router.Signals
	Checkpoints []string
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
			Class:     d.attempt.Decision.ToClass,
			Source:    runpkg.RoutingSourceEscalation,
			Reasons:   []string{d.attempt.Decision.Reason},
			Selection: d.attempt.Selection,
		}, true, nil
	}
	if strings.TrimSpace(d.modelClass) != "" {
		if d.routing.Active {
			// A manual override pins the CLASS; it does not pin the model within it,
			// so the local-first fallback applies to an explicitly chosen class too.
			res := applyLocalFallback(cfg, d, d.routing)
			return taskRouting{
				Class:     res.Selection.Class,
				Source:    runpkg.RoutingSourceManual,
				Reasons:   routingReasons([]string{model.RoutingReasonManual}, res.Selection),
				Selection: res.Selection,
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
	res = applyLocalFallback(cfg, d, res)
	return taskRouting{
		Class:       dec.Class,
		Source:      runpkg.RoutingSourcePolicy,
		Reasons:     routingReasons(dec.Reasons, res.Selection),
		Selection:   res.Selection,
		Signals:     sig,
		Checkpoints: checkpoints,
	}, true, nil
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
// class. It prints a concise routing block — the class, the resolved model, the
// source, the reasons, and the typed evidence — and returns the (possibly
// unchanged) agent, the decision, and any error. Nothing is printed when no
// routing applies.
//
// It is called immediately before implementation, after the pre-execution
// checkpoint, so the model is chosen from the freshest evidence and no expensive
// agent work precedes the decision. Building the agent for the selected class can
// fail (for example an unknown provider); that failure is surfaced rather than
// silently falling back to a different model (Phase 3.5 §10).
//
// When providers.validate is opted in, the FINAL per-task selection — not the
// run-level default — is validated here, before any agent work, so a routed model
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
		// executed (Phase 5.4 §36). Guarding here keeps the invariant that the agent
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
		// Fail closed (Phase 5 §14): routing selected a different model, but without
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
	Class       model.Class            `json:"class"`
	Source      string                 `json:"source"`
	Reasons     []string               `json:"reasons,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Locality    model.Locality         `json:"locality,omitempty"`
	Checkpoints []string               `json:"checkpoints,omitempty"`
	Signals     *runpkg.RoutingSignals `json:"signals,omitempty"`
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
		Class:       tr.Class,
		Source:      string(tr.Source),
		Reasons:     tr.Reasons,
		Provider:    tr.Selection.Provider,
		Model:       tr.Selection.Model,
		Locality:    tr.Selection.Locality,
		Checkpoints: tr.Checkpoints,
		Signals:     routingSignalsDoc(tr.Signals),
	}
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
// second source of truth — the model class always derives from the in-process
// router.Decide output, and no stage re-drives the decision from this file.
//
// The store writer fails closed on a contract violation (unknown version or
// source) and refuses to persist the artifact; the returned error is surfaced
// explicitly by the caller rather than silently discarded. A contract violation
// never changes the run's decision — the model choice in `tr` is already fixed —
// but it must not be mistaken for success.
func writeRoutingDecisionArtifact(rn *runpkg.Run, taskID string, tr *taskRouting) error {
	if tr == nil {
		return nil
	}
	art := runpkg.RoutingArtifact{
		Version:     runpkg.RoutingArtifactVersion,
		Task:        taskID,
		Class:       string(tr.Class),
		Source:      tr.Source,
		Reasons:     tr.Reasons,
		Provider:    tr.Selection.Provider,
		Model:       tr.Selection.Model,
		Locality:    string(tr.Selection.Locality),
		Checkpoints: tr.Checkpoints,
		Signals:     routingSignalsDoc(tr.Signals),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
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
	fmt.Fprintf(w, "  %-11s %s\n", "Class:", r.Class)
	fmt.Fprintf(w, "  %-11s %s\n", "Provider:", r.Provider)
	fmt.Fprintf(w, "  %-11s %s\n", "Model:", r.Model)
	fmt.Fprintf(w, "  %-11s %s\n", "Locality:", r.Locality)
	fmt.Fprintf(w, "  %-11s %s\n", "Source:", r.Source)
	if len(r.Reasons) > 0 {
		fmt.Fprintf(w, "  %-11s %s\n", "Reasons:", router.ReasonsText(r.Reasons))
	}
	if cps := routingCheckpointsText(r.Checkpoints); cps != "" {
		fmt.Fprintf(w, "  %-11s %s\n", "Checkpoints:", cps)
	}
	if ev := routingEvidenceLine(r.Signals); ev != "" {
		fmt.Fprintf(w, "  %-11s %s\n", "Evidence:", ev)
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
