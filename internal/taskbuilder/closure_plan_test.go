package taskbuilder

import (
	"os"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/planner"
)

// closurePlanPath is the named closure plan this regression protects. It lives
// outside the package, so the test reads it through the repository-relative
// path the other repo-plan tests use.
const closurePlanPath = "../../docs/history/plans/PLAN-Pre-Performance-Closure.md"

// closureStage is one CLOSE stage's expected shape under the task projection.
type closureStage struct {
	id    string
	paths []string
}

// closureStages is the contract for the closure plan: every stage keeps its
// permitted evidence artifact path(s) in the generated task's Objective or
// AcceptanceCriteria, because taskbuilder.Build drops Deliverables. CLOSE-001
// additionally keeps its sole-mutation contract and all eight validation
// commands. A stage that moves or renames its report updates this list
// deliberately.
var closureStages = []closureStage{
	{"CLOSE-001", []string{"docs/reports/pre-performance-closure/CLOSE-001-baseline.md"}},
	{"CLOSE-002", []string{"docs/history/pre-performance-closure/CLOSE-002-status-reconciliation.md"}},
	{"CLOSE-003", []string{"docs/reports/pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md"}},
	{"CLOSE-004", []string{"docs/reports/pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md"}},
	{"CLOSE-005", []string{"docs/history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md"}},
	{"CLOSE-006", []string{"docs/history/pre-performance-closure/CLOSE-006-named-plan-dogfood.md"}},
	{"CLOSE-007", []string{"docs/history/pre-performance-closure/CLOSE-007-resume-idempotency.md"}},
	{"CLOSE-008", []string{"docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md"}},
	{"CLOSE-009", []string{
		"docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json",
		"docs/reports/pre-performance-closure/CLOSE-009-performance-baseline.md",
		"docs/reports/pre-performance-closure/workloads/{A,B,C,D}/",
	}},
	{"CLOSE-010", []string{"docs/reports/PERFORMANCE-BASELINE.md"}},
	{"CLOSE-011", []string{"docs/reports/pre-performance-closure/CLOSE-011-readiness.md"}},
}

// compileClosurePlan reads and compiles the repository closure plan through the
// same deterministic path `sop run <plan>.md` uses.
func compileClosurePlan(t *testing.T) *planner.Plan {
	t.Helper()
	data, err := os.ReadFile(closurePlanPath)
	if err != nil {
		t.Fatalf("read %s: %v", closurePlanPath, err)
	}
	plan, err := planner.PlanFromMarkdown(string(data))
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("compiled plan invalid: %v", err)
	}
	return plan
}

// TestClosurePlanTasksKeepArtifactPaths proves the closure plan's evidence
// artifact paths survive into the generated task text. Deliverables are dropped
// by Build, so a path named only there disappears from the task an agent sees;
// restating it in the Objective or AcceptanceCriteria keeps it visible.
func TestClosurePlanTasksKeepArtifactPaths(t *testing.T) {
	plan := compileClosurePlan(t)
	tasks, err := Build(plan)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(tasks) != len(closureStages) {
		t.Fatalf("tasks = %d, want %d", len(tasks), len(closureStages))
	}

	byID := make(map[string]int, len(tasks))
	for i, task := range tasks {
		byID[task.ID] = i
	}

	for _, want := range closureStages {
		i, ok := byID[want.id]
		if !ok {
			t.Errorf("%s: no generated task with that ID", want.id)
			continue
		}
		task := tasks[i]
		text := task.Objective + "\n" + task.AcceptanceCriteria
		for _, path := range want.paths {
			if !strings.Contains(text, path) {
				t.Errorf("%s: task text does not name evidence artifact %q; Deliverables are dropped by Build:\n%s", want.id, path, text)
			}
		}
	}
}

