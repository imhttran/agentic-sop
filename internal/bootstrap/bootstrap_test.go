package bootstrap

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/planner"
)

func task(id string, deps ...string) *domain.Task {
	return &domain.Task{ID: id, Title: id, Status: domain.PLANNED, DependencyIDs: deps}
}

func stage(id, kind string) planner.Stage {
	return planner.Stage{
		ID:                 id,
		Title:              id,
		Objective:          id,
		AcceptanceCriteria: []string{"done"},
		Kind:               kind,
	}
}

func TestApplyAddsEnvironmentDependency(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{
		stage("S000", planner.KindEnvironment),
		stage("S001", planner.KindFeature),
		stage("S002", ""),
	}}
	tasks := []*domain.Task{task("S000"), task("S001"), task("S002", "S001")}

	if err := Apply(plan, tasks); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(tasks[0].DependencyIDs) != 0 {
		t.Errorf("environment task dependencies = %v, want none", tasks[0].DependencyIDs)
	}
	if !reflect.DeepEqual(tasks[1].DependencyIDs, []string{"S000"}) {
		t.Errorf("S001 dependencies = %v, want [S000]", tasks[1].DependencyIDs)
	}
	if !reflect.DeepEqual(tasks[2].DependencyIDs, []string{"S001", "S000"}) {
		t.Errorf("S002 dependencies = %v, want [S001 S000]", tasks[2].DependencyIDs)
	}
}

func TestApplyNoEnvironmentStageIsNoOp(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{stage("S001", planner.KindFeature)}}
	tasks := []*domain.Task{task("S001")}

	if err := Apply(plan, tasks); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(tasks[0].DependencyIDs) != 0 {
		t.Errorf("dependencies = %v, want none", tasks[0].DependencyIDs)
	}
}

func TestApplyDoesNotDuplicateExistingDependency(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{
		stage("S000", planner.KindEnvironment),
		stage("S001", planner.KindFeature),
	}}
	tasks := []*domain.Task{task("S000"), task("S001", "S000")}

	if err := Apply(plan, tasks); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !reflect.DeepEqual(tasks[1].DependencyIDs, []string{"S000"}) {
		t.Errorf("S001 dependencies = %v, want [S000]", tasks[1].DependencyIDs)
	}
}

func TestApplyRejectsMultipleEnvironmentStages(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{
		stage("S000", planner.KindEnvironment),
		stage("S001", planner.KindEnvironment),
	}}
	if err := Apply(plan, []*domain.Task{task("S000"), task("S001")}); err == nil {
		t.Error("expected error for multiple environment stages")
	}
}

func TestApplyRejectsMissingEnvironmentTask(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{
		stage("S000", planner.KindEnvironment),
		stage("S001", planner.KindFeature),
	}}
	if err := Apply(plan, []*domain.Task{task("S001")}); err == nil {
		t.Error("expected error for a missing environment task")
	}
}

func TestApplyRejectsNilPlan(t *testing.T) {
	if err := Apply(nil, nil); err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestFeatureStaysBlockedUntilBootstrapDone(t *testing.T) {
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{
		stage("S000", planner.KindEnvironment),
		stage("S001", planner.KindFeature),
	}}
	tasks := []*domain.Task{task("S000"), task("S001")}
	if err := Apply(plan, tasks); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	byID := map[string]*domain.Task{"S000": tasks[0], "S001": tasks[1]}
	if _, satisfied := tasks[1].ResolveDependencies(byID); satisfied {
		t.Error("feature should be blocked while the bootstrap task is PLANNED")
	}

	tasks[0].Status = domain.DONE
	if _, satisfied := tasks[1].ResolveDependencies(byID); !satisfied {
		t.Error("feature should be ready once the bootstrap task is DONE")
	}
}
