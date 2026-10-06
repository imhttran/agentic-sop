package orchestration

import (
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestCoordinatorContractHasNoLifecycleAuthority asserts that the coordinator
// contract types expose no field or method that advances task state, approves,
// or commits. It extends the lifecycle-guard conventions in
// lifecycle_guard_test.go.
func TestCoordinatorContractHasNoLifecycleAuthority(t *testing.T) {
	for _, forbidden := range []string{"transition", "approve", "commit", "accept", "advance", "completeTask"} {
		for _, typ := range []reflect.Type{
			reflect.TypeOf(AssignmentOutcome{}),
			reflect.TypeOf(ExecutionReport{}),
			reflect.TypeOf(ExecutionPolicy{}),
			reflect.TypeOf(Coordinator{}),
		} {
			assertNoForbiddenName(t, typ, forbidden)
		}
	}
}

// assertNoForbiddenName fails if typ declares a field or method whose name
// contains the forbidden substring (case-insensitive).
func assertNoForbiddenName(t *testing.T, typ reflect.Type, forbidden string) {
	t.Helper()
	lower := strings.ToLower(forbidden)
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if strings.Contains(name, lower) {
			t.Fatalf("%s field %q exposes forbidden lifecycle vocabulary %q", typ.Name(), typ.Field(i).Name, forbidden)
		}
	}
	for i := 0; i < typ.NumMethod(); i++ {
		name := strings.ToLower(typ.Method(i).Name)
		if strings.Contains(name, lower) {
			t.Fatalf("%s method %q exposes forbidden lifecycle vocabulary %q", typ.Name(), typ.Method(i).Name, forbidden)
		}
	}
}

// TestCoordinatorSourceImportsOnlyAgentAndStdlib asserts that coordinator.go
// depends only on internal/agent and the standard library: no provider, model,
// transport, or filesystem dependency.
func TestCoordinatorSourceImportsOnlyAgentAndStdlib(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "coordinator.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse coordinator.go: %v", err)
	}
	allowed := map[string]bool{
		"context":      true,
		"errors":       true,
		"sort":         true,
		"sync":         true,
		"github.com/imhttran/agentic-sop/internal/agent": true,
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, "\"")
		if !allowed[p] {
			t.Fatalf("coordinator.go imports %q; only internal/agent and stdlib are permitted", p)
		}
	}
}

// TestCoordinatorSourceHasNoFilesystemImport asserts coordinator.go never
// imports the filesystem or a network transport directly.
func TestCoordinatorSourceHasNoFilesystemImport(t *testing.T) {
	src, err := os.ReadFile("coordinator.go")
	if err != nil {
		t.Fatalf("read coordinator.go: %v", err)
	}
	for _, bad := range []string{"\"os\"", "\"io/fs\"", "\"net/http\""} {
		if strings.Contains(string(src), bad) {
			t.Fatalf("coordinator.go imports forbidden dependency %s", bad)
		}
	}
}

// TestCoordinatorIdenticalInputsYieldIdenticalReportShapes asserts identical
// inputs yield identical report shapes at the contract level.
func TestCoordinatorIdenticalInputsYieldIdenticalReportShapes(t *testing.T) {
	a := AssignmentOutcome{AssignmentID: "a1", TaskID: "t1", Capability: "REVIEW", Failure: FailureNone}
	b := AssignmentOutcome{AssignmentID: "a1", TaskID: "t1", Capability: "REVIEW", Failure: FailureNone}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("identical outcomes differ: %#v vs %#v", a, b)
	}
	r1 := summarise([]AssignmentOutcome{a})
	r2 := summarise([]AssignmentOutcome{b})
	if r1.Completed != r2.Completed || r1.Errored != r2.Errored {
		t.Fatalf("identical inputs produced different counts: %+v vs %+v", r1, r2)
	}
}
