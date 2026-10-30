package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// These tests pin FIX-158-001: one required criterion per deliverable line, with
// path-aware recognition of repository-relative paths, root-level files, code
// symbols, commands, and URLs.
//
// Pre-fix failure recorded before the change: for the exact example line below,
// deliverablePaths returned [internal/provider/provider.go ModelInfo.Locality]
// (the dotted symbol passed the old "has / or ." rule) and deliverableCriteria
// emitted two criteria, so a line naming one file produced a duplicate
// ModelInfo.Locality criterion that failed even when the file existed.

const exampleDeliverable = "- `internal/provider/provider.go` — document that a non-empty `ModelInfo.Locality` is authoritative runtime evidence and that an empty value means undetermined."

func TestDeliverablePathsExactExample(t *testing.T) {
	got := deliverablePaths(exampleDeliverable)
	if len(got) != 1 || got[0] != "internal/provider/provider.go" {
		t.Fatalf("deliverablePaths = %v, want exactly [internal/provider/provider.go]", got)
	}
	for _, p := range got {
		if strings.Contains(p, "ModelInfo") {
			t.Fatalf("descriptive symbol returned as a path: %q", p)
		}
	}
}

func TestDeliverablePathsRootLevelFiles(t *testing.T) {
	for in, want := range map[string][]string{
		"- `Makefile` — build entry point":             {"Makefile"},
		"`LICENSE`":                                    {"LICENSE"},
		"the module root, `go.mod`, stays":             {"go.mod"},
		"`Makefile`, `LICENSE`, `go.mod` — root files": {"Makefile", "LICENSE", "go.mod"},
	} {
		got := deliverablePaths(in)
		if len(got) != len(want) {
			t.Errorf("deliverablePaths(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("deliverablePaths(%q) = %v, want %v", in, got, want)
			}
		}
	}
	// Recognition is syntactic: the shape is accepted without the file existing.
	if !looksLikeDeliverablePath("Makefile") || !looksLikeDeliverablePath("go.mod") || !looksLikeDeliverablePath("LICENSE") {
		t.Fatal("root-level file recognition must not depend on the file existing")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "Makefile")); err == nil {
		t.Fatal("test fixture unexpected: Makefile exists")
	}
}

func TestDeliverablePathsRejectsSymbolsCommandsURLs(t *testing.T) {
	for _, in := range []string{
		"`ModelInfo.Locality`",
		"`ModelInfo`",
		"`errors.New`",
		"`fmt.Errorf`",
		"runs in the standard `go test ./...` suite",
		"`https://example.com/x`",
	} {
		if got := deliverablePaths(in); len(got) != 0 {
			t.Errorf("deliverablePaths(%q) = %v, want no paths", in, got)
		}
	}
}

func TestDeliverablePathsExistingForms(t *testing.T) {
	bullet := "`internal/ollamaagent/already_satisfied.go`, `internal/ollamaagent/orchestrate.go` — index/normalized criterion matching; accept independently verified existing work"
	if got := deliverablePaths(bullet); len(got) != 2 || got[0] != "internal/ollamaagent/already_satisfied.go" || got[1] != "internal/ollamaagent/orchestrate.go" {
		t.Fatalf("HARD-03 multi-path bullet = %v, want both paths", got)
	}
	if got := deliverablePaths("`docs/reference/` — reference docs"); len(got) != 1 || got[0] != "docs/reference/" {
		t.Fatalf("directory path = %v, want [docs/reference/]", got)
	}
	if got := deliverablePaths("`internal/planner/markdown.go:164-166`"); len(got) != 1 || got[0] != "internal/planner/markdown.go:164-166" {
		t.Fatalf("line-range form = %v, want one path", got)
	}
}

func TestDeliverableCriteriaOnePerLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal/provider"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal/provider/provider.go"), []byte("package provider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := &taskfile.Spec{Deliverables: []string{exampleDeliverable}}
	criteria := deliverableCriteria(spec, dir)
	if len(criteria) != 1 {
		t.Fatalf("criteria = %d, want exactly 1: %+v", len(criteria), criteria)
	}
	if criteria[0].Description != exampleDeliverable {
		t.Fatalf("description = %q, want the original line", criteria[0].Description)
	}
	if err := criteria[0].Check(); err != nil {
		t.Fatalf("present deliverable failed: %v", err)
	}

	// Absent: the check fails and names the path, not the symbol.
	empty := t.TempDir()
	absent := deliverableCriteria(spec, empty)
	if len(absent) != 1 {
		t.Fatalf("absent criteria = %d, want 1", len(absent))
	}
	err := absent[0].Check()
	if err == nil {
		t.Fatal("absent deliverable must fail")
	}
	if !strings.Contains(err.Error(), "internal/provider/provider.go") {
		t.Fatalf("error does not name the path: %v", err)
	}
	if strings.Contains(err.Error(), "ModelInfo") {
		t.Fatalf("error names the descriptive symbol: %v", err)
	}
}

func TestDeliverableCriteriaMultiPathOneCriterion(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"a/x.go", "a/y.go"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spec := &taskfile.Spec{Deliverables: []string{"`a/x.go`, `a/y.go` — both required"}}
	criteria := deliverableCriteria(spec, dir)
	if len(criteria) != 1 {
		t.Fatalf("multi-path line produced %d criteria, want 1", len(criteria))
	}
	if err := criteria[0].Check(); err != nil {
		t.Fatalf("all paths present must pass: %v", err)
	}

	// Remove one: the combined error names the missing path and not the present one.
	if err := os.Remove(filepath.Join(dir, "a/y.go")); err != nil {
		t.Fatal(err)
	}
	err := deliverableCriteria(spec, dir)[0].Check()
	if err == nil {
		t.Fatal("missing path must fail the criterion")
	}
	if !strings.Contains(err.Error(), "a/y.go") {
		t.Fatalf("error does not name the missing path: %v", err)
	}
	if strings.Contains(err.Error(), "a/x.go") {
		t.Fatalf("error names the present path: %v", err)
	}
	if got := missingDeliverables(spec, dir); len(got) != 1 || got[0] != "a/y.go" {
		t.Fatalf("missingDeliverables = %v, want [a/y.go]", got)
	}
}

func TestDeliverableCriteriaMissingRootLevelFile(t *testing.T) {
	dir := t.TempDir()
	spec := &taskfile.Spec{Deliverables: []string{"`Makefile` — build entry point"}}
	criteria := deliverableCriteria(spec, dir)
	if len(criteria) != 1 {
		t.Fatalf("criteria = %d, want 1", len(criteria))
	}
	err := criteria[0].Check()
	if err == nil || !strings.Contains(err.Error(), "Makefile") {
		t.Fatalf("missing root-level file must fail and name it: %v", err)
	}
	if got := missingDeliverables(spec, dir); len(got) != 1 || got[0] != "Makefile" {
		t.Fatalf("missingDeliverables = %v, want [Makefile]", got)
	}
}

func TestDeliverableGroupsClassifiesExample(t *testing.T) {
	spec := &taskfile.Spec{Deliverables: []string{exampleDeliverable, "Report the observed locality in your summary"}}
	groups := deliverableGroups(spec)
	var paths, outcomes []string
	for _, g := range groups {
		switch g.Heading {
		case "Declared repository paths":
			paths = append(paths, g.Items...)
		case "Expected outcomes (descriptive)":
			outcomes = append(outcomes, g.Items...)
		}
	}
	if len(paths) != 1 || paths[0] != exampleDeliverable {
		t.Fatalf("Declared repository paths = %v, want the example line", paths)
	}
	if len(outcomes) != 1 {
		t.Fatalf("Expected outcomes = %v, want the prose-only line", outcomes)
	}
}
