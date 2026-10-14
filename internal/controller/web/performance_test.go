package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// perfFixture is a realistic agentic-sop perf.Task record, shaped from a real SOP
// run (agentic-sop .agent-sdlc/runs/TASK001/metrics.json).
const perfFixture = `{
  "id": "t1",
  "total_ms": 133591,
  "stages_ms": {"fix": 15994, "implement": 94246, "plan": 11861, "review": 7085, "validation": 4303},
  "validation_ms": {"build": 945, "lint": 337, "test": 3020},
  "counts": {"agent_calls": 3, "agent_calls_avoided": 2, "validation_runs": 2, "validation_reused": 1, "review_runs": 1, "review_reused": 0, "fix_cycles": 1, "plan_repairs": 0}
}`

// planPerfFixture is a realistic agentic-sop perf.Run aggregate.
const planPerfFixture = `{
  "started_at": "2026-09-30T12:39:46.373208Z",
  "total_ms": 70000,
  "tasks": [
    {"id": "T1", "total_ms": 40000, "stages_ms": {"plan": 10000, "implement": 20000, "validation": 5000, "review": 5000}, "validation_ms": {"test": 5000}, "counts": {"agent_calls": 2, "review_runs": 1}},
    {"id": "T2", "total_ms": 30000, "stages_ms": {"plan": 5000, "implement": 15000, "validation": 8000, "review": 2000}, "validation_ms": {"test": 8000}, "counts": {"agent_calls": 1, "validation_runs": 1, "review_runs": 1}}
  ],
  "counts": {"agent_calls": 3, "agent_calls_avoided": 1, "validation_runs": 1, "validation_reused": 1, "review_runs": 2, "fix_cycles": 0}
}`

func writePlanMeta(t *testing.T, root, planID string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md","plan_id":"`+planID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The task page renders SOP's performance values when a record exists.
func TestTaskPageRendersPerformance(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	writeRun(t, root, "t1", "metrics.json", perfFixture)

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t1")
	if code != 200 {
		t.Fatalf("task page = %d, want 200", code)
	}
	for _, want := range []string{
		"Performance",
		"2m13.591s", // Total, SOP-measured
		"2m2.101s",  // Agent = plan + implement + fix
		"91% of measured",
		"Validation breakdown",
		"Agent calls",
		"Agent calls avoided",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("task page missing %q", want)
		}
	}
}

// A task with no performance artifact renders an explicit absence, never a row of
// 0s that could read as a measurement.
func TestTaskPageRendersUnavailablePerformance(t *testing.T) {
	srv, id, _ := newTestServerRoot(t, "sop")

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t2")
	if code != 200 {
		t.Fatalf("task page = %d, want 200", code)
	}
	if !strings.Contains(body, "Performance metrics unavailable for this run") {
		t.Errorf("task page should state performance is unavailable")
	}
}

// The project page renders SOP's plan-level aggregate, keyed by the recorded plan.
func TestProjectPageRendersPlanPerformance(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	writePlanMeta(t, root, "plan-x")
	writeRun(t, root, "plan-x", "metrics.json", planPerfFixture)

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("project page = %d, want 200", code)
	}
	for _, want := range []string{"Plan performance", "plan-x", "1m10s", "Tasks measured"} {
		if !strings.Contains(body, want) {
			t.Errorf("project page missing %q", want)
		}
	}
}

// With no recorded plan the project page states plan performance is unavailable.
func TestProjectPageRendersUnavailablePlanPerformance(t *testing.T) {
	srv, id, _ := newTestServerRoot(t, "sop")

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("project page = %d, want 200", code)
	}
	if !strings.Contains(body, "Plan performance unavailable") {
		t.Errorf("project page should state plan performance is unavailable")
	}
}
