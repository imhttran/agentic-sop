package orchestration

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

func allCaps() agent.Capabilities { return agent.AllCapabilities() }

func TestDecideSingleForLoneTask(t *testing.T) {
	got := Decide(Request{
		Units:     []Unit{{ID: "t1", Capability: agent.Implement}},
		Available: allCaps(),
	})
	if got != ModeSingle {
		t.Fatalf("lone task: got %s, want SINGLE", got)
	}
}

func TestDecideSingleForEmpty(t *testing.T) {
	if got := Decide(Request{Available: allCaps()}); got != ModeSingle {
		t.Fatalf("empty: got %s, want SINGLE", got)
	}
}

func TestDecideSequentialForDependencyOrdered(t *testing.T) {
	got := Decide(Request{
		Units: []Unit{
			{ID: "t1", Capability: agent.DesignTests},
			{ID: "t2", Capability: agent.Implement},
			{ID: "t3", Capability: agent.Review},
		},
		Dependencies: map[string][]string{
			"t2": {"t1"},
			"t3": {"t2"},
		},
		Available: allCaps(),
	})
	if got != ModeSequential {
		t.Fatalf("dependency-ordered: got %s, want SEQUENTIAL", got)
	}
}

func TestDecideParallelForIndependent(t *testing.T) {
	got := Decide(Request{
		Units: []Unit{
			{ID: "a", Capability: agent.Implement},
			{ID: "b", Capability: agent.DesignTests},
			{ID: "c", Capability: agent.Review},
		},
		Available: allCaps(),
	})
	if got != ModeParallel {
		t.Fatalf("independent: got %s, want PARALLEL", got)
	}
}

func TestDecideDefaultsToSingle(t *testing.T) {
	cases := []struct {
		name string
		req  Request
	}{
		{
			name: "unsatisfiable capability",
			req: Request{
				Units: []Unit{
					{ID: "a", Capability: agent.Implement},
					{ID: "b", Capability: agent.Capability("UNKNOWN_CAP")},
				},
				Available: allCaps(),
			},
		},
		{
			name: "ambiguous blank id",
			req: Request{
				Units: []Unit{
					{ID: "", Capability: agent.Implement},
					{ID: "b", Capability: agent.Review},
				},
				Available: allCaps(),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.req); got != ModeSingle {
				t.Fatalf("got %s, want SINGLE", got)
			}
		})
	}
}

func TestDecideIsDeterministicAcrossOrder(t *testing.T) {
	reqA := Request{
		Units: []Unit{
			{ID: "a", Capability: agent.Implement},
			{ID: "b", Capability: agent.Review},
		},
		Available: allCaps(),
	}
	reqB := Request{
		Units: []Unit{
			{ID: "b", Capability: agent.Review},
			{ID: "a", Capability: agent.Implement},
		},
		Available: allCaps(),
	}
	for i := 0; i < 10; i++ {
		if Decide(reqA) != Decide(reqB) {
			t.Fatal("mode depends on unit order")
		}
	}
}

func TestDecideOverlappingUnitsSequential(t *testing.T) {
	got := Decide(Request{
		Units: []Unit{
			{ID: "a", Capability: agent.Implement},
			{ID: "a", Capability: agent.Review},
		},
		Available: allCaps(),
	})
	if got != ModeSequential {
		t.Fatalf("overlapping: got %s, want SEQUENTIAL", got)
	}
}

func TestModelBuiltFromDomainTasks(t *testing.T) {
	dep := &domain.Task{ID: "dep", Status: domain.LOCAL_DONE}
	task := &domain.Task{
		ID:            "t1",
		Status:        domain.PLANNED,
		DependencyIDs: []string{"dep"},
		ExecutionMode: domain.ExecutionImplement,
	}
	tasks := map[string]*domain.Task{"dep": dep, "t1": task}

	view := NewTaskView(task)
	if view.ID != "t1" || view.ExecutionMode != domain.ExecutionImplement {
		t.Fatalf("view projection wrong: %+v", view)
	}
	if view.Status() != domain.PLANNED {
		t.Fatalf("status must delegate to domain, got %s", view.Status())
	}
	if !view.IsRunnable() {
		t.Fatal("PLANNED task must be runnable via domain predicate")
	}
	unmet, ok := view.ResolveDependencies(tasks)
	if !ok || len(unmet) != 0 {
		t.Fatalf("dependencies should resolve: unmet=%v ok=%v", unmet, ok)
	}

	model := NewModel([]WorkUnit{{TaskID: "t1", Capability: agent.Implement}}, []string{"it compiles"}, []string{"go build ./..."})
	if model.Orchestrator.Role != RoleOrchestrator {
		t.Fatal("missing orchestrator role")
	}
	if len(model.Workers) != 1 || model.Workers[0].TaskID != "t1" {
		t.Fatalf("workers not built: %+v", model.Workers)
	}
	if model.Integrator.Role != RoleIntegrator || model.Reviewer.Role != RoleReviewer || model.Verification.Role != RoleVerification {
		t.Fatal("post-step roles missing")
	}
}
