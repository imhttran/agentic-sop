package planflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// planDocOwnerlessMissing is a structured plan whose only stage requires a MISSING
// capability declared without an owner or resolution: a true prerequisite, so the
// ownership gate must reject it rather than proceed. The capability is written in
// the canonical rendered shape (a "### <name> — <status>" heading with body fields)
// that PlanFromMarkdown actually parses.
const planDocOwnerlessMissing = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## Capabilities\n\n### External Tool — MISSING\n\n- Evidence: no external tool found\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Requires\n\n- External Tool\n\n### Acceptance Criteria\n\n- starts\n"

// TestReconcileOwnerlessMissingCapabilityIsRejected proves the reconciliation path
// enforces the same capability-ownership gate the planner applies: a MISSING
// capability declared as a true prerequisite with neither owner nor resolution is a
// NEEDS_HUMAN ambiguity, and no graph, provenance, or evidence is written.
func TestReconcileOwnerlessMissingCapabilityIsRejected(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	beforeTasks := len(st.tasks)
	planPath := filepath.Join(dir, config.DirName, planFileName)
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)

	write(t, dir, "PLAN.md", planDocOwnerlessMissing)

	_, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for an ownerless required capability")
	}
	var gap *planner.CapabilityGapError
	if !errors.As(err, &gap) {
		t.Fatalf("error = %v, want *planner.CapabilityGapError", err)
	}
	if !strings.Contains(err.Error(), "NEEDS_HUMAN") {
		t.Errorf("error = %q, want a NEEDS_HUMAN gate", err)
	}
	// No state mutation: graph, plan.json and plan.meta.json are untouched.
	if len(st.tasks) != beforeTasks {
		t.Errorf("store mutated: %d tasks, want %d", len(st.tasks), beforeTasks)
	}
	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json was mutated on the rejected path")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json was mutated on the rejected path")
	}
}

// TestReconcileCaseVariantMissingPrerequisiteNeedsHuman proves a stage Require that
// matches a declared capability only by case/repeated-whitespace still produces the
// ownership gate when that capability is MISSING with no owner or resolution, so the
// gate cannot be bypassed by a spelling variant.
func TestReconcileCaseVariantMissingPrerequisiteNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	// Declared "External Tool", required as "external   tool" (a case/repeated-whitespace
	// variant).
	doc := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## Capabilities\n\n### External Tool — MISSING\n\n- Evidence: no external tool found\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Requires\n\n- external   tool\n\n### Acceptance Criteria\n\n- starts\n"
	write(t, dir, "PLAN.md", doc)

	_, err := Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for a case-variant MISSING prerequisite")
	}
	var gap *planner.CapabilityGapError
	if !errors.As(err, &gap) {
		t.Fatalf("error = %v, want *planner.CapabilityGapError", err)
	}
}

// TestReconcileCanceledContextPreservesState proves a cancelled reconciliation
// returns the context error and mutates nothing: no ReplaceGraph, no plan.json or
// plan.meta.json write.
func TestReconcileCanceledContextPreservesState(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	planPath := filepath.Join(dir, config.DirName, planFileName)
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)
	beforeTitle := taskByID(t, st.tasks, "S001").Title

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Reconcile(ctx, ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, "PLAN.md"),
		Store:      st,
	})
	if err == nil {
		t.Fatal("expected the cancelled context to surface")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("error = %q, want the context cancellation", err)
	}
	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json was mutated by a cancelled reconciliation")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json was mutated by a cancelled reconciliation")
	}
	if got := taskByID(t, st.tasks, "S001").Title; got != beforeTitle {
		t.Errorf("task title = %q, want %q (state preserved)", got, beforeTitle)
	}
}

// TestChangedDoubleQuotedLiteralWhitespaceIsReal proves a whitespace change inside a
// double-quoted literal is a real change.
func TestChangedDoubleQuotedLiteralWhitespaceIsReal(t *testing.T) {
	if sameAcceptanceCriteria([]string{"prints \"a  b\""}, []string{"prints \"a b\""}) {
		t.Error("whitespace inside double quotes must not be folded away")
	}
}

// TestEscapedQuotesInLiteralPreservedExactly proves escaped quotes inside a literal
// are preserved and compared exactly.
func TestEscapedQuotesInLiteralPreservedExactly(t *testing.T) {
	same := "emit \"a \\\"b\\\" c\""
	if !sameAcceptanceCriteria([]string{same}, []string{same}) {
		t.Error("identical escaped-quote literals must compare equal")
	}
	if sameAcceptanceCriteria([]string{"emit \"a \\\"b\\\" c\""}, []string{"emit \"a \\\"b\\\"d\""}) {
		t.Error("a changed escaped-quote literal must be a real change")
	}
}

// TestMultilineQuotedLineOrderIsReal proves reordering lines inside one multiline
// quoted literal is a real change, not a cosmetic set one.
func TestMultilineQuotedLineOrderIsReal(t *testing.T) {
	a := "\"line one\nline two\""
	b := "\"line two\nline one\""
	if sameAcceptanceCriteria([]string{a}, []string{b}) {
		t.Error("reordering lines inside a multiline literal must be a real change")
	}
}

// TestReorderedIndependentCriteriaUnchanged pins that independent single-line
// criteria may reorder without being reported as a change.
func TestReorderedIndependentCriteriaUnchanged(t *testing.T) {
	if !sameAcceptanceCriteria([]string{"starts", "stops cleanly"}, []string{"stops cleanly", "starts"}) {
		t.Error("reordered independent criteria must compare equivalent")
	}
}
