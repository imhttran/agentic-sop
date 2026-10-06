package orchestration

// ORCH-010 Context & Routing Integration tests. They are deterministic and
// model-free: they use the CTX-001 Context Engine, the CTX-011 Adaptive Routing
// policy, and the deterministic in-package fakes only. No provider, model,
// network, or filesystem is invoked.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/adaptiveroute"
	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/model"
)

// captureAgent is a deterministic agent double that records the request it
// receives, so a test can prove which context and capability crossed the
// provider-neutral boundary.
type captureAgent struct {
	request  agent.Request
	response agent.Response
}

func (c *captureAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	c.request = r
	return c.response, nil
}

// workerFixtureAssignment builds a valid, provider-neutral assignment.
func workerFixtureAssignment() WorkAssignment {
	b := AssignmentBuilder{
		AssignmentID:       "A1",
		TaskID:             "T1",
		Capability:         agent.Implement,
		Task:               "implement the change",
		Scope:              Scope{Paths: []string{"internal/orchestration"}},
		Context:            "caller-supplied pre-existing context",
		AllowedTools:       []string{"read_file"},
		Budget:             Budget{MaxSteps: 5},
		OutputContract:     OutputContract{Format: "json"},
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
		AcceptanceCriteria: []string{"ac-1"},
		ValidationCommands: []string{"go test ./..."},
	}
	return b.Build()
}

func hasSource(sc []sopctx.SourceCount, s sopctx.Source) bool {
	for _, c := range sc {
		if c.Source == s {
			return true
		}
	}
	return false
}

// TestBuildWorkerContextIsBoundedAndScoped proves a worker receives bounded,
// scoped context with provenance, never the whole repository.
func TestBuildWorkerContextIsBoundedAndScoped(t *testing.T) {
	var retrieved []sopctx.Item
	for i := 0; i < 50; i++ {
		retrieved = append(retrieved, sopctx.Item{
			Source:   sopctx.SourceRepository,
			Identity: "internal/pkg/file" + strconv.Itoa(i) + ".go",
			Reason:   "retrieved evidence",
			Priority: sopctx.PriorityRepository,
			Text:     strings.Repeat("x", 200),
		})
	}
	wc := BuildWorkerContext(WorkerContextRequest{
		AssignmentID:       "A1",
		TaskID:             "T1",
		Task:               "implement the change",
		Plan:               "the plan",
		AcceptanceCriteria: []string{"ac-1"},
		ScopePaths:         []string{"internal/orchestration"},
		Retrieved:          retrieved,
		Limits:             sopctx.Limits{MaxItems: 8, MaxFiles: 4, MaxBytes: 1024},
	})
	if wc.Items > 8 {
		t.Errorf("items = %d, want <= 8", wc.Items)
	}
	if wc.Bytes > 1024 {
		t.Errorf("bytes = %d, want <= 1024", wc.Bytes)
	}
	if wc.Files > 4 {
		t.Errorf("files = %d, want <= 4", wc.Files)
	}
	if !wc.Truncated {
		t.Error("expected truncation under a tight bound")
	}
	if wc.Rendered == "" {
		t.Error("rendered context is empty")
	}
	if !hasSource(wc.Sources, sopctx.SourceTask) || !hasSource(wc.Sources, sopctx.SourceRepository) {
		t.Errorf("provenance missing task/repository: %+v", wc.Sources)
	}
}

// TestWorkerContextProvenanceOrderIsCanonical proves the per-source provenance is
// reported deterministically in the Context Engine's canonical source order.
func TestWorkerContextProvenanceOrderIsCanonical(t *testing.T) {
	wc := BuildWorkerContext(WorkerContextRequest{
		AssignmentID: "A1",
		TaskID:       "T1",
		Task:         "do the work",
		ScopePaths:   []string{"internal/orchestration"},
		Memory:       []sopctx.Item{{Source: sopctx.SourceMemory, Identity: "d1", Text: "guidance"}},
	})
	idx := map[sopctx.Source]int{}
	for i, c := range wc.Sources {
		idx[c.Source] = i
	}
	task, okTask := idx[sopctx.SourceTask]
	repo, okRepo := idx[sopctx.SourceRepository]
	mem, okMem := idx[sopctx.SourceMemory]
	if !okTask || !okRepo || !okMem {
		t.Fatalf("expected task, repository, and memory provenance: %+v", wc.Sources)
	}
	if !(task < repo && repo < mem) {
		t.Errorf("provenance order not canonical: %+v", wc.Sources)
	}
}

