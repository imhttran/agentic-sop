//go:build acceptance

package planflow

// Operator-authored acceptance test for ACC-007: reconciliation must detect a
// change to a stage's operator-authorized acceptance checks. A changed, added, or
// removed check on an executed task is an executable-semantics (material) change
// that needs explicit human approval; on a never-executed task it is an update.
// It drives the same planflow entry points `sop reconcile` uses: Inspect for
// --list-changed, Reconcile (with the CLI's autonomy callback) for apply and
// --accept-changed.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// accOp007Store is an in-memory TaskStore and GraphStore.
type accOp007Store struct{ tasks []*domain.Task }

func (s *accOp007Store) List() ([]*domain.Task, error) { return s.tasks, nil }
func (s *accOp007Store) SaveTasks(tasks []*domain.Task) error {
	s.tasks = append(s.tasks, tasks...)
	return nil
}
func (s *accOp007Store) ClearTasks() error { s.tasks = nil; return nil }
func (s *accOp007Store) ReplaceGraph(tasks []*domain.Task, remove []string) error {
	var kept []*domain.Task
	for _, t := range s.tasks {
		if !slices.Contains(remove, t.ID) {
			kept = append(kept, t)
		}
	}
	for _, t := range tasks {
		i := slices.IndexFunc(kept, func(k *domain.Task) bool { return k.ID == t.ID })
		if i >= 0 {
			kept[i] = t
		} else {
			kept = append(kept, t)
		}
	}
	s.tasks = kept
	return nil
}

func (s *accOp007Store) task(t *testing.T, id string) *domain.Task {
	t.Helper()
	for _, task := range s.tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not in graph", id)
	return nil
}

// accOp007Doc renders a two-stage "## Tasks"-shape plan. summary lets the control
// scenario vary whitespace outside any stage.
func accOp007Doc(summary string, s1Checks, s2Checks []string) string {
	var b strings.Builder
	b.WriteString("# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\n" + summary + "\n\n## Tasks\n\n")
	b.WriteString("### S001 — Application skeleton\n\n- **Objective:** Create the application skeleton.\n- **Acceptance:** the application starts.\n")
	for _, c := range s1Checks {
		b.WriteString("- **Acceptance check:** `" + c + "`\n")
	}
	b.WriteString("\n### S002 — Ingestion\n\n- **Objective:** Ingest records.\n- **Depends on:** S001\n- **Acceptance:** records are ingested.\n")
	for _, c := range s2Checks {
		b.WriteString("- **Acceptance check:** `" + c + "`\n")
	}
	return b.String()
}

const (
	accOp007Summary = "Add a widget."
	accOp007Check1  = "go test ./internal/widget/..."
	accOp007Check2  = "go vet ./internal/widget/..."
	accOp007CheckS2 = "go test ./internal/ingest/..."
)

