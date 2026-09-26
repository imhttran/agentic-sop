package taskbuilder

import (
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/domain"
)

func task(id string, deps ...string) *domain.Task {
	return &domain.Task{ID: id, DependencyIDs: deps}
}

func TestValidateDAGAcceptsAcyclicGraphs(t *testing.T) {
	cases := map[string][]*domain.Task{
		"single node": {task("A")},
		"linear":      {task("A"), task("B", "A"), task("C", "B")},
		"diamond":     {task("A"), task("B", "A"), task("C", "A"), task("D", "B", "C")},
		"forest":      {task("A"), task("B"), task("C", "A"), task("D", "B")},
	}
	for name, tasks := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDAG(tasks); err != nil {
				t.Errorf("expected valid, got %v", err)
			}
		})
	}
}

func TestValidateDAGRejectsCycles(t *testing.T) {
	cases := map[string][]*domain.Task{
		"direct cycle":  {task("A", "B"), task("B", "A")},
		"long cycle":    {task("A", "C"), task("B", "A"), task("C", "B")},
		"self cycle":    {task("A", "A")},
		"duplicate ids": {task("A"), task("A")},
	}
	for name, tasks := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDAG(tasks); err == nil {
				t.Errorf("expected cycle error for %s", name)
			}
		})
	}
}

func TestValidateDAGIsOrderIndependent(t *testing.T) {
	forward := []*domain.Task{task("A"), task("B", "A"), task("C", "A"), task("D", "B", "C")}
	reverse := []*domain.Task{task("D", "B", "C"), task("C", "A"), task("B", "A"), task("A")}

	if err := ValidateDAG(forward); err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	if err := ValidateDAG(reverse); err != nil {
		t.Fatalf("reverse failed: %v", err)
	}
}

func TestValidateDAGRejectsUnknownDependency(t *testing.T) {
	if err := ValidateDAG([]*domain.Task{task("A", "MISSING")}); err == nil {
		t.Error("expected error for unknown dependency")
	}
}
