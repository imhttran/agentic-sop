package cli

import (
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
// checkpoints informed it). It is diagnostic evidence; nothing reads it back to
// drive a decision beyond the model choice it records.
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
	if strings.TrimSpace(d.modelClass) != "" {
		if d.routing.Active {
			return taskRouting{
				Class:     d.routing.Selection.Class,
				Source:    runpkg.RoutingSourceManual,
				Reasons:   []string{model.RoutingReasonManual},
				Selection: d.routing.Selection,
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
	return taskRouting{
		Class:       dec.Class,
		Source:      runpkg.RoutingSourcePolicy,
		Reasons:     dec.Reasons,
		Selection:   res.Selection,
		Signals:     sig,
		Checkpoints: checkpoints,
	}, true, nil
}

// applyTaskRouting computes the routing decision for the task and, when it
// applies, replaces the implementation agent with one built for the selected
// class. It prints a single concise routing line and returns the (possibly
// unchanged) agent, the decision, and any error.
//
// It is called immediately before implementation, after the pre-execution
// checkpoint, so the model is chosen from the freshest evidence and no expensive
// agent work precedes the decision. Building the agent for the selected class can
// fail (for example an unknown provider); that failure is surfaced rather than
// silently falling back to a different model (Phase 3.5 §10).
func applyTaskRouting(cfg config.Config, d deps, spec *taskfile.Spec, tri, pre earlyGateResult, a agent.Agent, stdout io.Writer) (agent.Agent, *taskRouting, error) {
	tr, ok, err := routingForTask(cfg, d, spec, tri, pre)
	if err != nil {
		return a, nil, err
	}
	if !ok {
		return a, nil, nil
	}
	if d.newAgent == nil {
		// Without an agent factory there is nothing to rebuild; leave the existing
		// agent rather than failing the run.
		return a, nil, nil
	}
	ta, err := d.newAgent(cfg.Agent.Harness, tr.Selection.Provider, tr.Selection.Model)
	if err != nil {
		return a, nil, fmt.Errorf("model routing: build agent for class %s: %w", tr.Class, err)
	}
	if err := guardCapability(ta, agent.Implement); err != nil {
		return a, nil, err
	}
	fmt.Fprintf(stdout, "Task routing: %s (%s; %s)\n", tr.Class, tr.Selection.Model, router.ReasonsText(tr.Reasons))
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
// routing.json, best-effort. It is diagnostic evidence: nothing reads it back.
func writeRoutingDecisionArtifact(rn *runpkg.Run, taskID string, tr *taskRouting) {
	if tr == nil {
		return
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
	_ = rn.WriteRoutingArtifact(art)
}

// writeRoutingSummary renders the routing decision in `sop report`, so the class,
// the resolved model, and the reason are auditable. It renders nothing when no
// routing decision was recorded.
func writeRoutingSummary(w io.Writer, r *routingDoc) {
	if r == nil {
		return
	}
	fmt.Fprintln(w, "Model routing:")
	fmt.Fprintf(w, "  %-11s %s\n", "Class:", r.Class)
	fmt.Fprintf(w, "  %-11s %s\n", "Model:", r.Model)
	fmt.Fprintf(w, "  %-11s %s\n", "Source:", r.Source)
	if len(r.Reasons) > 0 {
		fmt.Fprintf(w, "  %-11s %s\n", "Reasons:", router.ReasonsText(r.Reasons))
	}
	if len(r.Checkpoints) > 0 {
		fmt.Fprintf(w, "  %-11s %s\n", "Evidence:", strings.Join(r.Checkpoints, ", "))
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
