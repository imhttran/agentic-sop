package scheduler

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// This file locks in the TASK008 determinism and dependency guardrails: the
// scheduler regression suite must not depend on an LLM, network access,
// destructive Git operations, or deletion of persisted state. It inspects the
// package's own test sources rather than running anything external, so the check
// itself is deterministic and hermetic.

// TestGuardrailNoForbiddenTestDependencies asserts no scheduler test source
// (other than this guardrail file) imports or invokes LLM, network, or
// destructive Git APIs.
func TestGuardrailNoForbiddenTestDependencies(t *testing.T) {
	forbiddenImports := []string{
		"net/http",
		"net",
		"os/exec",
	}
	// Substrings that would indicate a forbidden external dependency in test
	// source.
	forbiddenCode := []string{
		"ollama",
		"openai",
		"anthropic",
		"http.Get(",
		"http.Post(",
		"exec.Command(",
		"git reset --hard",
		"git clean",
		"os.Remove(",
		"os.RemoveAll(",
	}

	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob test files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no test files found to check")
	}

	fset := token.NewFileSet()
	for _, name := range files {
		if name == "guardrails_regression_test.go" {
			// This file legitimately contains the forbidden substrings as data.
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(src)

		parsed, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			for _, bad := range forbiddenImports {
				if path == bad {
					t.Errorf("%s imports forbidden package %q", name, path)
				}
			}
		}

		for _, bad := range forbiddenCode {
			if strings.Contains(body, bad) {
				t.Errorf("%s references forbidden construct %q", name, bad)
			}
		}
	}
}

// TestGuardrailRepeatedRunsAreDeterministic reruns the scheduler and recovery
// decision sequences to prove they are stable across repeated identical runs.
func TestGuardrailRepeatedRunsAreDeterministic(t *testing.T) {
	build := func() *fakeStore {
		return &fakeStore{tasks: []*domain.Task{
			blockedTask("T001"),
			blockedTask("T002"),
			task("T003", domain.PLANNED),
		}}
	}

	var baseline []Outcome
	for run := 0; run < 10; run++ {
		store := build()
		s := New(store)
		var got []Outcome
		for i := 0; i < 6; i++ {
			res, err := s.Next(nil)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			got = append(got, res.Outcome)
			if res.Task == nil {
				break
			}
			switch res.Outcome {
			case RecoveredTask:
				find(t, store.tasks, res.Task.ID).Status = domain.PLANNED
			case ReadyTask:
				find(t, store.tasks, res.Task.ID).Status = domain.LOCAL_DONE
			}
		}
		if baseline == nil {
			baseline = got
			continue
		}
		if len(got) != len(baseline) {
			t.Fatalf("run %d decisions = %v, want %v", run, got, baseline)
		}
		for i := range got {
			if got[i] != baseline[i] {
				t.Fatalf("run %d decisions = %v, want %v", run, got, baseline)
			}
		}
	}
}
