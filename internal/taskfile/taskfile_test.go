package taskfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

const fullDoc = `# T023 --- End-to-End Dogfood

## Status

DONE

## Objective

Exercise the whole pipeline on a small project.

## Dependencies

- Stages 4-21 (the components being composed)

## Scope

- testdata/dogfood/PRD.md

## Rules

- No network or real model in automated tests.
- Only external boundaries are faked.

## Acceptance Criteria

- [x] A PRD produces a plan of at least five tasks.
- [ ] The completion loop drives every task to DONE.
`

func TestParseFullTaskFile(t *testing.T) {
	spec, err := Parse([]byte(fullDoc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ID != "T023" {
		t.Errorf("ID = %q, want T023", spec.ID)
	}
	if spec.Title != "End-to-End Dogfood" {
		t.Errorf("Title = %q, want End-to-End Dogfood", spec.Title)
	}
	if !strings.Contains(spec.Description, "Exercise the whole pipeline") {
		t.Errorf("Description = %q", spec.Description)
	}
	if len(spec.Dependencies) != 1 || !strings.Contains(spec.Dependencies[0], "Stages 4-21") {
		t.Errorf("Dependencies = %v", spec.Dependencies)
	}
	if len(spec.Constraints) != 2 || !strings.Contains(spec.Constraints[0], "No network") {
		t.Errorf("Constraints = %v", spec.Constraints)
	}
	if len(spec.AcceptanceCriteria) != 2 {
		t.Fatalf("AcceptanceCriteria = %v", spec.AcceptanceCriteria)
	}
	if spec.AcceptanceCriteria[0] != "A PRD produces a plan of at least five tasks." {
		t.Errorf("criteria[0] = %q, checkbox not stripped", spec.AcceptanceCriteria[0])
	}
	if spec.AcceptanceCriteria[1] != "The completion loop drives every task to DONE." {
		t.Errorf("criteria[1] = %q", spec.AcceptanceCriteria[1])
	}
	if strings.Contains(spec.Description, "DONE") {
		t.Errorf("Description should not absorb the Status section: %q", spec.Description)
	}
}

func TestParsePlainMarkdown(t *testing.T) {
	spec, err := Parse([]byte("# My Task\n\nDo the thing carefully.\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ID != "" {
		t.Errorf("ID = %q, want empty", spec.ID)
	}
	if spec.Title != "My Task" {
		t.Errorf("Title = %q, want My Task", spec.Title)
	}
	if spec.Description != "Do the thing carefully." {
		t.Errorf("Description = %q", spec.Description)
	}
}

func TestParseTitleFallbackWithoutHeading(t *testing.T) {
	spec, err := Parse([]byte("Just some text\nmore text\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.Title != "Just some text" {
		t.Errorf("Title = %q, want the first line", spec.Title)
	}
}

func TestParseNamedSectionsOverrideHeading(t *testing.T) {
	doc := "# ignored heading\n\n## ID\n\nT999\n\n## Title\n\nReal Title\n\n## Summary\n\nA summary.\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ID != "T999" {
		t.Errorf("ID = %q, want T999", spec.ID)
	}
	if spec.Title != "Real Title" {
		t.Errorf("Title = %q, want Real Title", spec.Title)
	}
	if spec.Description != "A summary." {
		t.Errorf("Description = %q, want A summary.", spec.Description)
	}
}

func TestParseBulletVariants(t *testing.T) {
	doc := "## Requirements\n\n* one\n+ two\n1. three\n2) four\nplain five\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := []string{"one", "two", "three", "four", "plain five"}
	if len(spec.Requirements) != len(want) {
		t.Fatalf("Requirements = %v, want %v", spec.Requirements, want)
	}
	for i, w := range want {
		if spec.Requirements[i] != w {
			t.Errorf("Requirements[%d] = %q, want %q", i, spec.Requirements[i], w)
		}
	}
}

func TestParseEmptyFile(t *testing.T) {
	for _, in := range []string{"", "   \n\t\n"} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%q) = nil error, want empty-file error", in)
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TASK.md")
	if err := os.WriteFile(path, []byte(fullDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if spec.Title != "End-to-End Dogfood" {
		t.Errorf("Title = %q", spec.Title)
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Error("expected error loading a missing file")
	}
}

func TestRenderRoundTrips(t *testing.T) {
	original, err := Parse([]byte(fullDoc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	reparsed, err := Parse([]byte(original.Render()))
	if err != nil {
		t.Fatalf("Parse(Render) failed: %v", err)
	}
	if reparsed.ID != original.ID || reparsed.Title != original.Title {
		t.Errorf("heading changed: (%q,%q) -> (%q,%q)", original.ID, original.Title, reparsed.ID, reparsed.Title)
	}
	if strings.Join(reparsed.AcceptanceCriteria, "|") != strings.Join(original.AcceptanceCriteria, "|") {
		t.Errorf("criteria changed: %v -> %v", original.AcceptanceCriteria, reparsed.AcceptanceCriteria)
	}
	if strings.Join(reparsed.Constraints, "|") != strings.Join(original.Constraints, "|") {
		t.Errorf("constraints changed: %v -> %v", original.Constraints, reparsed.Constraints)
	}
}

func TestParseExecutionMode(t *testing.T) {
	doc := "# T001 — Verify\n\n## Objective\n\nCheck the boundary.\n\n## Execution\n\n- verify-first\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ExecutionMode != domain.ExecutionVerifyFirst {
		t.Errorf("ExecutionMode = %q, want %q", spec.ExecutionMode, domain.ExecutionVerifyFirst)
	}
}

func TestParseIgnoresUnknownExecutionMode(t *testing.T) {
	// An unrelated "Execution" section must not break the task or be mistaken for
	// a mode; the task stays implement-mode.
	doc := "# T001 — Do work\n\n## Execution\n\nRun the build with `make`.\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ExecutionMode != "" {
		t.Errorf("ExecutionMode = %q, want empty (unrecognized value ignored)", spec.ExecutionMode)
	}
}

func TestParseDiscoveryIterations(t *testing.T) {
	doc := "# T001 — Explore\n\n## Objective\n\nWiden discovery.\n\n## Discovery iterations\n\n24 — a multi-package task needs more exploration.\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.DiscoveryIterations != 24 {
		t.Errorf("DiscoveryIterations = %d, want 24", spec.DiscoveryIterations)
	}
	// The value round-trips through Render and Parse.
	reparsed, err := Parse([]byte(spec.Render()))
	if err != nil {
		t.Fatalf("Parse(Render) failed: %v", err)
	}
	if reparsed.DiscoveryIterations != 24 {
		t.Errorf("reparsed DiscoveryIterations = %d, want 24", reparsed.DiscoveryIterations)
	}
}

func TestParseIgnoresInvalidDiscoveryIterations(t *testing.T) {
	// A non-numeric or non-positive allowance must be ignored, so it can never unbound
	// a run; the task keeps the default resolution (0).
	for _, value := range []string{"many", "0", "-4"} {
		doc := "# T001 — Do work\n\n## Discovery iterations\n\n" + value + "\n"
		spec, err := Parse([]byte(doc))
		if err != nil {
			t.Fatalf("Parse(%q) failed: %v", value, err)
		}
		if spec.DiscoveryIterations != 0 {
			t.Errorf("Parse(%q) DiscoveryIterations = %d, want 0", value, spec.DiscoveryIterations)
		}
	}
}

func TestRenderExecutionModeRoundTrips(t *testing.T) {
	spec := &Spec{Title: "Verify boundary", ExecutionMode: domain.ExecutionVerifyFirst}
	rendered := spec.Render()
	if !strings.Contains(rendered, "## Execution") {
		t.Fatalf("Render missing the execution section:\n%s", rendered)
	}
	got, err := Parse([]byte(rendered))
	if err != nil {
		t.Fatalf("Parse(Render) failed: %v", err)
	}
	if got.ExecutionMode != domain.ExecutionVerifyFirst {
		t.Errorf("ExecutionMode = %q after round-trip", got.ExecutionMode)
	}
}

func TestRenderOmitsMissingSections(t *testing.T) {
	got := (&Spec{Title: "Only title"}).Render()
	if strings.Contains(got, "Requirements") || strings.Contains(got, "Constraints") || strings.Contains(got, "Execution") {
		t.Errorf("Render should omit empty sections:\n%s", got)
	}
	if !strings.Contains(got, "# Only title") {
		t.Errorf("Render missing heading:\n%s", got)
	}
}

func TestParseHierarchicalIDHeading(t *testing.T) {
	spec, err := Parse([]byte("# PREJEV012-S6 — Repository Reconciliation\n\nbody\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if spec.ID != "PREJEV012-S6" {
		t.Errorf("ID = %q, want PREJEV012-S6", spec.ID)
	}
	if spec.Title != "Repository Reconciliation" {
		t.Errorf("Title = %q, want Repository Reconciliation", spec.Title)
	}
}

// TestParseJoinsWrappedListItems proves a bullet wrapped onto an indented second
// line is ONE item rather than one per physical line, for every list section.
func TestParseJoinsWrappedListItems(t *testing.T) {
	doc := "# T001 — Wrapped\n\n" +
		"## Acceptance criteria\n\n" +
		"- the first criterion wraps onto\n  an indented second line\n" +
		"- the second criterion also wraps\n  onto its own continuation line\n\n" +
		"## Deliverables\n\n" +
		"- the binary at cmd/sop, and also\n  the docs it emits\n" +
		"- a second deliverable that\n  wraps too\n" +
		"## Requirements\n\n" +
		"- must build, and must\n  pass vet\n" +
		"## Constraints\n\n" +
		"- no network, and no\n  real model calls\n" +
		"## Dependencies\n\n" +
		"- T000, and the\n  shared library T000b\n"

	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if got, want := spec.AcceptanceCriteria, []string{
		"the first criterion wraps onto an indented second line",
		"the second criterion also wraps onto its own continuation line",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("AcceptanceCriteria = %v, want %v", got, want)
	}
	if got, want := spec.Deliverables, []string{
		"the binary at cmd/sop, and also the docs it emits",
		"a second deliverable that wraps too",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("Deliverables = %v, want %v", got, want)
	}
	if got, want := spec.Requirements, []string{"must build, and must pass vet"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Requirements = %v, want %v", got, want)
	}
	if got, want := spec.Constraints, []string{"no network, and no real model calls"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Constraints = %v, want %v", got, want)
	}
	if got, want := spec.Dependencies, []string{"T000, and the shared library T000b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Dependencies = %v, want %v", got, want)
	}
}

// TestParsePreservedListBehaviour proves the join rule does not change the cases
// existing plans rely on: unindented unbulleted lines, nested bullets, and blank
// lines each keep their own item.
func TestParsePreservedListBehaviour(t *testing.T) {
	doc := "# T001 — Preserved\n\n" +
		"## Acceptance criteria\n\n" +
		"- a bullet with an indented continuation\n  joined on\n" +
		"- a nested bullet stays its own item:\n  - the nested one\n" +
		"plain unindented line stays its own item\n" +
		"\n" +
		"- after a blank line this starts a new item\n"

	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := []string{
		"a bullet with an indented continuation joined on",
		"a nested bullet stays its own item:",
		"the nested one",
		"plain unindented line stays its own item",
		"after a blank line this starts a new item",
	}
	if !reflect.DeepEqual(spec.AcceptanceCriteria, want) {
		t.Errorf("AcceptanceCriteria = %v, want %v", spec.AcceptanceCriteria, want)
	}
}

// TestParseSeparatorEndsItem proves a "---" separator still ends an item.
func TestParseSeparatorEndsItem(t *testing.T) {
	doc := "# T001 — Separator\n\n## Acceptance criteria\n\n- one\n\n---\n\n- two\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got, want := spec.AcceptanceCriteria, []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AcceptanceCriteria = %v, want %v", got, want)
	}
}

// TestParseAcceptanceChecks proves a task file authored with an '## Acceptance
// checks' section and shell-command bullets yields the same Spec.AcceptanceChecks
// in order, distinct from the '## Acceptance criteria' section.
func TestParseAcceptanceChecks(t *testing.T) {
	doc := "# T001 — Checked\n\n" +
		"## Acceptance criteria\n\n" +
		"- the build is green\n" +
		"- the docs exist\n\n" +
		"## Acceptance checks\n\n" +
		"- go test ./...\n" +
		"- go vet ./...\n"

	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got, want := spec.AcceptanceChecks, []string{"go test ./...", "go vet ./..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("AcceptanceChecks = %v, want %v", got, want)
	}
	if got, want := spec.AcceptanceCriteria, []string{"the build is green", "the docs exist"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AcceptanceCriteria = %v, want %v", got, want)
	}
}

// TestRenderParseAcceptanceChecksRoundTrip proves a Spec with AcceptanceChecks
// renders a section that Parse reads back into the same slice, in order and with
// the command text preserved, and that checks and criteria stay distinct sections.
func TestRenderParseAcceptanceChecksRoundTrip(t *testing.T) {
	original := &Spec{
		ID:                 "T001",
		Title:              "Checked",
		AcceptanceCriteria: []string{"the build is green"},
		AcceptanceChecks:   []string{"go test ./...", "go vet ./..."},
	}
	rendered := original.Render()
	if !strings.Contains(rendered, "## Acceptance checks") {
		t.Fatalf("Render missing the acceptance-checks section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## Acceptance criteria") {
		t.Fatalf("Render missing the acceptance-criteria section:\n%s", rendered)
	}

	got, err := Parse([]byte(rendered))
	if err != nil {
		t.Fatalf("Parse(Render) failed: %v", err)
	}
	if !reflect.DeepEqual(got.AcceptanceChecks, original.AcceptanceChecks) {
		t.Errorf("AcceptanceChecks = %v, want %v", got.AcceptanceChecks, original.AcceptanceChecks)
	}
	if !reflect.DeepEqual(got.AcceptanceCriteria, original.AcceptanceCriteria) {
		t.Errorf("AcceptanceCriteria = %v, want %v", got.AcceptanceCriteria, original.AcceptanceCriteria)
	}
}

// TestAcceptanceChecksAndCriteriaStayDistinct proves the two sections do not merge
// on either side of the round-trip.
func TestAcceptanceChecksAndCriteriaStayDistinct(t *testing.T) {
	doc := "# T001 — Distinct\n\n" +
		"## Acceptance criteria\n\n" +
		"- free-text criterion\n\n" +
		"## Acceptance checks\n\n" +
		"- go test ./...\n"
	spec, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !reflect.DeepEqual(spec.AcceptanceCriteria, []string{"free-text criterion"}) {
		t.Errorf("AcceptanceCriteria = %v", spec.AcceptanceCriteria)
	}
	if !reflect.DeepEqual(spec.AcceptanceChecks, []string{"go test ./..."}) {
		t.Errorf("AcceptanceChecks = %v", spec.AcceptanceChecks)
	}
	reparsed, err := Parse([]byte(spec.Render()))
	if err != nil {
		t.Fatalf("Parse(Render) failed: %v", err)
	}
	if !reflect.DeepEqual(reparsed.AcceptanceCriteria, spec.AcceptanceCriteria) {
		t.Errorf("criteria changed across round-trip: %v -> %v", spec.AcceptanceCriteria, reparsed.AcceptanceCriteria)
	}
	if !reflect.DeepEqual(reparsed.AcceptanceChecks, spec.AcceptanceChecks) {
		t.Errorf("checks changed across round-trip: %v -> %v", spec.AcceptanceChecks, reparsed.AcceptanceChecks)
	}
}
