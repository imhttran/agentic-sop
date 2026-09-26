package taskfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestRenderOmitsMissingSections(t *testing.T) {
	got := (&Spec{Title: "Only title"}).Render()
	if strings.Contains(got, "Requirements") || strings.Contains(got, "Constraints") {
		t.Errorf("Render should omit empty sections:\n%s", got)
	}
	if !strings.Contains(got, "# Only title") {
		t.Errorf("Render missing heading:\n%s", got)
	}
}
