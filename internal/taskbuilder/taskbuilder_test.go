package taskbuilder

import (
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/planner"
)

func validPlan() *planner.Plan {
	return &planner.Plan{
		Project: "Book RAG",
		Summary: "Build it.",
		Stages: []planner.Stage{
			{
				ID:                 "S001",
				Title:              "Application skeleton",
				Objective:          "Create the Go application skeleton.",
				AcceptanceCriteria: []string{"application starts", "no runtime errors"},
			},
			{
				ID:                 "S002",
				Title:              "Ingestion",
				Objective:          "Ingest books into the index.",
				Dependencies:       []string{"S001"},
				Deliverables:       []string{"Ingester"},
				AcceptanceCriteria: []string{"a book can be ingested"},
			},
		},
	}
}

func TestBuildMapsStagesToTasks(t *testing.T) {
	tasks, err := Build(validPlan())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}

	// Stage order is preserved.
	if tasks[0].ID != "S001" || tasks[1].ID != "S002" {
		t.Errorf("task order = [%s %s], want [S001 S002]", tasks[0].ID, tasks[1].ID)
	}

	first := tasks[0]
	if first.Title != "Application skeleton" || first.Objective != "Create the Go application skeleton." {
		t.Errorf("title/objective not preserved: %+v", first)
	}
	if len(first.DependencyIDs) != 0 {
		t.Errorf("S001 dependencies = %v, want none", first.DependencyIDs)
	}

	second := tasks[1]
	if !reflect.DeepEqual(second.DependencyIDs, []string{"S001"}) {
		t.Errorf("S002 dependencies = %v, want [S001]", second.DependencyIDs)
	}
}

func TestBuildInitializesWorkflowFields(t *testing.T) {
	tasks, err := Build(validPlan())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	for _, task := range tasks {
		if task.Status != domain.PLANNED {
			t.Errorf("%s status = %s, want PLANNED", task.ID, task.Status)
		}
		if task.Attempt != 0 {
			t.Errorf("%s attempt = %d, want 0", task.ID, task.Attempt)
		}
		if task.MaxAttempts != domain.DefaultRetryPolicy().MaxAttempts {
			t.Errorf("%s max attempts = %d, want %d", task.ID, task.MaxAttempts, domain.DefaultRetryPolicy().MaxAttempts)
		}
		if task.BlockedReason != domain.NO_REASON {
			t.Errorf("%s blocked reason = %q, want empty", task.ID, task.BlockedReason)
		}
		if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
			t.Errorf("%s timestamps not initialized", task.ID)
		}
		if !task.CreatedAt.Equal(task.UpdatedAt) {
			t.Errorf("%s CreatedAt/UpdatedAt differ at creation", task.ID)
		}
		if task.Attempts == nil {
			t.Errorf("%s attempts should be initialized, not nil", task.ID)
		}
	}
}

func TestBuildSerializesAcceptanceCriteria(t *testing.T) {
	tasks, err := Build(validPlan())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	want := "application starts\nno runtime errors"
	if tasks[0].AcceptanceCriteria != want {
		t.Errorf("criteria = %q, want %q", tasks[0].AcceptanceCriteria, want)
	}
}

func TestSerializeAcceptanceCriteria(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"one"}, "one"},
		{[]string{"one", "two", "three"}, "one\ntwo\nthree"},
	}
	for _, tc := range cases {
		if got := SerializeAcceptanceCriteria(tc.in); got != tc.want {
			t.Errorf("SerializeAcceptanceCriteria(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildRejectsInvalidPlan(t *testing.T) {
	cases := map[string]*planner.Plan{
		"no stages":     {Project: "p", Summary: "s"},
		"duplicate ids": {Project: "p", Summary: "s", Stages: []planner.Stage{{ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}}, {ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}}}},
		"unknown dep":   {Project: "p", Summary: "s", Stages: []planner.Stage{{ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}, Dependencies: []string{"S9"}}}},
	}
	for name, plan := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(plan); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestBuildRejectsNilPlan(t *testing.T) {
	if _, err := Build(nil); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-plan error, got %v", err)
	}
}

func envPlan(envDeps ...string) *planner.Plan {
	return &planner.Plan{
		Project: "Book RAG",
		Summary: "Build it.",
		Stages: []planner.Stage{
			{
				ID:                 "S000",
				Title:              "Development Environment",
				Objective:          "Set up the dev environment.",
				Dependencies:       envDeps,
				AcceptanceCriteria: []string{"environment builds"},
				Kind:               planner.KindEnvironment,
			},
			{
				ID:                 "S001",
				Title:              "Feature",
				Objective:          "Build the feature.",
				AcceptanceCriteria: []string{"feature works"},
				Kind:               planner.KindFeature,
			},
		},
	}
}

func TestCreateTasksFromPlanAddsEnvironmentDependency(t *testing.T) {
	var saved []*domain.Task
	tasks, err := CreateTasksFromPlan(envPlan(), func(ts []*domain.Task) error {
		saved = ts
		return nil
	})
	if err != nil {
		t.Fatalf("CreateTasksFromPlan failed: %v", err)
	}

	byID := map[string]*domain.Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	if !reflect.DeepEqual(byID["S001"].DependencyIDs, []string{"S000"}) {
		t.Errorf("feature dependencies = %v, want [S000]", byID["S001"].DependencyIDs)
	}
	if len(byID["S000"].DependencyIDs) != 0 {
		t.Errorf("environment dependencies = %v, want none", byID["S000"].DependencyIDs)
	}
	if len(saved) != len(tasks) {
		t.Errorf("saved %d tasks, want %d", len(saved), len(tasks))
	}
}

func TestCreateTasksFromPlanRejectsBootstrapCycle(t *testing.T) {
	// The environment stage depends on the feature, so adding the implicit
	// feature→environment edge creates a cycle that DAG validation must reject.
	if _, err := CreateTasksFromPlan(envPlan("S001"), func([]*domain.Task) error { return nil }); err == nil {
		t.Error("expected a cycle error")
	}
}
