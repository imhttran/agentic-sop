package planflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

const deliverablePlanDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Deliverables\n\n- `cmd/main.go` — the entry point\n\n### Acceptance Criteria\n\n- starts\n"

const deliverablePlanDocChanged = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Deliverables\n\n- `cmd/entry.go` — the entry point\n\n### Acceptance Criteria\n\n- starts\n"

// TestReconcileAcceptsAlreadyAppliedPlanOnlyChange proves an operator can record
// acceptance for a plan-only change that is already in the machine plan, using the
// stale pending request's definition digest as the before value.
func TestReconcileAcceptsAlreadyAppliedPlanOnlyChange(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", deliverablePlanDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	before, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !before.Present {
		t.Fatalf("before identity: %v present=%v", err, before.Present)
	}
	const prior = "prior-definition-digest"

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:                    dir,
		PlanSource:             filepath.Join(dir, "PLAN.md"),
		Store:                  st,
		AcceptChanged:          []string{"S001"},
		PriorDefinitionDigests: map[string]string{"S001": prior},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.PlanChanged {
		t.Error("the requested plan already matches the machine plan")
	}
	if len(res.Accepted) != 1 || res.Accepted[0] != "S001" {
		t.Fatalf("accepted = %v, want [S001]", res.Accepted)
	}

	after, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !after.Present {
		t.Fatalf("after identity: %v present=%v", err, after.Present)
	}
	if after.Digest != before.Digest {
		t.Fatalf("the machine plan changed for an already-applied acceptance: %q -> %q", before.Digest, after.Digest)
	}
	prov, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if !prov.Accepted || prov.AcceptedAt.IsZero() {
		t.Fatalf("provenance = %+v, want a task-scoped acceptance with a time", prov)
	}
	if prov.DefinitionDigestBefore != prior {
		t.Errorf("digest before = %q, want the pending request digest %q", prov.DefinitionDigestBefore, prior)
	}
	if prov.DefinitionDigestAfter != after.Digest {
		t.Errorf("digest after = %q, want the current definition %q", prov.DefinitionDigestAfter, after.Digest)
	}
	task := taskByID(t, st.tasks, "S001")
	if task.Status != domain.LOCAL_DONE || task.Attempt != 1 {
		t.Errorf("task history changed: status=%s attempt=%d", task.Status, task.Attempt)
	}
}

// TestReconcileAcceptsPendingPlanOnlyChange proves a deliverable-only change in the
// requested plan, which the executable task model cannot see, can be accepted with
// --accept-changed and is bound to the recorded -> requested digests.
func TestReconcileAcceptsPendingPlanOnlyChange(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", deliverablePlanDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	before, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !before.Present {
		t.Fatalf("before identity: %v present=%v", err, before.Present)
	}
	write(t, dir, "PLAN.md", deliverablePlanDocChanged)

	res, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:           dir,
		PlanSource:    filepath.Join(dir, "PLAN.md"),
		Store:         st,
		AcceptChanged: []string{"S001"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Accepted) != 1 || res.Accepted[0] != "S001" {
		t.Fatalf("accepted = %v, want [S001]", res.Accepted)
	}

	after, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !after.Present {
		t.Fatalf("after identity: %v present=%v", err, after.Present)
	}
	if after.Digest == before.Digest {
		t.Fatal("the deliverable-only change did not change the definition digest")
	}
	prov, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if !prov.Accepted || prov.AcceptedAt.IsZero() {
		t.Fatalf("provenance = %+v, want a task-scoped acceptance with a time", prov)
	}
	if prov.DefinitionDigestBefore != before.Digest {
		t.Errorf("digest before = %q, want the recorded definition %q", prov.DefinitionDigestBefore, before.Digest)
	}
	if prov.DefinitionDigestAfter != after.Digest {
		t.Errorf("digest after = %q, want the requested definition %q", prov.DefinitionDigestAfter, after.Digest)
	}
	if prov.DefinitionDigestBefore == prov.DefinitionDigestAfter {
		t.Error("the acceptance recorded no definition change")
	}
}

// TestReconcileRefusesAppliedAcceptanceWhoseDigestStillMatches proves a prior digest
// equal to the current definition is refused, so a still-applicable request is never
// recorded as an accepted change.
func TestReconcileRefusesAppliedAcceptanceWhoseDigestStillMatches(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", deliverablePlanDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	current, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !current.Present {
		t.Fatalf("identity: %v present=%v", err, current.Present)
	}
	_, err = Reconcile(context.Background(), ReconcileOptions{
		Dir:                    dir,
		PlanSource:             filepath.Join(dir, "PLAN.md"),
		Store:                  st,
		AcceptChanged:          []string{"S001"},
		PriorDefinitionDigests: map[string]string{"S001": current.Digest},
	})
	if err == nil {
		t.Fatal("Reconcile accepted a prior digest that still matches the current definition")
	}
	if !strings.Contains(err.Error(), "already matches the current definition") {
		t.Fatalf("err = %v, want an explicit still-applicable refusal", err)
	}
	prov, perr := TaskReconciliationProvenance(dir, "S001")
	if perr != nil {
		t.Fatalf("provenance: %v", perr)
	}
	if prov.Accepted {
		t.Fatal("a refused acceptance wrote task-scoped provenance")
	}
}
