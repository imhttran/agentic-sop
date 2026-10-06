package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/orchestration"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runOrchestrate runs the opt-in, bounded Phase 9 multi-agent execution path
// (ORCH-011) for one task file and prints the deterministic evidence it produced.
//
// It is OFF by default: it refuses unless orchestration.enabled is set, so
// single-agent execution (`sop run`) is unchanged unless an operator explicitly opts
// in. It performs no lifecycle transition, approval, commit, verification, or model-
// routing change: the workers are analysis/proposal workers whose results are
// claims, and the integrated view hands off to centralized verification.
func runOrchestrate(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "usage: sop orchestrate TASK.md")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return exitError
	}
	if !cfg.Orchestration.Enabled {
		fmt.Fprintln(stderr, "orchestrate: multi-agent execution is disabled; set orchestration.enabled: true to opt in (single-agent `sop run` is unchanged)")
		return exitError
	}
	spec, err := loadTaskFile(dir, args[0])
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return exitError
	}
	if spec.ExecutionMode.Done() {
		fmt.Fprintln(stderr, "orchestrate: execution_mode `done` declares the work already exists; nothing to orchestrate")
		return exitError
	}

	head, _ := repoState(dir)
	if head == "" {
		fmt.Fprintln(stderr, "orchestrate: cannot determine the repository HEAD; orchestration requires a repository identity")
		return exitError
	}

	a, err := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return exitError
	}

	revision := orchestration.RepositoryIdentity{Revision: head}
	ev, err := orchestration.Orchestrate(context.Background(), a, orchestrateRequest(cfg, spec, revision))
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return exitError
	}

	printOrchestrationEvidence(stdout, spec, ev)
	return exitOK
}

// orchestrateRequest assembles the deterministic opt-in request from config and the
// task file. The initial production policy is parallel reasoning/proposal (a
// non-mutating analysis unit with a deny-all write scope) plus controlled
// integration, per the Phase 9 plan.
func orchestrateRequest(cfg config.Config, spec *taskfile.Spec, revision orchestration.RepositoryIdentity) orchestration.OrchestrateRequest {
	budget := orchestration.Defaults()
	if cfg.Orchestration.MaxAssignments > 0 {
		budget.MaxAssignments = cfg.Orchestration.MaxAssignments
	}
	if cfg.Orchestration.MaxWorkers > 0 {
		budget.MaxWorkers = cfg.Orchestration.MaxWorkers
	}
	if cfg.Orchestration.MaxActiveWorkers > 0 {
		budget.MaxActiveWorkers = cfg.Orchestration.MaxActiveWorkers
	}
	policy := orchestration.ExecutionPolicy{
		Mode:           orchestration.ExecutionParallel,
		MaxConcurrency: cfg.Orchestration.MaxConcurrency,
		Ownership:      orchestration.OwnershipSingleWriter,
		Budget:         budget,
	}
	return orchestration.OrchestrateRequest{
		Decompose: orchestration.DecomposeRequest{
			Candidates: []orchestration.DecompositionCandidate{{
				TaskID:     spec.ID,
				Capability: agent.Plan,
				Task:       spec.Render(),
				Scope:      orchestration.Scope{},
			}},
			Limit:              cfg.Orchestration.DecompositionLimit,
			RepositoryIdentity: revision,
			AcceptanceCriteria: spec.AcceptanceCriteria,
			ValidationCommands: completionValidationCommands(cfg),
		},
		Policy:   policy,
		Conflict: orchestration.ConflictInput{CurrentIdentity: revision},
		Baseline: orchestration.NewProgressBaseline(),
		Context: orchestration.WorkerContextRequest{
			TaskID:             spec.ID,
			Task:               spec.Render(),
			AcceptanceCriteria: spec.AcceptanceCriteria,
		},
		Routing: orchestration.WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Plan},
	}
}

// printOrchestrationEvidence renders the deterministic evidence one opt-in run
// produced. Nothing here is a verdict: worker results are claims and the integrated
// view is non-authoritative.
func printOrchestrationEvidence(w io.Writer, spec *taskfile.Spec, ev orchestration.OrchestrateEvidence) {
	fmt.Fprintln(w, "SOP multi-agent orchestration (opt-in)")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Task: %s\n", spec.ID)
	fmt.Fprintf(w, "Assignments: %d\n", len(ev.Assignments))
	for i, asn := range ev.Assignments {
		class := ""
		if i < len(ev.Routes) {
			class = string(ev.Routes[i].Class)
		}
		fmt.Fprintf(w, "  - %s capability=%s class=%s\n", asn.AssignmentID, asn.Capability, class)
	}
	for _, o := range ev.Report.Outcomes {
		if o.Failure == orchestration.FailureNone {
			fmt.Fprintf(w, "  %s: %s\n", o.AssignmentID, o.Result.Status)
		} else {
			fmt.Fprintf(w, "  %s: rejected (%s)\n", o.AssignmentID, o.Failure)
		}
	}
	fmt.Fprintf(w, "completed=%d needs_human=%d failed=%d cancelled=%d errored=%d\n",
		ev.Report.Completed, ev.Report.NeedsHuman, ev.Report.Failed, ev.Report.Cancelled, ev.Report.Errored)
	fmt.Fprintf(w, "integration: %d claim(s), next stage %s, authoritative=%t\n", len(ev.View.Claims), ev.View.NextStage, ev.View.Authoritative)
	fmt.Fprintln(w, "note: claims are non-authoritative; centralized verification remains authoritative")
}
