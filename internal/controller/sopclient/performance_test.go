package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// taskMetricsFixture is a realistic agentic-sop perf.Task record
// (.agent-sdlc/runs/<task>/metrics.json). Its shape is copied from a real SOP run
// (agentic-sop's own .agent-sdlc/runs/TASK001/metrics.json) rather than invented,
// so the reader is tested against SOP's actual persisted schema.
const taskMetricsFixture = `{
  "id": "t1",
  "total_ms": 133591,
  "stages_ms": {"fix": 15994, "implement": 94246, "plan": 11861, "review": 7085, "validation": 4303},
  "validation_ms": {"build": 945, "lint": 337, "test": 3020},
  "counts": {"agent_calls": 3, "agent_calls_avoided": 0, "validation_runs": 2, "validation_reused": 0, "review_runs": 1, "review_reused": 0, "fix_cycles": 1, "plan_repairs": 0}
}`

// runMetricsFixture is a realistic agentic-sop perf.Run record
// (.agent-sdlc/runs/<plan-id>/metrics.json, the plan-level aggregate): a started
// time, a task array, and SOP's own already-aggregated counts.
const runMetricsFixture = `{
  "started_at": "2026-09-30T12:39:46.373208Z",
  "total_ms": 70000,
  "tasks": [
    {"id": "T1", "total_ms": 40000, "stages_ms": {"plan": 10000, "implement": 20000, "validation": 5000, "review": 5000, "fix": 0}, "validation_ms": {"build": 1000, "test": 4000}, "counts": {"agent_calls": 2, "validation_runs": 1, "review_runs": 1}},
    {"id": "T2", "total_ms": 30000, "stages_ms": {"plan": 5000, "implement": 15000, "validation": 8000, "review": 2000}, "validation_ms": {"test": 8000}, "counts": {"agent_calls": 1, "validation_runs": 1, "review_runs": 1}}
  ],
  "counts": {"agent_calls": 3, "agent_calls_avoided": 1, "validation_runs": 2, "validation_reused": 1, "review_runs": 2, "review_reused": 0, "fix_cycles": 0, "plan_repairs": 0}
}`

func openPerfStore(t *testing.T, root string) *Store {
	t.Helper()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func writePlanMeta(t *testing.T, root, planID string) {
	t.Helper()
	path := filepath.Join(root, ".agent-sdlc", "plan.meta.json")
	if err := os.WriteFile(path, []byte(`{"source":"docs/PLAN.md","plan_id":"`+planID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Durations and counts map straight from SOP's persisted perf.Task record.
func TestPerformanceProjectsTaskMetrics(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "metrics.json", taskMetricsFixture)
	st := openPerfStore(t, root)

	got := st.Performance("t1")
	if !got.Present {
		t.Fatal("Present = false, want true (metrics.json exists and carries measurements)")
	}
	ms := func(n int64) time.Duration { return time.Duration(n) * time.Millisecond }
	if got.Total != ms(133591) {
		t.Errorf("Total = %v, want 133.591s", got.Total)
	}
	if got.Plan != ms(11861) || got.Implement != ms(94246) || got.Validation != ms(4303) || got.Review != ms(7085) || got.Fix != ms(15994) {
		t.Errorf("stage durations wrong: %+v", got)
	}
	if got.Build != ms(945) || got.Test != ms(3020) || got.Lint != ms(337) {
		t.Errorf("validation breakdown wrong: build=%v test=%v lint=%v", got.Build, got.Test, got.Lint)
	}
	if want := ms(11861 + 94246 + 15994); got.Agent() != want {
		t.Errorf("Agent() = %v, want %v (plan+implement+fix)", got.Agent(), want)
	}
	if got.AgentCalls != 3 || got.ValidationRuns != 2 || got.ReviewRuns != 1 || got.FixCycles != 1 {
		t.Errorf("counts wrong: %+v", got)
	}
	if got.AgentCallsAvoided != 0 || got.ValidationReused != 0 || got.ReviewReused != 0 || got.PlanRepairs != 0 {
		t.Errorf("zero counts wrong: %+v", got)
	}
}

// A missing metrics artifact is an explicit absence, never zeros.
func TestPerformanceMissingIsAbsent(t *testing.T) {
	root := newProject(t)
	st := openPerfStore(t, root)

	got := st.Performance("nope")
	if got.Present {
		t.Fatalf("Present = true, want false for a task with no performance artifact: %+v", got)
	}
	if got.Total != 0 || got.AgentCalls != 0 {
		t.Errorf("absent record must be the zero value, got %+v", got)
	}
	if got.Shares() != nil {
		t.Errorf("Shares() = %v, want nil for an absent record", got.Shares())
	}
}

// A malformed metrics artifact must not crash the reader; it degrades to absent.
func TestPerformanceMalformedIsAbsent(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "metrics.json", "{ this is not json")
	st := openPerfStore(t, root)

	got := st.Performance("t1")
	if got.Present {
		t.Fatalf("Present = true, want false for a malformed metrics artifact: %+v", got)
	}
}

// A present-but-empty metrics document (all zeros) is treated as absent so a
// renderer never shows a fabricated 0s measurement.
func TestPerformanceEmptyDocumentIsAbsent(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "metrics.json", `{"id":"t1","total_ms":0,"counts":{}}`)
	st := openPerfStore(t, root)

	if got := st.Performance("t1"); got.Present {
		t.Fatalf("Present = true, want false for an all-zero document: %+v", got)
	}
}

// When metrics.json is absent, report.json's redundant performance field is the
// fallback (matching SOP's own read order).
func TestPerformanceFallsBackToReportPerformance(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "report.json", `{"id":"t1","stage":"PASSED","performance":`+taskMetricsFixture+`}`)
	st := openPerfStore(t, root)

	got := st.Performance("t1")
	if !got.Present || got.Total != time.Duration(133591)*time.Millisecond {
		t.Fatalf("report fallback not read: Present=%v Total=%v", got.Present, got.Total)
	}
}