// TestWorkerContextMemoryNeverOutranksRepository proves stale decision memory
// cannot outrank current repository evidence when the context is bounded: memory is
// forced to the lowest priority and is dropped first.
func TestWorkerContextMemoryNeverOutranksRepository(t *testing.T) {
	repo := sopctx.Item{Source: sopctx.SourceRepository, Identity: "internal/orch/x.go", Priority: sopctx.PriorityRepository, Text: "REPO EVIDENCE"}
	// The caller supplies the memory item with the HIGHEST priority; the integration
	// must still force it below repository evidence.
	mem := sopctx.Item{Source: sopctx.SourceMemory, Identity: "decision-1", Priority: sopctx.PriorityExplicit, Text: "STALE MEMORY GUIDANCE"}
	wc := BuildWorkerContext(WorkerContextRequest{
		AssignmentID: "A1",
		TaskID:       "T1",
		Task:         "do the work",
		Retrieved:    []sopctx.Item{repo},
		Memory:       []sopctx.Item{mem},
		Limits:       sopctx.Limits{MaxItems: 2},
	})
	if !strings.Contains(wc.Rendered, "REPO EVIDENCE") {
		t.Errorf("repository evidence must survive: %q", wc.Rendered)
	}
	if strings.Contains(wc.Rendered, "STALE MEMORY GUIDANCE") {
		t.Errorf("stale memory outranked repository evidence: %q", wc.Rendered)
	}
}

// TestRouteWorkerBaselineWithoutEvidence proves the baseline class is retained when
// there is no evidence to adapt on.
func TestRouteWorkerBaselineWithoutEvidence(t *testing.T) {
	r := RouteWorker(WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Implement})
	if r.Class != model.ClassMedium {
		t.Errorf("class = %q, want baseline MEDIUM", r.Class)
	}
	if r.Baseline != model.ClassMedium {
		t.Errorf("baseline = %q", r.Baseline)
	}
	if r.Reason != adaptiveroute.ReasonNoEvidence {
		t.Errorf("reason = %q, want %q", r.Reason, adaptiveroute.ReasonNoEvidence)
	}
}

// TestRouteWorkerAdaptiveEscalatesClassOnly proves adaptive routing changes only the
// canonical model class (never a provider or model) and preserves the baseline.
func TestRouteWorkerAdaptiveEscalatesClassOnly(t *testing.T) {
	var evidence []adaptiveroute.Outcome
	for i := 0; i < 3; i++ {
		evidence = append(evidence, adaptiveroute.Outcome{Class: model.ClassMedium, Capability: agent.Implement, Success: false})
	}
	r := RouteWorker(WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Implement, Evidence: evidence})
	if r.Class == model.ClassMedium {
		t.Errorf("expected escalation above MEDIUM, got %q", r.Class)
	}
	if !r.Class.Valid() {
		t.Errorf("class %q is not a canonical model class", r.Class)
	}
	if r.Baseline != model.ClassMedium {
		t.Errorf("baseline must be preserved: %q", r.Baseline)
	}
	if r.Evidence == "" {
		t.Error("adaptive decision must expose its evidence summary")
	}
}

// TestRouteWorkerOverrideWins proves an explicit operator override is authoritative.
func TestRouteWorkerOverrideWins(t *testing.T) {
	r := RouteWorker(WorkerRoutingRequest{Baseline: model.ClassSmall, Capability: agent.Plan, Override: model.ClassLarge})
	if r.Class != model.ClassLarge || r.Reason != adaptiveroute.ReasonOverride {
		t.Errorf("override not honoured: %+v", r)
	}
}

