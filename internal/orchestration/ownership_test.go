package orchestration

// ORCH-005 tests: the write-ownership model, deterministic scope-overlap
// detection, and coordinator admission enforcement. They use deterministic fake
// adapters, no network, no model calls, and no filesystem mutation.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// countingAgent is a deterministic agent double that records how many times it
// was invoked, so tests can assert that no worker executed on admission
// rejection.
type countingAgent struct {
	calls int
}

func (c *countingAgent) Generate(ctx context.Context, request agent.Request) (agent.Response, error) {
	c.calls++
	return agent.Response{
		Content: "ok",
		Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "completed"},
	}, nil
}

// ownershipAssignment builds a valid mutating assignment with the given id and
// scope.
func ownershipAssignment(id string, scope Scope) WorkAssignment {
	b := AssignmentBuilder{
		AssignmentID:       id,
		TaskID:             "ORCH-005",
		Capability:         agent.Implement,
		Task:               "mutate the repository",
		Scope:              scope,
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
	}
	return b.Build()
}

// analysisAssignment builds a valid non-mutating assignment with the given id and
// scope.
func analysisAssignment(id string, scope Scope) WorkAssignment {
	b := AssignmentBuilder{
		AssignmentID:       id,
		TaskID:             "ORCH-005",
		Capability:         agent.Plan,
		Task:               "analyze the repository",
		Scope:              scope,
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
	}
	return b.Build()
}

// TestOwnershipZeroValueIsSingleWriter asserts the default (zero-value) ownership
// policy resolves to the single-writer/integrator mode.
func TestOwnershipZeroValueIsSingleWriter(t *testing.T) {
	var zero WriteOwnership
	if zero != "" {
		t.Fatalf("zero WriteOwnership = %q, want empty", zero)
	}
	p := ExecutionPolicy{}
	if got := p.ownership(); got != OwnershipSingleWriter {
		t.Fatalf("zero ExecutionPolicy.ownership() = %q, want %q", got, OwnershipSingleWriter)
	}
	if OwnershipSingleWriter != "SINGLE_WRITER" {
		t.Fatalf("single-writer mode = %q, want SINGLE_WRITER", OwnershipSingleWriter)
	}
}

