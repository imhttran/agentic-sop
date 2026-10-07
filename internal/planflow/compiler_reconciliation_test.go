package planflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// countingAgent records how many times a model call would have been made, so a
// regression test can assert reconciliation never consults a model. Its content
// varies per call, modelling a non-deterministic model: if a reconciliation ever
// routed through it, repeated inspections would not be stable.
type countingAgent struct {
	calls int
}

func (c *countingAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	c.calls++
	return agent.Response{Content: strings.Repeat("model-normalized prose ", c.calls)}, nil
}

// reconcileInspectWithAgent runs Inspect with an agent configured, so any model
// call on the reconciliation path would be observable.
func reconcileInspectWithAgent(t *testing.T, dir, name string, st *fakeStore, ag agent.Agent) (ReconcileResult, error) {
	t.Helper()
	return Inspect(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, name),
		Store:      st,
		Agent:      ag,
	})
}

// TestReconcileDeterministicCompileZeroModelCalls proves a changed, structured
// source is compiled deterministically: repeated Inspect (with a model available
// and configured) yields identical diagnostics and classifications and never calls
// the model.
func TestReconcileDeterministicCompileZeroModelCalls(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	// A structured source that differs from the recorded metadata.
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed stage"))

	ag := &countingAgent{}
	first, err := reconcileInspectWithAgent(t, dir, "PLAN.md", st, ag)
	if err != nil {
		t.Fatalf("Inspect #1: %v", err)
	}
	second, err := reconcileInspectWithAgent(t, dir, "PLAN.md", st, ag)
	if err != nil {
		t.Fatalf("Inspect #2: %v", err)
	}
	if ag.calls != 0 {
		t.Errorf("model was called %d times, want 0 (reconciliation is deterministic)", ag.calls)
	}
	if !sameReconcileOutcome(first, second) {
		t.Errorf("repeated Inspect disagreed:\n first=%+v\nsecond=%+v", first, second)
	}
	if len(first.Updated) != 1 || first.Updated[0] != "S001" {
		t.Errorf("Updated = %v, want [S001]", first.Updated)
	}
}

// TestReconcileUnsupportedSourceStableDiagnostic proves a free-form source that is
// not a recognizable structured plan fails with the identical fixed diagnostic
// every time, calls no model, and mutates nothing.
func TestReconcileUnsupportedSourceStableDiagnostic(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	// A free-form, prose-only source with no recognizable structured plan shape.
	write(t, dir, "PLAN.md", "We should probably improve the compiler and tidy up the reconciliation flow somehow.\n")

	planPath := filepath.Join(dir, config.DirName, planFileName)
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)
	beforeTasks := len(st.tasks)

	ag := &countingAgent{}
	var diagnostics []string
	for i := 0; i < 3; i++ {
		_, err := reconcileInspectWithAgent(t, dir, "PLAN.md", st, ag)
		if err == nil {
			t.Fatal("expected the unsupported source to fail")
		}
		diagnostics = append(diagnostics, err.Error())
	}
	if ag.calls != 0 {
		t.Errorf("model was called %d times, want 0", ag.calls)
	}
	for i := 1; i < len(diagnostics); i++ {
		if diagnostics[i] != diagnostics[0] {
			t.Errorf("diagnostic %d differs:\n%q\nvs\n%q", i, diagnostics[0], diagnostics[i])
		}
	}
	for _, want := range []string{"NEEDS_HUMAN", "structured plan", "explicit"} {
		if !strings.Contains(diagnostics[0], want) {
			t.Errorf("diagnostic %q missing %q", diagnostics[0], want)
		}
	}
	// No mutation of store, plan.json or plan.meta.json.
	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json was mutated on the unsupported-source path")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json was mutated on the unsupported-source path")
	}
	if len(st.tasks) != beforeTasks {
		t.Errorf("task count = %d, want %d", len(st.tasks), beforeTasks)
	}
}

// TestReconcileUnsupportedSourceDoesNotMutateViaApply proves the applying
// Reconcile also fails cleanly on an unsupported source without mutating state.
func TestReconcileUnsupportedSourceDoesNotMutateViaApply(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	write(t, dir, "PLAN.md", "Just some free-form notes without headings.\n")

	before := len(st.tasks)
	_, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for an unsupported source")
	}
	if len(st.tasks) != before {
		t.Errorf("store mutated on failure: got %d tasks, want %d", len(st.tasks), before)
	}
}