// TestPrepareWorkerPreservesAuthority proves routing and context selection cannot
// expand a worker's authority: only Context and ModelClass change.
func TestPrepareWorkerPreservesAuthority(t *testing.T) {
	before := workerFixtureAssignment()
	prep := PrepareWorker(WorkerPreparationRequest{
		Assignment: before,
		Context:    WorkerContextRequest{Plan: "the plan", AcceptanceCriteria: []string{"ac-1"}},
		Routing:    WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: before.Capability},
	})
	after := prep.Assignment

	if !reflect.DeepEqual(after.Scope, before.Scope) {
		t.Errorf("scope changed: %+v -> %+v", before.Scope, after.Scope)
	}
	if !reflect.DeepEqual(after.AllowedTools, before.AllowedTools) {
		t.Errorf("allowed tools changed: %v -> %v", before.AllowedTools, after.AllowedTools)
	}
	if after.Budget != before.Budget {
		t.Errorf("budget changed: %+v -> %+v", before.Budget, after.Budget)
	}
	if after.Capability != before.Capability {
		t.Errorf("capability changed: %q -> %q", before.Capability, after.Capability)
	}
	if after.RepositoryIdentity != before.RepositoryIdentity {
		t.Errorf("repository identity changed")
	}
	if !reflect.DeepEqual(after.OutputContract, before.OutputContract) {
		t.Errorf("output contract changed")
	}
	if !reflect.DeepEqual(after.AcceptanceCriteria, before.AcceptanceCriteria) {
		t.Errorf("acceptance criteria changed")
	}
	if !reflect.DeepEqual(after.ValidationCommands, before.ValidationCommands) {
		t.Errorf("validation commands changed")
	}

	// The routing decision is carried OUTSIDE the provider-neutral assignment
	// contract (ORCH-002 forbids model/provider-named fields there): the class is
	// returned as data for the caller to resolve behind the adapter boundary.
	if !prep.Route.Class.Valid() {
		t.Errorf("route class %q is not canonical", prep.Route.Class)
	}
	if after.Context != prep.Context.Rendered {
		t.Errorf("assignment context does not equal the prepared bounded context")
	}
}

// TestPreparedAssignmentFlowsThroughCoordinator proves the prepared assignment
// crosses the existing worker-adapter boundary with the bounded context and its
// capability intact.
func TestPreparedAssignmentFlowsThroughCoordinator(t *testing.T) {
	before := workerFixtureAssignment()
	prep := PrepareWorker(WorkerPreparationRequest{
		Assignment: before,
		Context:    WorkerContextRequest{Plan: "the plan"},
		Routing:    WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: before.Capability},
	})
	ag := &captureAgent{response: agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "done"}}}
	report := NewCoordinator(ag).Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{prep.Assignment})
	if !report.Succeeded() {
		t.Fatalf("report not successful: %+v", report)
	}
	if ag.request.Input != prep.Context.Rendered {
		t.Errorf("worker did not receive the bounded context:\n got %q\nwant %q", ag.request.Input, prep.Context.Rendered)
	}
	if ag.request.Capability != before.Capability {
		t.Errorf("capability changed across the boundary: %q", ag.request.Capability)
	}
}

// TestOrchestrationIntegrationIsOptIn proves the unprepared assignment path is
// unchanged: without PrepareWorker, the caller's context is passed through verbatim
// and no model class is attached.
func TestOrchestrationIntegrationIsOptIn(t *testing.T) {
	a := workerFixtureAssignment()
	ag := &captureAgent{response: agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "done"}}}
	report := NewCoordinator(ag).Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{a})
	if !report.Succeeded() {
		t.Fatalf("report not successful: %+v", report)
	}
	if ag.request.Input != a.Context {
		t.Errorf("unprepared context not passed through: got %q want %q", ag.request.Input, a.Context)
	}
}

// TestContextRoutingSourceIsProviderNeutral proves the integration file imports
// only provider-neutral packages and the standard library.
func TestContextRoutingSourceIsProviderNeutral(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "context_routing.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse context_routing.go: %v", err)
	}
	allowed := map[string]bool{
		"sort":    true,
		"strings": true,
		"github.com/imhttran/agentic-sop/internal/adaptiveroute": true,
		"github.com/imhttran/agentic-sop/internal/agent":         true,
		"github.com/imhttran/agentic-sop/internal/context":       true,
		"github.com/imhttran/agentic-sop/internal/model":         true,
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, "\"")
		if !allowed[p] {
			t.Fatalf("context_routing.go imports %q; only provider-neutral packages are permitted", p)
		}
	}
}

// TestOrchestrationCoreHasNoProviderLiterals proves the orchestration core contains
// no provider-specific string literal (for example a branch on a provider name). It
// scans parsed string literals, so comments are ignored.
func TestOrchestrationCoreHasNoProviderLiterals(t *testing.T) {
	providers := []string{"ollama", "openai", "anthropic", "claude", "deepseek", "qwen", "nemotron", "mlx", "laya", "nimble"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			bl, ok := n.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			val := strings.ToLower(strings.Trim(bl.Value, "\"`"))
			for _, p := range providers {
				if strings.Contains(val, p) {
					t.Errorf("%s contains provider-specific string literal %s", name, bl.Value)
				}
			}
			return true
		})
	}
}