// An older report.json with no performance field yields an explicit absence.
func TestPerformanceReportWithoutPerformanceIsAbsent(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "report.json", `{"id":"t1","stage":"PASSED","decision":"PASS"}`)
	st := openPerfStore(t, root)

	if got := st.Performance("t1"); got.Present {
		t.Fatalf("Present = true, want false for a report predating performance: %+v", got)
	}
}

// The task-detail projection carries SOP's performance record.
func TestTaskDetailExposesPerformance(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','Task One','obj','ac','LOCAL_DONE',NULL,1,3,'`+now+`','`+now+`')`)
	writeArtifact(t, root, "t1", "metrics.json", taskMetricsFixture)
	st := openPerfStore(t, root)

	d, err := st.Task(context.Background(), "t1", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Performance.Present || d.Performance.AgentCalls != 3 {
		t.Fatalf("TaskDetail.Performance = %+v, want the SOP record", d.Performance)
	}
}

// The plan-level aggregate is keyed by SOP's recorded plan id, never guessed.
func TestPlanPerformanceReadsRunAggregate(t *testing.T) {
	root := newProject(t)
	writePlanMeta(t, root, "plan-x")
	writeArtifact(t, root, "plan-x", "metrics.json", runMetricsFixture)
	st := openPerfStore(t, root)

	got := st.PlanPerformance()
	if !got.Present || got.PlanID != "plan-x" {
		t.Fatalf("PlanPerformance = %+v, want Present with plan-x", got)
	}
	if got.Tasks != 2 {
		t.Errorf("Tasks = %d, want 2", got.Tasks)
	}
	if got.Total != time.Duration(70000)*time.Millisecond {
		t.Errorf("Total = %v, want 70s", got.Total)
	}
	// Category split uses SOP's own rule: agent = plan+implement+fix over tasks.
	if want := time.Duration(10000+20000+5000+15000) * time.Millisecond; got.Agent != want {
		t.Errorf("Agent = %v, want %v", got.Agent, want)
	}
	if want := time.Duration(5000+8000) * time.Millisecond; got.Validation != want {
		t.Errorf("Validation = %v, want %v", got.Validation, want)
	}
	if want := time.Duration(5000+2000) * time.Millisecond; got.Review != want {
		t.Errorf("Review = %v, want %v", got.Review, want)
	}
	// Per-run counts come straight from SOP's aggregate, not recomputed.
	if got.AgentCalls != 3 || got.AgentCallsAvoided != 1 || got.ValidationRuns != 2 || got.ValidationReused != 1 || got.ReviewRuns != 2 {
		t.Errorf("aggregate counts wrong: %+v", got)
	}
}

// No recorded plan means no plan performance (never a guess).
func TestPlanPerformanceAbsentWithoutPlan(t *testing.T) {
	root := newProject(t)
	st := openPerfStore(t, root)
	if got := st.PlanPerformance(); got.Present {
		t.Fatalf("Present = true, want false with no plan.meta.json: %+v", got)
	}
}

// A recorded plan with no persisted aggregate is an explicit absence.
func TestPlanPerformanceAbsentForUnknownPlan(t *testing.T) {
	root := newProject(t)
	writePlanMeta(t, root, "plan-x")
	st := openPerfStore(t, root)
	if got := st.PlanPerformance(); got.Present {
		t.Fatalf("Present = true, want false when SOP persisted no aggregate: %+v", got)
	}
}

// Shares are arithmetic over SOP durations with whole-percent rounding; they are
// nil when no stage time was measured, so a renderer shows no percentage.
func TestPerformanceShares(t *testing.T) {
	p := Performance{
		Present:    true,
		Plan:       60 * time.Second,
		Validation: 30 * time.Second,
		Review:     10 * time.Second,
	}
	shares := p.Shares()
	if len(shares) != 3 {
		t.Fatalf("Shares() len = %d, want 3", len(shares))
	}
	want := []struct {
		name string
		pct  int
	}{{"Agent", 60}, {"Validation", 30}, {"Review", 10}}
	for i, w := range want {
		if shares[i].Name != w.name || shares[i].Percent != w.pct {
			t.Errorf("share %d = %s/%d%%, want %s/%d%%", i, shares[i].Name, shares[i].Percent, w.name, w.pct)
		}
	}

	// Measured stage time of zero (counts only) yields no shares at all.
	onlyCounts := Performance{Present: true, AgentCalls: 3}
	if onlyCounts.Shares() != nil {
		t.Errorf("Shares() = %v, want nil when no stage time was measured", onlyCounts.Shares())
	}
}

// Reading performance never mutates SOP state: the run artifacts and state.db are
// byte-identical before and after.
func TestPerformanceReadDoesNotMutateSOPState(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "metrics.json", taskMetricsFixture)
	st := openPerfStore(t, root)

	before := snapshotTree(t, filepath.Join(root, ".agent-sdlc"))
	_ = st.Performance("t1")
	_ = st.Performance("t1")
	_ = st.PlanPerformance()
	after := snapshotTree(t, filepath.Join(root, ".agent-sdlc"))

	if len(before) != len(after) {
		t.Fatalf("tree changed: %d entries before, %d after", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("%s changed after a performance read", path)
		}
	}
}
