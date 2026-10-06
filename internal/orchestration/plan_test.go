package orchestration

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

func TestPlanModeSingleTaskUnchanged(t *testing.T) {
	task := &domain.Task{ID: "t1", Status: domain.PLANNED, ExecutionMode: domain.ExecutionImplement}
	got := PlanMode(
		[]*domain.Task{task},
		map[string]agent.Capability{"t1": agent.Implement},
		agent.AllCapabilities(),
	)
	if got != ModeSingle {
		t.Fatalf("existing single-task plan: got %s, want SINGLE", got)
	}
}

func TestPlanModeSurfacesSequentialAndParallelAsMetadata(t *testing.T) {
	seq := []*domain.Task{
		{ID: "a", Status: domain.PLANNED, DependencyIDs: []string{"b"}},
		{ID: "b", Status: domain.PLANNED},
	}
	got := PlanMode(seq, map[string]agent.Capability{"a": agent.Implement, "b": agent.DesignTests}, agent.AllCapabilities())
	if got != ModeSequential {
		t.Fatalf("ordered plan: got %s, want SEQUENTIAL", got)
	}

	par := []*domain.Task{
		{ID: "a", Status: domain.PLANNED},
		{ID: "b", Status: domain.PLANNED},
	}
	got = PlanMode(par, map[string]agent.Capability{"a": agent.Implement, "b": agent.Review}, agent.AllCapabilities())
	if got != ModeParallel {
		t.Fatalf("independent plan: got %s, want PARALLEL", got)
	}
}