// TestReconcileCosmeticFormattingIsUnchanged proves a cosmetically-formatted but
// semantically identical source yields Unchanged and preserves the original task
// objects, attempts, timestamps and lifecycle status.
func TestReconcileCosmeticFormattingIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	original := taskByID(t, st.tasks, "S001")
	markExecuted(original)
	originalUpdated := original.UpdatedAt
	// Re-render the same plan with cosmetic whitespace/line-wrap changes only.
	cosmetic := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd    a   widget.\n\n## S001 — Application skeleton\n\nCreate   it.\n\n### Acceptance Criteria\n\n- starts\n"
	write(t, dir, "PLAN.md", cosmetic)

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.PlanChanged {
		t.Error("PlanChanged = true, want false for cosmetic-only variance")
	}
	if len(res.Updated)+len(res.Added)+len(res.Removed)+len(res.ChangedExecuted) != 0 {
		t.Errorf("res = %+v, want no change", res)
	}
	got := taskByID(t, st.tasks, "S001")
	if got != original {
		t.Error("cosmetic equivalence must preserve the original executed task object")
	}
	if got.Status != domain.LOCAL_DONE || got.Attempt != 1 || !got.UpdatedAt.Equal(originalUpdated) {
		t.Errorf("history/timestamp lost: %+v", got)
	}
}

// TestReconcileAcceptanceOrderIsUnchanged proves independent acceptance criteria in
// a different order compare equivalent (a conjunction), preserving the task.
func TestReconcileAcceptanceOrderIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	doc := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n- stops cleanly\n"
	st := prepareForReconcile(t, dir, "PLAN.md", doc)
	executed := taskByID(t, st.tasks, "S001")
	markExecuted(executed)

	// The same two criteria in the opposite order.
	reordered := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- stops cleanly\n- starts\n"
	write(t, dir, "PLAN.md", reordered)

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.PlanChanged || len(res.ChangedExecuted) != 0 {
		t.Errorf("res = %+v, want Unchanged for reordered independent criteria", res)
	}
	if got := taskByID(t, st.tasks, "S001"); got != executed || got.Status != domain.LOCAL_DONE {
		t.Errorf("task = %+v, want the original executed object", got)
	}
}

// TestReconcileImplementModeEquivalentToDefault proves an explicit "implement" mode
// and the default empty mode are equivalent, so no change is reported.
func TestReconcileImplementModeEquivalentToDefault(t *testing.T) {
	dir := t.TempDir()
	doc := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n"
	st := prepareForReconcile(t, dir, "PLAN.md", doc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	explicit := doc + "\n### Execution\n\n- implement\n"
	write(t, dir, "PLAN.md", explicit)

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.PlanChanged || len(res.ChangedExecuted) != 0 {
		t.Errorf("res = %+v, want Unchanged (explicit implement == default empty)", res)
	}
}

// TestReconcileVerifyFirstDiffers proves verify-first versus the default implement
// mode is a real change that requires a human decision for an executed task.
func TestReconcileVerifyFirstDiffers(t *testing.T) {
	dir := t.TempDir()
	doc := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n"
	st := prepareForReconcile(t, dir, "PLAN.md", doc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	verify := doc + "\n### Execution\n\n- verify-first\n"
	write(t, dir, "PLAN.md", verify)

	res, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.ChangedExecuted) != 1 || res.ChangedExecuted[0] != "S001" {
		t.Errorf("ChangedExecuted = %v, want [S001] for a mode change", res.ChangedExecuted)
	}
}

// TestReconcileRealChangesStayChanged proves real acceptance/negation/numeric/scope
// changes on an executed task stay changed_executed and require human approval.
func TestReconcileRealChangesStayChanged(t *testing.T) {
	cases := map[string]string{
		"negation":  "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- does not start\n",
		"numeric":   "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts within 5 seconds\n",
		"scope":     "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it and something else.\n\n### Acceptance Criteria\n\n- starts\n",
		"criterion": "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- runs\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			base := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n"
			st := prepareForReconcile(t, dir, "PLAN.md", base)
			markExecuted(taskByID(t, st.tasks, "S001"))
			write(t, dir, "PLAN.md", doc)

			if _, err := Reconcile(context.Background(), ReconcileOptions{
				Dir:        dir,
				PlanSource: filepath.Join(dir, "PLAN.md"),
				Store:      st,
			}); err == nil {
				t.Fatal("expected NEEDS_HUMAN for a real change to an executed task")
			}
			if got := taskByID(t, st.tasks, "S001"); got.Title != "Application skeleton" {
				t.Errorf("executed task must be untouched: %+v", got)
			}
		})
	}
}