// TestWriteScopesOverlap covers Path/Path, Path/Glob, Glob/Glob, disjoint, and
// deny-all cases.
func TestWriteScopesOverlap(t *testing.T) {
	cases := []struct {
		name string
		a, b Scope
		want bool
	}{
		{
			name: "identical paths overlap",
			a:    Scope{Paths: []string{"internal/orchestration"}},
			b:    Scope{Paths: []string{"internal/orchestration"}},
			want: true,
		},
		{
			name: "ancestor directory and descendant file overlap",
			a:    Scope{Paths: []string{"internal"}},
			b:    Scope{Paths: []string{"internal/orchestration/ownership.go"}},
			want: true,
		},
		{
			name: "descendant and ancestor overlap symmetrically",
			a:    Scope{Paths: []string{"internal/orchestration/ownership.go"}},
			b:    Scope{Paths: []string{"internal"}},
			want: true,
		},
		{
			name: "sibling disjoint directories do not overlap",
			a:    Scope{Paths: []string{"internal/agent"}},
			b:    Scope{Paths: []string{"internal/domain"}},
			want: false,
		},
		{
			name: "shared glob matches overlap",
			a:    Scope{Paths: []string{"internal/orchestration/coordinator.go"}},
			b:    Scope{Globs: []string{"internal/**/*.go"}},
			want: true,
		},
		{
			name: "prefix glob path overlap",
			a:    Scope{Paths: []string{"internal/orchestration/coordinator.go"}},
			b:    Scope{Globs: []string{"internal/**"}},
			want: true,
		},
		{
			name: "nested globs overlap",
			a:    Scope{Globs: []string{"internal/**"}},
			b:    Scope{Globs: []string{"internal/orchestration/**"}},
			want: true,
		},
		{
			name: "disjoint globs do not overlap",
			a:    Scope{Globs: []string{"cmd/**"}},
			b:    Scope{Globs: []string{"internal/**"}},
			want: false,
		},
		{
			name: "empty deny-all never overlaps a path",
			a:    Scope{},
			b:    Scope{Paths: []string{"internal"}},
			want: false,
		},
		{
			name: "empty deny-all never overlaps another empty",
			a:    Scope{},
			b:    Scope{},
			want: false,
		},
		{
			name: "disjoint paths with globs do not overlap",
			a:    Scope{Paths: []string{"internal/agent"}, Globs: []string{"internal/agent/**"}},
			b:    Scope{Paths: []string{"internal/domain"}, Globs: []string{"internal/domain/**"}},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WriteScopesOverlap(tc.a, tc.b); got != tc.want {
				t.Fatalf("WriteScopesOverlap(%+v, %+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			// The predicate is symmetric.
			if got := WriteScopesOverlap(tc.b, tc.a); got != tc.want {
				t.Fatalf("WriteScopesOverlap(%+v, %+v) = %v, want %v (symmetry)", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

// TestAdmitSingleWriterRejectsOverlappingWriters asserts the single-writer
// default rejects a run with more than one repository-mutating assignment.
func TestAdmitSingleWriterRejectsOverlappingWriters(t *testing.T) {
	assignments := []WorkAssignment{
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/agent"}}),
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal/domain"}}),
	}

	err := AdmitWriteOwnership(OwnershipSingleWriter, assignments)
	if err == nil {
		t.Fatal("expected single-writer rejection, got nil")
	}
	var oe *OwnershipError
	if !errors.As(err, &oe) {
		t.Fatalf("error = %v, want *OwnershipError", err)
	}
	if oe.Mode != OwnershipSingleWriter {
		t.Fatalf("mode = %q, want %q", oe.Mode, OwnershipSingleWriter)
	}
	if len(oe.Diagnostics) == 0 || oe.Diagnostics[0].Code != OwnershipCodeSingleWriterExceeded {
		t.Fatalf("diagnostics = %+v, want single_writer_exceeded", oe.Diagnostics)
	}
}

// TestAdmitSingleWriterAdmitsOneWriter asserts one mutating assignment is admitted
// under the default policy.
func TestAdmitSingleWriterAdmitsOneWriter(t *testing.T) {
	assignments := []WorkAssignment{
		ownershipAssignment("assign-1", Scope{Paths: []string{"internal/agent"}}),
		analysisAssignment("assign-2", Scope{Paths: []string{"internal/agent"}}),
	}
	if err := AdmitWriteOwnership(OwnershipSingleWriter, assignments); err != nil {
		t.Fatalf("single-writer admitted one mutating writer, got %v", err)
	}
}

// TestAdmitDisjointMultiWriterAdmitsDisjoint verifies disjoint mutating scopes are
// admitted.
func TestAdmitDisjointMultiWriterAdmitsDisjoint(t *testing.T) {
	assignments := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal/agent"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/domain"}}),
		ownershipAssignment("assign-c", Scope{Paths: []string{"cmd"}}),
	}
	if err := AdmitWriteOwnership(OwnershipDisjointMultiWriter, assignments); err != nil {
		t.Fatalf("disjoint multi-writer rejected disjoint scopes: %v", err)
	}
}

// TestAdmitDisjointMultiWriterRejectsOverlap verifies overlapping mutating scopes
// are rejected with an explicit typed failure and stable diagnostic code.
func TestAdmitDisjointMultiWriterRejectsOverlap(t *testing.T) {
	assignments := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/orchestration"}}),
	}
	err := AdmitWriteOwnership(OwnershipDisjointMultiWriter, assignments)
	if err == nil {
		t.Fatal("expected disjoint multi-writer rejection, got nil")
	}
	var oe *OwnershipError
	if !errors.As(err, &oe) {
		t.Fatalf("error = %v, want *OwnershipError", err)
	}
	if len(oe.Diagnostics) == 0 || oe.Diagnostics[0].Code != OwnershipCodeOverlappingScope {
		t.Fatalf("diagnostics = %+v, want overlapping_write_scope", oe.Diagnostics)
	}
}

// TestOwnershipDiagnosticsAreOrderIndependent verifies identical inputs yield
// byte-identical diagnostics regardless of assignment order.
func TestOwnershipDiagnosticsAreOrderIndependent(t *testing.T) {
	base := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/orchestration"}}),
		ownershipAssignment("assign-c", Scope{Paths: []string{"internal/orchestration/ownership.go"}}),
	}

	perms := [][]int{
		{0, 1, 2},
		{2, 1, 0},
		{1, 0, 2},
		{2, 0, 1},
	}

	var want string
	for i, perm := range perms {
		set := make([]WorkAssignment, len(perm))
		for j, idx := range perm {
			set[j] = base[idx]
		}
		err := AdmitWriteOwnership(OwnershipDisjointMultiWriter, set)
		if err == nil {
			t.Fatalf("perm %v: expected rejection", perm)
		}
		var oe *OwnershipError
		if !errors.As(err, &oe) {
			t.Fatalf("perm %v: error = %v, want *OwnershipError", perm, err)
		}
		got := renderOwnershipDiagnostics(oe.Diagnostics)
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("perm %v: diagnostics not order-independent\n got: %s\nwant: %s", perm, got, want)
		}
	}
}

// renderOwnershipDiagnostics renders diagnostics for equality comparison.
func renderOwnershipDiagnostics(diags []OwnershipDiagnostic) string {
	out := ""
	for _, d := range diags {
		out += fmt.Sprintf("%s|%s|%s\n", d.Code, d.AssignmentA, d.AssignmentB)
	}
	return out
}

