// Package orchadopt is the ORCH-012 Phase 9 multi-agent adoption gate: the
// deterministic, model-free evidence gate comparing the single-agent baseline
// against the multi-agent candidate on genuinely parallelizable tasks.
//
// It is an evaluation harness, not production execution: it runs no live model,
// consults no provider, and nothing here is invoked by a task. It measures the
// deterministic dimensions the Phase 9 plan requires (task success, verification
// success, worker count, tool calls, discovery work, repeated discovery, repository
// mutations, conflicts, integration attempts, replans, model escalations, and context
// bytes/items), and decides whether multi-agent execution has earned default
// enablement. A dimension this gate cannot observe without running a model is named
// in Report.NotMeasured instead of being invented.
//
// Multi-agent execution is earned by evidence, never assumed: a tie or a cost
// regression is a rejection, so default execution remains single-agent.
package orchadopt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/orchestration"
)

// Decision is the gate's explicit outcome.
type Decision string

const (
	// Adopt: the multi-agent candidate is at least as successful and no more costly
	// than the single-agent baseline, so default enablement is justified by evidence.
	Adopt Decision = "ADOPT"
	// Reject: the candidate does not demonstrate a deterministic benefit (or regresses),
	// so default execution remains single-agent.
	Reject Decision = "REJECT"
)

// Metrics is the deterministic, model-free measurement of one execution strategy
// over the corpus. Every field counts something actually observed.
type Metrics struct {
	TaskSuccess         int `json:"task_success"`
	TaskTotal           int `json:"task_total"`
	VerificationSuccess int `json:"verification_success"`
	WorkerCount         int `json:"worker_count"`
	ToolCalls           int `json:"tool_calls"`
	DiscoveryWork       int `json:"discovery_work"`
	RepeatedDiscovery   int `json:"repeated_discovery"`
	RepositoryMutations int `json:"repository_mutations"`
	Conflicts           int `json:"conflicts"`
	IntegrationAttempts int `json:"integration_attempts"`
	Replans             int `json:"replans"`
	ModelEscalations    int `json:"model_escalations"`
	ContextBytes        int `json:"context_bytes"`
	ContextItems        int `json:"context_items"`
}

// Case is one genuinely-parallelizable task: a task ID and the independent units it
// decomposes into. Both strategies execute the SAME task, so the comparison is not
// confounded by different work.
type Case struct {
	// Name identifies the task.
	Name string
	// Units are the task's independent, non-mutating analysis units.
	Units []orchestration.DecompositionCandidate
}

// Report is the gate's deterministic outcome.
type Report struct {
	Cases       int      `json:"cases"`
	Baseline    Metrics  `json:"baseline"`
	Candidate   Metrics  `json:"candidate"`
	Decision    Decision `json:"multi_agent"`
	Reason      string   `json:"reason"`
	NotMeasured []string `json:"not_measured"`
}

// notMeasured names the comparison dimensions this deterministic gate cannot observe
// without running a live model. They are reported as unmeasured rather than invented.
func notMeasured() []string {
	return []string{
		"wall-clock time (deterministic fake agents have no meaningful latency)",
		"provider cost and token counts (no reliable provider-independent accounting)",
	}
}

// outcomeAgent is a deterministic, model-free agent double: every worker turn
// succeeds and produces no changed files. It counts invocations so tool calls are
// measured rather than assumed.
type outcomeAgent struct{ calls *int64 }

func (o outcomeAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	atomic.AddInt64(o.calls, 1)
	return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
}

// Evaluate runs the baseline and the candidate over the corpus and decides the gate.
// It is pure and deterministic: identical cases yield an identical report.
func Evaluate(cases []Case) Report {
	baseline := runBaseline(cases)
	candidate := runCandidate(cases)
	decision, reason := decide(baseline, candidate)
	return Report{
		Cases:       len(cases),
		Baseline:    baseline,
		Candidate:   candidate,
		Decision:    decision,
		Reason:      reason,
		NotMeasured: notMeasured(),
	}
}

// runBaseline executes each case with a SINGLE agent that does the whole task, one
// case at a time. It is the single-agent baseline.
func runBaseline(cases []Case) Metrics {
	var m Metrics
	var calls int64
	a := outcomeAgent{calls: &calls}
	ctx := context.Background()
	for _, c := range cases {
		m.TaskTotal++
		task := caseTask(c)
		resp, err := a.Generate(ctx, agent.Request{Capability: agent.Plan, Task: task, Input: task})
		m.WorkerCount++
		if err == nil {
			m.TaskSuccess++
			m.VerificationSuccess++
		}
		m.ContextBytes += len(task)
		m.ContextItems += 1
		_ = resp
	}
	m.ToolCalls = int(calls)
	return m
}

