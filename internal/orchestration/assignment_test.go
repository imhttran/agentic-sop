package orchestration

// ORCH-002.3 boundary validation tests. Every rejection path is asserted to fail
// with an explicit typed error and is never surfaced as success.

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

func TestValidateAssignmentRejectsMalformed(t *testing.T) {
	base := conformingAssignment()

	tests := []struct {
		name   string
		mutate func(a *WorkAssignment)
		code   string
	}{
		{name: "empty assignment id", mutate: func(a *WorkAssignment) { a.AssignmentID = "" }, code: "missing_assignment_id"},
		{name: "empty task id", mutate: func(a *WorkAssignment) { a.TaskID = "" }, code: "missing_task_id"},
		{name: "empty task", mutate: func(a *WorkAssignment) { a.Task = "" }, code: "missing_task"},
		{name: "unknown capability", mutate: func(a *WorkAssignment) { a.Capability = "BOGUS" }, code: "invalid_capability"},
		{name: "empty repository identity", mutate: func(a *WorkAssignment) { a.RepositoryIdentity = RepositoryIdentity{} }, code: "missing_repository_identity"},
		{name: "absolute scope path", mutate: func(a *WorkAssignment) { a.Scope = Scope{Paths: []string{"/etc/passwd"}} }, code: "malformed_scope"},
		{name: "escaping scope path", mutate: func(a *WorkAssignment) { a.Scope = Scope{Paths: []string{"../outside"}} }, code: "malformed_scope"},
		{name: "negative budget", mutate: func(a *WorkAssignment) { a.Budget = Budget{MaxSteps: -1} }, code: "invalid_budget"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			tc.mutate(&a)
			err := ValidateAssignment(a)
			if err == nil {
				t.Fatalf("ValidateAssignment accepted a malformed assignment")
			}
			var assignmentErr *AssignmentError
			if !errors.As(err, &assignmentErr) {
				t.Fatalf("error %v is not *AssignmentError", err)
			}
			if !hasDiagnostic(assignmentErr.Diagnostics, tc.code) {
				t.Fatalf("diagnostics %v missing code %q", assignmentErr.Diagnostics, tc.code)
			}
		})
	}
}

func TestValidateResultRejectsMalformedAndOutOfScope(t *testing.T) {
	assignment := conformingAssignment()
	validResult := WorkResult{
		AssignmentID:       assignment.AssignmentID,
		Status:             agent.OutcomeCompleted,
		ChangedFiles:       []string{"internal/orchestration/assignment.go"},
		RepositoryIdentity: assignment.RepositoryIdentity,
	}
	if err := ValidateResult(assignment, validResult); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(r *WorkResult)
		code   string
	}{
		{name: "missing assignment id", mutate: func(r *WorkResult) { r.AssignmentID = "" }, code: "missing_assignment_id"},
		{name: "assignment id mismatch", mutate: func(r *WorkResult) { r.AssignmentID = "other" }, code: "assignment_id_mismatch"},
		{name: "unknown status", mutate: func(r *WorkResult) { r.Status = "SUCCESS" }, code: "unknown_status"},
		{name: "out of scope file", mutate: func(r *WorkResult) { r.ChangedFiles = []string{"internal/agent/agent.go"} }, code: "out_of_scope"},
		{name: "escaping file", mutate: func(r *WorkResult) { r.ChangedFiles = []string{"../secret"} }, code: "out_of_scope"},
		{name: "missing result identity", mutate: func(r *WorkResult) { r.RepositoryIdentity = RepositoryIdentity{} }, code: "missing_repository_identity"},
		{name: "stale identity", mutate: func(r *WorkResult) { r.RepositoryIdentity = RepositoryIdentity{Revision: "stale"} }, code: "repository_identity_mismatch"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validResult
			tc.mutate(&r)
			err := ValidateResult(assignment, r)
			if err == nil {
				t.Fatalf("ValidateResult accepted a malformed/out-of-scope result")
			}
			var resultErr *ResultError
			if !errors.As(err, &resultErr) {
				t.Fatalf("error %v is not *ResultError", err)
			}
			if !hasDiagnostic(resultErr.Diagnostics, tc.code) {
				t.Fatalf("diagnostics %v missing code %q", resultErr.Diagnostics, tc.code)
			}
		})
	}
}

// TestUnparsablePayloadFailsSafely asserts that a response with no structured
// outcome is not reinterpreted as success.
func TestUnparsablePayloadFailsSafely(t *testing.T) {
	assignment := conformingAssignment()
	result := FromAgentResponse(assignment, agent.Response{Content: "free-form prose"})
	if result.Status != agent.OutcomeFailed {
		t.Fatalf("status = %q, want failed for unparsable payload", result.Status)
	}
	if !hasDiagnostic(result.Diagnostics, "unparsable_payload") {
		t.Fatalf("diagnostics %v missing unparsable_payload", result.Diagnostics)
	}
	if err := ValidateResult(assignment, result); err == nil {
		t.Fatalf("failed result validated as success")
	}
}

// TestAdapterTransportErrorIsFailure asserts a transport error surfaces as an
// explicit failure result plus an error, never success.
func TestAdapterTransportErrorIsFailure(t *testing.T) {
	assignment := conformingAssignment()
	worker := NewWorkerAdapter(WorkUnit{TaskID: assignment.TaskID, Capability: assignment.Capability}, fakeAgent{err: errors.New("boom")}, assignment)

	result, err := worker.Assign(context.Background(), assignment)
	if err == nil {
		t.Fatalf("transport error was not surfaced")
	}
	if result.Status != agent.OutcomeFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

// TestContractTypesAreProviderNeutral is the ORCH-002 arch assertion: the
// assignment/result source contains no provider/model/transport/vendor field or
// identifier. It parses the contract files and inspects field names.
func TestContractTypesAreProviderNeutral(t *testing.T) {
	forbidden := []string{"openai", "ollama", "mlx", "anthropic", "claude", "gpt", "model", "vendor", "http", "grpc", "provider"}

	files := []string{"assignment.go", "worker_adapter.go"}
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if !isContractType(spec.Name.Name) {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				for _, ident := range field.Names {
					lower := strings.ToLower(ident.Name)
					for _, bad := range forbidden {
						if strings.Contains(lower, bad) {
							t.Fatalf("%s.%s mentions provider/model-specific term %q", spec.Name.Name, ident.Name, bad)
						}
					}
				}
			}
			return true
		})
	}
}

// TestScopeContainsSemantics locks the scope containment rules.
func TestScopeContainsSemantics(t *testing.T) {
	scope := Scope{Paths: []string{"internal/orchestration"}, Globs: []string{"cmd/**"}}
	inScope := []string{"internal/orchestration/assignment.go", "cmd/sop/main.go"}
	outOfScope := []string{"internal/agent/agent.go", "../escape", "/abs"}
	for _, f := range inScope {
		if !scopeContains(scope, f) {
			t.Fatalf("scopeContains(%q) = false, want true", f)
		}
	}
	for _, f := range outOfScope {
		if scopeContains(scope, f) {
			t.Fatalf("scopeContains(%q) = true, want false", f)
		}
	}
	if scopeContains(Scope{}, "anything") {
		t.Fatalf("empty scope must deny all files")
	}
}

func hasDiagnostic(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func isContractType(name string) bool {
	switch name {
	case "WorkAssignment", "WorkResult", "Scope", "RepositoryIdentity", "Budget", "OutputContract", "Diagnostic", "Progress":
		return true
	default:
		return false
	}
}