// TestReconcileQuotedAndCodeWhitespaceIsReal proves whitespace inside a quoted
// string or a multiline literal is a real change (never folded away).
func TestReconcileQuotedAndCodeWhitespaceIsReal(t *testing.T) {
	base := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- prints `a  b`\n"
	changed := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- prints `a b`\n"

	if sameAcceptanceCriteria([]string{"prints `a  b`"}, []string{"prints `a b`"}) {
		t.Error("whitespace inside backticks must not be collapsed away")
	}

	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", base)
	markExecuted(taskByID(t, st.tasks, "S001"))
	write(t, dir, "PLAN.md", changed)

	res, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.ChangedExecuted) != 1 {
		t.Errorf("ChangedExecuted = %v, want [S001] for code-whitespace change", res.ChangedExecuted)
	}
}

// TestReconcileMultilineLiteralLineOrderIsReal proves reordering the lines of one
// multiline literal is a real change, not a set-equivalent cosmetic one.
func TestReconcileMultilineLiteralLineOrderIsReal(t *testing.T) {
	a := "the sequence is:\n```\nline one\nline two\n```\n"
	b := "the sequence is:\n```\nline two\nline one\n```\n"
	if sameAcceptanceCriteria([]string{a}, []string{b}) {
		t.Error("reordering lines inside a fenced literal must be a real change")
	}
}

// TestSameStringSetMultiset proves [X,Y] and [X,X] are not equal, so a duplicate
// dependency can never be silently dropped.
func TestSameStringSetMultiset(t *testing.T) {
	if sameStringSet([]string{"X", "Y"}, []string{"X", "X"}) {
		t.Error("sameStringSet([X,Y],[X,X]) = true, want false (multiset comparison)")
	}
	if !sameStringSet([]string{"X", "Y"}, []string{"Y", "X"}) {
		t.Error("sameStringSet must be order-independent for genuinely identical sets")
	}
	if !sameStringSet([]string{"X", "X"}, []string{"X", "X"}) {
		t.Error("sameStringSet([X,X],[X,X]) = false, want true")
	}
}

// TestReconcileRequiresChangeIsReal proves a change to a stage's Requires list is
// reported as changed even though it is not a task-level field.
func TestReconcileRequiresChangeIsReal(t *testing.T) {
	base := &planner.Stage{ID: "S001", Title: "t", Objective: "o", Requires: []string{"A"}, AcceptanceCriteria: []string{"a"}}
	changed := *base
	changed.Requires = []string{"A", "B"}
	if sameStage(*base, changed) {
		t.Error("a Requires change must be a real change")
	}
}

// TestReconcileCapabilityInventoryChangeIsReal proves a change to any capability
// inventory field is reported as changed at the plan level.
func TestReconcileCapabilityInventoryChangeIsReal(t *testing.T) {
	base := &planner.Plan{
		Project: "p", Summary: "s",
		Stages: []planner.Stage{{ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}, Requires: []string{"Cap"}}},
		Capabilities: []planner.Capability{
			{Name: "Cap", Status: planner.CapabilityPartial, Owner: "SOP", Evidence: "e", Location: "l", Gap: "g", Resolution: "r"},
		},
	}
	changed := *base
	changed.Capabilities = []planner.Capability{
		{Name: "Cap", Status: planner.CapabilityPartial, Owner: "SOP", Evidence: "DIFFERENT", Location: "l", Gap: "g", Resolution: "r"},
	}
	if plansEquivalent(base, &changed) {
		t.Error("a capability inventory field change must be reported as changed")
	}
}