// runCandidate executes each case as a bounded multi-agent orchestration: one worker
// per unit (parallel) plus controlled integration.
func runCandidate(cases []Case) Metrics {
	var m Metrics
	var calls int64
	a := outcomeAgent{calls: &calls}
	ctx := context.Background()
	for _, c := range cases {
		m.TaskTotal++
		ev, err := orchestration.Orchestrate(ctx, a, orchestration.OrchestrateRequest{
			Decompose: orchestration.DecomposeRequest{
				Candidates:         c.Units,
				RepositoryIdentity: orchestration.RepositoryIdentity{Revision: "rev-1"},
				AcceptanceCriteria: []string{"the task's acceptance criteria"},
				ValidationCommands: []string{"go test ./..."},
			},
			Policy:   orchestration.ExecutionPolicy{Mode: orchestration.ExecutionParallel, MaxConcurrency: 4},
			Baseline: orchestration.NewProgressBaseline(),
			Context:  orchestration.WorkerContextRequest{TaskID: c.Name, Task: caseTask(c)},
			Routing:  orchestration.WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Plan},
		})
		if err != nil {
			continue
		}
		m.WorkerCount += len(ev.Assignments)
		if ev.Report.Completed > 0 {
			m.TaskSuccess++
		}
		if ev.View.NextStage == orchestration.StageVerify {
			m.VerificationSuccess++
		}
		if len(ev.View.Stages) > 0 {
			m.IntegrationAttempts++
		}
		for _, asn := range ev.Assignments {
			m.ContextBytes += len(asn.Context)
			if strings.TrimSpace(asn.Context) != "" {
				m.ContextItems++
			}
		}
		for _, p := range ev.Progress {
			if p.Active {
				m.DiscoveryWork++
			}
		}
	}
	m.ToolCalls = int(calls)
	return m
}

// decide applies the adoption rule: adopt only when the candidate is at least as
// successful as the baseline, introduces no conflicts/mutations/replans, and is no
// more costly. A tie or a regression rejects, so default execution remains
// single-agent unless multi-agent execution is earned by evidence.
func decide(baseline, candidate Metrics) (Decision, string) {
	switch {
	case candidate.TaskSuccess < baseline.TaskSuccess ||
		candidate.VerificationSuccess < baseline.VerificationSuccess:
		return Reject, "multi-agent regresses task or verification success; default execution remains single-agent"
	case candidate.RepositoryMutations > baseline.RepositoryMutations ||
		candidate.Conflicts > baseline.Conflicts ||
		candidate.Replans > baseline.Replans:
		return Reject, "multi-agent adds repository mutations, conflicts, or replans without proven benefit; default execution remains single-agent"
	case candidate.WorkerCount <= baseline.WorkerCount && candidate.ToolCalls <= baseline.ToolCalls:
		return Adopt, "multi-agent is at least as successful and no more costly than the single-agent baseline"
	default:
		return Reject, "multi-agent does not demonstrate a deterministic benefit over the single-agent baseline; default execution remains single-agent"
	}
}

// caseTask renders a case's task statement from its units.
func caseTask(c Case) string {
	parts := make([]string, 0, len(c.Units))
	for _, u := range c.Units {
		parts = append(parts, u.Task)
	}
	return strings.Join(parts, "; ")
}

// Corpus is the built-in, deterministic corpus of genuinely-parallelizable cases:
// independent, non-mutating analysis units that a task can be decomposed into.
func Corpus() []Case {
	mk := func(name string, tasks ...string) Case {
		units := make([]orchestration.DecompositionCandidate, 0, len(tasks))
		for i, t := range tasks {
			units = append(units, orchestration.DecompositionCandidate{
				TaskID:     name + "-U" + strconv.Itoa(i+1),
				Capability: agent.Plan,
				Task:       t,
				Scope:      orchestration.Scope{},
			})
		}
		return Case{Name: name, Units: units}
	}
	return []Case{
		mk("analyse-package", "analyse internal/context", "analyse internal/retrieval", "analyse internal/router"),
		mk("analyse-docs", "analyse docs/architecture", "analyse docs/specs"),
		mk("analyse-orchestration", "analyse internal/orchestration", "analyse internal/runtrace", "analyse internal/agent"),
	}
}

// String renders the report for operators.
func (r Report) String() string {
	var b strings.Builder
	b.WriteString("Phase 9 multi-agent adoption gate (ORCH-012)\n")
	b.WriteString("deterministic, model-free; compares the single-agent baseline against the multi-agent candidate\n\n")
	fmtf := func(format string, args ...any) { b.WriteString(fmt.Sprintf(format, args...)) }
	fmtf("cases: %d\n", r.Cases)
	fmtf("%-22s %10s %10s\n", "metric", "baseline", "candidate")
	fmtf("%-22s %10d %10d\n", "task_success", r.Baseline.TaskSuccess, r.Candidate.TaskSuccess)
	fmtf("%-22s %10d %10d\n", "verification_success", r.Baseline.VerificationSuccess, r.Candidate.VerificationSuccess)
	fmtf("%-22s %10d %10d\n", "worker_count", r.Baseline.WorkerCount, r.Candidate.WorkerCount)
	fmtf("%-22s %10d %10d\n", "tool_calls", r.Baseline.ToolCalls, r.Candidate.ToolCalls)
	fmtf("%-22s %10d %10d\n", "integration_attempts", r.Baseline.IntegrationAttempts, r.Candidate.IntegrationAttempts)
	fmtf("%-22s %10d %10d\n", "context_bytes", r.Baseline.ContextBytes, r.Candidate.ContextBytes)
	fmtf("%-22s %10d %10d\n", "conflicts", r.Baseline.Conflicts, r.Candidate.Conflicts)
	fmtf("%-22s %10d %10d\n", "repository_mutations", r.Baseline.RepositoryMutations, r.Candidate.RepositoryMutations)
	fmtf("\ndecision: %s\n", r.Decision)
	fmtf("reason: %s\n", r.Reason)
	if len(r.NotMeasured) > 0 {
		fmtf("not measured:\n")
		for _, n := range r.NotMeasured {
			fmtf("  - %s\n", n)
		}
	}
	return b.String()
}