func accOp007WritePlan(t *testing.T, dir, doc string) string {
	t.Helper()
	path := filepath.Join(dir, "PLAN.md")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// accOp007PersistedChecks reads a stage's checks from the persisted plan.json.
func accOp007PersistedChecks(t *testing.T, dir, id string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, planFileName))
	if err != nil {
		t.Fatalf("read plan.json: %v", err)
	}
	var plan struct {
		Stages []struct {
			ID     string   `json:"id"`
			Checks []string `json:"acceptance_checks"`
		} `json:"stages"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("decode plan.json: %v", err)
	}
	for _, s := range plan.Stages {
		if s.ID == id {
			return s.Checks
		}
	}
	t.Fatalf("stage %s not in plan.json", id)
	return nil
}

// accOp007Setup prepares the base plan the way `sop run` does (Prepare persists
// plan.json and creates the task graph), then gives S001 execution history.
func accOp007Setup(t *testing.T) (string, *accOp007Store) {
	t.Helper()
	dir := t.TempDir()
	path := accOp007WritePlan(t, dir, accOp007Doc(accOp007Summary, []string{accOp007Check1}, []string{accOp007CheckS2}))
	st := &accOp007Store{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: path, Store: st}); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// Precondition: the operator's checks reached plan.json, so a later assertion on
	// them is meaningful.
	accOp007Eq(t, "precondition plan.json S001 checks", accOp007PersistedChecks(t, dir, "S001"), []string{accOp007Check1})
	accOp007Eq(t, "precondition plan.json S002 checks", accOp007PersistedChecks(t, dir, "S002"), []string{accOp007CheckS2})

	s1 := st.task(t, "S001")
	s1.Status = domain.LOCAL_DONE
	s1.Attempt = 1
	s1.Attempts = []domain.Attempt{{Number: 1, Status: domain.LOCAL_DONE, Reason: "done"}}
	if st.task(t, "S002").Executed() {
		t.Fatal("precondition: S002 must be never-executed")
	}
	return dir, st
}

// accOp007AutoAccept mirrors the CLI's reconcile callback under the most permissive
// policy (safe changes auto-reconcile). A check change must still stop for a human,
// so it can never be classified as a descriptive-only (equivalent) change.
func accOp007AutoAccept(change ExecutedChange) autonomy.Decision {
	kind := autonomy.PlanChangeExecutedMaterial
	if change.Equivalent {
		kind = autonomy.PlanChangeExecutedEquivalent
	}
	return autonomy.DecidePlanChange(kind, autonomy.Policy{Level: autonomy.High, AutoReconcileSafeChanges: true})
}

// accOp007List is `sop reconcile PLAN.md --list-changed` (no autonomy callback).
func accOp007List(t *testing.T, dir string, st *accOp007Store) ReconcileResult {
	t.Helper()
	res, err := Inspect(context.Background(), ReconcileOptions{Dir: dir, PlanSource: filepath.Join(dir, "PLAN.md"), Store: st})
	if err != nil {
		t.Fatalf("list-changed: %v", err)
	}
	return res
}

// accOp007Apply is `sop reconcile PLAN.md [--accept-changed ID]...`.
func accOp007Apply(dir string, st *accOp007Store, accept ...string) (ReconcileResult, error) {
	return Reconcile(context.Background(), ReconcileOptions{
		Dir:                dir,
		PlanSource:         filepath.Join(dir, "PLAN.md"),
		Store:              st,
		AcceptChanged:      accept,
		AutoAcceptExecuted: accOp007AutoAccept,
	})
}

func accOp007Eq(t *testing.T, label string, got, want []string) {
	t.Helper()
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func TestOperatorAcceptanceACC007ExecutedCheckChangeNeedsApproval(t *testing.T) {
	cases := []struct {
		name   string
		checks []string
	}{
		{"modify", []string{"go test -run TestNothing ./internal/widget/..."}},
		{"add", []string{accOp007Check1, accOp007Check2}},
		{"remove", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, st := accOp007Setup(t)
			accOp007WritePlan(t, dir, accOp007Doc(accOp007Summary, tc.checks, []string{accOp007CheckS2}))

			list := accOp007List(t, dir, st)
			if !list.PlanChanged {
				t.Error("list: PlanChanged = false, want true")
			}
			accOp007Eq(t, "list unchanged", list.Unchanged, []string{"S002"})
			accOp007Eq(t, "list changed_executed", list.ChangedExecuted, []string{"S001"})
			accOp007Eq(t, "list changed_executed_material", list.ChangedExecutedMaterial, []string{"S001"})
			accOp007Eq(t, "list changed_executed_equivalent", list.ChangedExecutedEquivalent, nil)
			accOp007Eq(t, "list updated", list.Updated, nil)
			accOp007Eq(t, "list added", list.Added, nil)
			accOp007Eq(t, "list removed", list.Removed, nil)
			accOp007Eq(t, "list removed_executed", list.RemovedExecuted, nil)
			if len(list.AutoReconciled) != 0 {
				t.Errorf("list auto_reconciled = %+v, want none", list.AutoReconciled)
			}

			// Apply without approval: stops for a human and mutates nothing.
			res, err := accOp007Apply(dir, st)
			if err == nil || !strings.Contains(err.Error(), "NEEDS_HUMAN") || !strings.Contains(err.Error(), "S001") {
				t.Errorf("apply without --accept-changed: err = %v (res %+v), want NEEDS_HUMAN naming S001", err, res)
			}
			if err != nil {
				accOp007Eq(t, "plan.json S001 checks after refused apply", accOp007PersistedChecks(t, dir, "S001"), []string{accOp007Check1})
			}

			// Apply with explicit approval: accepted, history preserved, new checks persisted.
			res, err = accOp007Apply(dir, st, "S001")
			if err != nil {
				t.Fatalf("apply --accept-changed S001: %v", err)
			}
			accOp007Eq(t, "apply accepted", res.Accepted, []string{"S001"})
			accOp007Eq(t, "apply unchanged", res.Unchanged, []string{"S002"})
			accOp007Eq(t, "apply updated", res.Updated, nil)
			if len(res.AutoReconciled) != 0 {
				t.Errorf("apply auto_reconciled = %+v, want none", res.AutoReconciled)
			}
			accOp007Eq(t, "plan.json S001 checks", accOp007PersistedChecks(t, dir, "S001"), tc.checks)
			accOp007Eq(t, "plan.json S002 checks", accOp007PersistedChecks(t, dir, "S002"), []string{accOp007CheckS2})
			if s1 := st.task(t, "S001"); s1.Status != domain.LOCAL_DONE || len(s1.Attempts) != 1 {
				t.Errorf("S001 status=%s attempts=%d, want LOCAL_DONE with history preserved", s1.Status, len(s1.Attempts))
			}
		})
	}
}

func TestOperatorAcceptanceACC007UnexecutedCheckChangeIsUpdate(t *testing.T) {
	dir, st := accOp007Setup(t)
	newCheck := "go test -race ./internal/ingest/..."
	accOp007WritePlan(t, dir, accOp007Doc(accOp007Summary, []string{accOp007Check1}, []string{newCheck}))

	list := accOp007List(t, dir, st)
	accOp007Eq(t, "list unchanged", list.Unchanged, []string{"S001"})
	accOp007Eq(t, "list updated", list.Updated, []string{"S002"})
	accOp007Eq(t, "list changed_executed", list.ChangedExecuted, nil)

	res, err := accOp007Apply(dir, st)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	accOp007Eq(t, "apply updated", res.Updated, []string{"S002"})
	accOp007Eq(t, "apply unchanged", res.Unchanged, []string{"S001"})
	accOp007Eq(t, "apply accepted", res.Accepted, nil)
	accOp007Eq(t, "plan.json S002 checks", accOp007PersistedChecks(t, dir, "S002"), []string{newCheck})
	accOp007Eq(t, "plan.json S001 checks", accOp007PersistedChecks(t, dir, "S001"), []string{accOp007Check1})
}

func TestOperatorAcceptanceACC007WhitespaceOnlyChangeIsUnchanged(t *testing.T) {
	dir, st := accOp007Setup(t)
	// Same checks; only whitespace in the Summary prose changes (the source hash
	// changes, so the full compile-and-diff path runs rather than the no-op shortcut).
	accOp007WritePlan(t, dir, accOp007Doc("Add   a widget.   ", []string{accOp007Check1}, []string{accOp007CheckS2}))

	list := accOp007List(t, dir, st)
	if list.PlanChanged {
		t.Error("list: PlanChanged = true, want false for a whitespace-only change")
	}
	accOp007Eq(t, "list unchanged", list.Unchanged, []string{"S001", "S002"})
	accOp007Eq(t, "list updated", list.Updated, nil)
	accOp007Eq(t, "list changed_executed", list.ChangedExecuted, nil)

	res, err := accOp007Apply(dir, st)
	if err != nil {
		t.Fatalf("apply without approval: %v", err)
	}
	accOp007Eq(t, "apply unchanged", res.Unchanged, []string{"S001", "S002"})
	accOp007Eq(t, "apply updated", res.Updated, nil)
	accOp007Eq(t, "apply accepted", res.Accepted, nil)
	accOp007Eq(t, "plan.json S001 checks", accOp007PersistedChecks(t, dir, "S001"), []string{accOp007Check1})

	// An approval for an unchanged executed task is rejected: nothing to accept.
	if _, err := accOp007Apply(dir, st, "S001"); err == nil {
		t.Error("apply --accept-changed S001 on an unchanged task succeeded, want rejection")
	}
}

// TestOperatorAcceptanceACC007TrailingWhitespaceOnCheckLineIsUnchanged is a
// control: trailing whitespace on an acceptance-check line is trimmed by the plan
// parser, so it is not a check change. Internal whitespace inside a command is
// compared exactly and deliberately counts as a change (the conservative choice:
// an extra approval, never a silent weakening).
func TestOperatorAcceptanceACC007TrailingWhitespaceOnCheckLineIsUnchanged(t *testing.T) {
	dir, st := accOp007Setup(t)
	doc := accOp007Doc("Add a widget.", []string{accOp007Check1}, []string{accOp007CheckS2})
	doc = strings.Replace(doc, "`"+accOp007Check1+"`\n", "`"+accOp007Check1+"`   \t\n", 1)
	accOp007WritePlan(t, dir, doc)

	list := accOp007List(t, dir, st)
	accOp007Eq(t, "list changed_executed", list.ChangedExecuted, nil)
	accOp007Eq(t, "list updated", list.Updated, nil)
	accOp007Eq(t, "plan.json S001 checks", accOp007PersistedChecks(t, dir, "S001"), []string{accOp007Check1})
}