// TestCoordinatorDefaultSingleWriterRejectsMultipleWriters asserts the default
// single-writer policy rejects a run with more than one mutating assignment and
// no worker executes.
func TestCoordinatorDefaultSingleWriterRejectsMultipleWriters(t *testing.T) {
	ag := &countingAgent{}
	c := NewCoordinator(ag)
	assignments := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal/agent"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/domain"}}),
	}

	report := c.Run(context.Background(), ExecutionPolicy{}, assignments)
	if ag.calls != 0 {
		t.Fatalf("agent invoked %d times, want 0 (no partial dispatch)", ag.calls)
	}
	if len(report.Outcomes) != len(assignments) {
		t.Fatalf("outcomes = %d, want %d", len(report.Outcomes), len(assignments))
	}
	for _, o := range report.Outcomes {
		if o.Failure != FailureOwnership {
			t.Fatalf("outcome %q failure = %q, want %q", o.AssignmentID, o.Failure, FailureOwnership)
		}
	}
}

// TestCoordinatorDefaultSingleWriterAdmitsOneWriter asserts the single-writer path
// executes exactly the one admitted mutating assignment.
func TestCoordinatorDefaultSingleWriterAdmitsOneWriter(t *testing.T) {
	ag := &countingAgent{}
	c := NewCoordinator(ag)
	assignments := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal/agent"}}),
		analysisAssignment("assign-b", Scope{Paths: []string{"internal"}}),
	}

	report := c.Run(context.Background(), ExecutionPolicy{}, assignments)
	if len(report.Outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(report.Outcomes))
	}
	if report.Completed != 2 {
		t.Fatalf("completed = %d, want 2", report.Completed)
	}
	if ag.calls != 2 {
		t.Fatalf("agent invoked %d times, want 2", ag.calls)
	}
}

// TestCoordinatorDisjointMultiWriterAdmitsDisjoint asserts the disjoint
// multi-writer path proceeds and preserves sequential execution semantics.
func TestCoordinatorDisjointMultiWriterAdmitsDisjoint(t *testing.T) {
	ag := &countingAgent{}
	c := NewCoordinator(ag)
	assignments := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal/agent"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/domain"}}),
	}

	report := c.Run(context.Background(), ExecutionPolicy{Ownership: OwnershipDisjointMultiWriter}, assignments)
	if report.Completed != 2 {
		t.Fatalf("completed = %d, want 2", report.Completed)
	}
	if ag.calls != 2 {
		t.Fatalf("agent invoked %d times, want 2", ag.calls)
	}
	if !report.Succeeded() {
		t.Fatal("disjoint multi-writer run did not succeed")
	}
}

// TestCoordinatorDisjointMultiWriterRejectsOverlap asserts overlapping mutating
// scopes are rejected before any execution, independent of assignment order.
func TestCoordinatorDisjointMultiWriterRejectsOverlap(t *testing.T) {
	base := []WorkAssignment{
		ownershipAssignment("assign-a", Scope{Paths: []string{"internal"}}),
		ownershipAssignment("assign-b", Scope{Paths: []string{"internal/orchestration"}}),
	}

	orders := [][]int{{0, 1}, {1, 0}}
	for _, order := range orders {
		set := []WorkAssignment{base[order[0]], base[order[1]]}
		ag := &countingAgent{}
		c := NewCoordinator(ag)
		report := c.Run(context.Background(), ExecutionPolicy{Ownership: OwnershipDisjointMultiWriter}, set)
		if ag.calls != 0 {
			t.Fatalf("order %v: agent invoked %d times, want 0", order, ag.calls)
		}
		for _, o := range report.Outcomes {
			if o.Failure != FailureOwnership {
				t.Fatalf("order %v: failure = %q, want %q", order, o.Failure, FailureOwnership)
			}
		}
	}
}

// TestCoordinatorOwnershipIndependentOfPromptInstructions asserts that a fixture
// that "asks nicely" in Context not to edit the same file is still rejected when
// scopes overlap: correctness is enforced by code, not by prompt text.
func TestCoordinatorOwnershipIndependentOfPromptInstructions(t *testing.T) {
	ag := &countingAgent{}
	c := NewCoordinator(ag)

	polite := AssignmentBuilder{
		AssignmentID:       "assign-a",
		TaskID:             "ORCH-005",
		Capability:         agent.Implement,
		Task:               "edit files",
		Scope:              Scope{Paths: []string{"internal"}},
		Context:            "Please do not edit the same file as other workers; coordinate nicely.",
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
	}
	other := AssignmentBuilder{
		AssignmentID:       "assign-b",
		TaskID:             "ORCH-005",
		Capability:         agent.Implement,
		Task:               "edit files",
		Scope:              Scope{Paths: []string{"internal/orchestration"}},
		Context:            "Do not edit the same file; someone else may be working here.",
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
	}

	report := c.Run(context.Background(), ExecutionPolicy{Ownership: OwnershipDisjointMultiWriter}, []WorkAssignment{polite.Build(), other.Build()})
	if ag.calls != 0 {
		t.Fatalf("agent invoked %d times, want 0 despite polite context", ag.calls)
	}
	for _, o := range report.Outcomes {
		if o.Failure != FailureOwnership {
			t.Fatalf("outcome %q failure = %q, want %q", o.AssignmentID, o.Failure, FailureOwnership)
		}
	}
}