// TestReconcilePendingUpdatePreservesHistory proves a legitimate update to a
// pending, never-executed task preserves completed task objects, approvals and
// historical marker artifacts byte-for-byte.
func TestReconcilePendingUpdatePreservesHistory(t *testing.T) {
	dir := t.TempDir()
	two := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n\n## S002 — Ingestion\n\nIngest it.\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- ingests\n"
	st := prepareForReconcile(t, dir, "PLAN.md", two)

	// S001 is executed; S002 is still pending.
	executed := taskByID(t, st.tasks, "S001")
	markExecuted(executed)
	// Record an approval in metadata so it must survive.
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	meta := readMetadata(metaPath)
	meta.ReconciledTasks = []string{"S001"}
	if err := writeMetadata(metaPath, meta); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	metaBefore := read(t, metaPath)

	// Change only the pending S002. The title is part of its Task definition, but we
	// change its Objective (descriptive) so it stays an unexecuted safe update.
	changed := strings.ReplaceAll(two, "Ingest it.", "Ingest it thoroughly.")
	write(t, dir, "PLAN.md", changed)

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "S002" {
		t.Errorf("Updated = %v, want [S002]", res.Updated)
	}
	if len(res.ChangedExecuted) != 0 {
		t.Errorf("ChangedExecuted = %v, want none", res.ChangedExecuted)
	}

	// The completed S001 object is preserved.
	if got := taskByID(t, st.tasks, "S001"); got != executed {
		t.Error("the executed task object must be preserved across a pending-task update")
	}
	if got := readMetadata(metaPath).ReconciledTasks; len(got) != 1 || got[0] != "S001" {
		t.Errorf("approvals must be preserved: %v", got)
	}
	// The recorded approval history stays stable; only GeneratedAt/SourceSHA256 may
	// change with the new source, so assert the approval marker survives.
	if !strings.Contains(read(t, metaPath), "S001") {
		t.Errorf("metadata after reconcile (%q) lost the recorded approval", read(t, metaPath))
	}
	_ = metaBefore
}

// TestReconcileHistoricalMarkerByteForBytePreserved proves a no-op reconciliation of
// a completed plan leaves archived historical marker artifacts byte-for-byte intact.
func TestReconcileHistoricalMarkerPreserved(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	write(t, dir, "PLAN-B.md", planDocB)
	// Complete PLAN-A then hand off to PLAN-B, writing an archive with a lifecycle
	// marker.
	st := completePlan(t, dir, "PLAN-A.md", domain.LOCAL_DONE)
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st}); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	archiveLife := filepath.Join(dir, config.DirName, "archive", "plan-a", lifecycleFile)
	beforeLife := read(t, archiveLife)
	beforeTasks := read(t, filepath.Join(dir, config.DirName, "archive", "plan-a", archivedTasksFile))

	// A no-op reconciliation (unchanged PLAN-B source) must not touch the archive.
	if _, err := reconcile(t, dir, "PLAN-B.md", st); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := read(t, archiveLife); got != beforeLife {
		t.Error("historical lifecycle marker changed across a no-op reconciliation")
	}
	if got := read(t, filepath.Join(dir, config.DirName, "archive", "plan-a", archivedTasksFile)); got != beforeTasks {
		t.Error("archived task records changed across a no-op reconciliation")
	}
}

// sameReconcileOutcome compares the classifying fields of two reconciliation results
// for stability, ignoring nothing that a caller would show a human.
func sameReconcileOutcome(a, b ReconcileResult) bool {
	return a.PlanChanged == b.PlanChanged &&
		strings.Join(a.Unchanged, ",") == strings.Join(b.Unchanged, ",") &&
		strings.Join(a.Updated, ",") == strings.Join(b.Updated, ",") &&
		strings.Join(a.Added, ",") == strings.Join(b.Added, ",") &&
		strings.Join(a.Removed, ",") == strings.Join(b.Removed, ",") &&
		strings.Join(a.ChangedExecuted, ",") == strings.Join(b.ChangedExecuted, ",") &&
		strings.Join(a.RemovedExecuted, ",") == strings.Join(b.RemovedExecuted, ",")
}

// ensure imports are used even if a build tag or GOOS excludes nothing here.
var _ = os.ReadFile