// TestClosurePlanIDsAndDAGUnchanged pins the eleven stage IDs and their
// dependency edges, so a documentation edit cannot silently renumber a stage or
// reorder the DAG.
func TestClosurePlanIDsAndDAGUnchanged(t *testing.T) {
	plan := compileClosurePlan(t)
	tasks, err := Build(plan)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	gotIDs := make([]string, len(tasks))
	for i, task := range tasks {
		gotIDs[i] = task.ID
	}
	wantIDs := []string{"CLOSE-001", "CLOSE-002", "CLOSE-003", "CLOSE-004", "CLOSE-005", "CLOSE-006", "CLOSE-007", "CLOSE-008", "CLOSE-009", "CLOSE-010", "CLOSE-011"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Errorf("task[%d] id = %q, want %q", i, gotIDs[i], wantIDs[i])
		}
	}

	wantDeps := map[string][]string{
		"CLOSE-001": nil,
		"CLOSE-002": {"CLOSE-001"},
		"CLOSE-003": {"CLOSE-001", "CLOSE-002"},
		"CLOSE-004": {"CLOSE-001", "CLOSE-002"},
		"CLOSE-005": {"CLOSE-002", "CLOSE-003", "CLOSE-004"},
		"CLOSE-006": {"CLOSE-005"},
		"CLOSE-007": {"CLOSE-006"},
		"CLOSE-008": {"CLOSE-003", "CLOSE-004"},
		"CLOSE-009": {"CLOSE-003", "CLOSE-004", "CLOSE-005", "CLOSE-006", "CLOSE-007", "CLOSE-008"},
		"CLOSE-010": {"CLOSE-009"},
		"CLOSE-011": {"CLOSE-002", "CLOSE-003", "CLOSE-004", "CLOSE-005", "CLOSE-006", "CLOSE-007", "CLOSE-008", "CLOSE-009", "CLOSE-010"},
	}
	for _, task := range tasks {
		want := wantDeps[task.ID]
		if len(task.DependencyIDs) != len(want) {
			t.Errorf("%s deps = %v, want %v", task.ID, task.DependencyIDs, want)
			continue
		}
		for i := range want {
			if task.DependencyIDs[i] != want[i] {
				t.Errorf("%s deps = %v, want %v", task.ID, task.DependencyIDs, want)
				break
			}
		}
	}
}

// TestClosure001TaskKeepsSoleReportContract proves CLOSE-001's task text keeps
// its sole-mutation contract, all eight deterministic commands, and the model
// evidence distinctions that keep configuration, selection and availability
// separate.
func TestClosure001TaskKeepsSoleReportContract(t *testing.T) {
	plan := compileClosurePlan(t)
	tasks, err := Build(plan)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var close001 string
	for _, task := range tasks {
		if task.ID == "CLOSE-001" {
			close001 = task.Objective + "\n" + task.AcceptanceCriteria
		}
	}
	if close001 == "" {
		t.Fatal("no CLOSE-001 task generated")
	}

	const report = "docs/reports/pre-performance-closure/CLOSE-001-baseline.md"
	if !strings.Contains(close001, report) {
		t.Errorf("CLOSE-001 task text does not name its sole report %q", report)
	}
	if !strings.Contains(close001, "sole") {
		t.Error("CLOSE-001 task text does not state the sole-mutation contract")
	}

	for _, cmd := range []string{
		"test -s " + report,
		`grep -q "agentic-sop" ` + report,
		`grep -q "sop-controller" ` + report,
		`grep -q "Go version" ` + report,
		`grep -q "SOP binary" ` + report,
		`grep -q "SMALL" ` + report,
		`grep -q "MEDIUM" ` + report,
		`grep -q "LARGE" ` + report,
	} {
		if !strings.Contains(close001, cmd) {
			t.Errorf("CLOSE-001 task text is missing deterministic command %q", cmd)
		}
	}

	for _, want := range []string{"SMALL", "MEDIUM", "LARGE", "UNAVAILABLE", "routing"} {
		if !strings.Contains(close001, want) {
			t.Errorf("CLOSE-001 task text is missing model-evidence distinction %q", want)
		}
	}
}
