package orchestration

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// conflictCountingAgent counts Generate invocations so tests can prove that no
// worker runs on a detected conflict.
type conflictCountingAgent struct{ calls int }

func (a *conflictCountingAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	a.calls++
	return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
}

func TestConflictClassValues(t *testing.T) {
	want := map[ConflictClass]string{
		ConflictOverlappingFiles:        "overlapping_files",
		ConflictOverlappingSymbols:      "overlapping_symbols",
		ConflictIncompatiblePatches:     "incompatible_patches",
		ConflictStaleRepositoryIdentity: "stale_repository_identity",
		ConflictDependencyChanged:       "dependency_changed",
		ConflictStaleWorkerResult:       "stale_worker_result",
	}
	for class, value := range want {
		if string(class) != value {
			t.Fatalf("class %v = %q, want %q", class, string(class), value)
		}
	}
	if len(allConflictClasses) != 6 {
		t.Fatalf("want 6 conflict classes, got %d", len(allConflictClasses))
	}
}

// TestConflictDecisionDistinct proves ConflictDecision is a distinct named type
// from Mode and FailureCode.
func TestConflictDecisionDistinct(t *testing.T) {
	var d ConflictDecision = DecisionReject
	var m Mode = ModeSingle
	var f FailureCode = FailureConflict
	_ = string(d)
	_ = string(m)
	_ = string(f)
	if ConflictDecision(FailureConflict) != ConflictDecision("conflict_rejected") {
		t.Fatal("unexpected conflict failure code")
	}
	if DecisionNone == ConflictDecision(ModeSingle) {
		t.Fatal("DecisionNone must not equal ModeSingle")
	}
}

func validIdentity() RepositoryIdentity { return RepositoryIdentity{Revision: "head-1"} }

// cleanInput returns a conflict-free input: disjoint files/symbols, matching
// identities, and matching dependencies.
func cleanInput() ConflictInput {
	return ConflictInput{
		CurrentIdentity: validIdentity(),
		CurrentDependencies: map[string][]string{
			"a": {"dep-1"},
			"b": {"dep-1"},
		},
		Units: []ConflictUnit{
			{AssignmentID: "a", Files: []string{"pkg/a.go"}, Symbols: []string{"pkg.A"}, RepositoryIdentity: validIdentity(), Dependencies: []string{"dep-1"}},
			{AssignmentID: "b", Files: []string{"pkg/b.go"}, Symbols: []string{"pkg.B"}, RepositoryIdentity: validIdentity(), Dependencies: []string{"dep-1"}},
		},
	}
}

func TestDetectConflictsCleanInputHasNoFindings(t *testing.T) {
	if findings := DetectConflicts(cleanInput()); len(findings) != 0 {
		t.Fatalf("clean input produced findings: %v", findings)
	}
}

// TestDetectConflictsPerClass is the table-driven per-class detection test. Each
// case must produce exactly one finding of the crafted class.
func TestDetectConflictsPerClass(t *testing.T) {
	tests := []struct {
		name         string
		input        ConflictInput
		wantClass    ConflictClass
		wantInvolved []string
	}{
		{
			name: "overlapping_files",
			input: func() ConflictInput {
				in := cleanInput()
				in.Units[1].Files = []string{"pkg/a.go"}
				return in
			}(),
			wantClass:    ConflictOverlappingFiles,
			wantInvolved: []string{"a", "b"},
		},
		{
			name: "overlapping_symbols",
			input: func() ConflictInput {
				in := cleanInput()
				in.Units[1].Symbols = []string{"pkg.A"}
				return in
			}(),
			wantClass:    ConflictOverlappingSymbols,
			wantInvolved: []string{"a", "b"},
		},
		{
			name: "incompatible_patches",
			input: func() ConflictInput {
				in := cleanInput()
				in.Units[0].Patches = []PatchDescriptor{{PatchID: "p1", TargetScope: "pkg/a.go", BaseIdentity: "rev-1"}}
				in.Units[1].Patches = []PatchDescriptor{{PatchID: "p2", TargetScope: "pkg/a.go", BaseIdentity: "rev-2"}}
				return in
			}(),
			wantClass:    ConflictIncompatiblePatches,
			wantInvolved: []string{"a", "b"},
		},
		{
			name: "stale_repository_identity",
			input: func() ConflictInput {
				in := cleanInput()
				in.Units[1].RepositoryIdentity = RepositoryIdentity{Revision: "head-0"}
				return in
			}(),
			wantClass:    ConflictStaleRepositoryIdentity,
			wantInvolved: []string{"b"},
		},
		{
			name: "dependency_changed",
			input: func() ConflictInput {
				in := cleanInput()
				in.CurrentDependencies["b"] = []string{"dep-2"}
				return in
			}(),
			wantClass:    ConflictDependencyChanged,
			wantInvolved: []string{"b"},
		},
		{
			name: "stale_worker_result",
			input: func() ConflictInput {
				in := cleanInput()
				in.Units = append(in.Units[:0:0], in.Units...)
				in.Units[1].HasResult = true
				in.Units[1].ResultIdentity = RepositoryIdentity{Revision: "head-0"}
				return in
			}(),
			wantClass:    ConflictStaleWorkerResult,
			wantInvolved: []string{"b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := DetectConflicts(tc.input)
			var matched []ConflictFinding
			for _, f := range findings {
				if f.Class == tc.wantClass {
					matched = append(matched, f)
				}
			}
			if len(matched) != 1 {
				t.Fatalf("want exactly one finding of class %q, got %d in %v", tc.wantClass, len(matched), findings)
			}
			got := sortedUnique(matched[0].Assignments)
			want := sortedUnique(tc.wantInvolved)
			if !equalStringSets(got, want) {
				t.Fatalf("involved %v, want %v", got, want)
			}
		})
	}
}

// TestStaleIdentityRejected proves a missing or mismatched identity is always
// reported as stale_repository_identity and never conflict-free.
func TestStaleIdentityRejected(t *testing.T) {
	cases := []ConflictInput{
		func() ConflictInput {
			in := cleanInput()
			in.Units[0].RepositoryIdentity = RepositoryIdentity{}
			return in
		}(),
		func() ConflictInput {
			in := cleanInput()
			in.Units[0].RepositoryIdentity = RepositoryIdentity{Revision: "old"}
			return in
		}(),
		func() ConflictInput { in := cleanInput(); in.CurrentIdentity = RepositoryIdentity{}; return in }(),
	}
	for i, in := range cases {
		findings := DetectConflicts(in)
		found := false
		for _, f := range findings {
			if f.Class == ConflictStaleRepositoryIdentity {
				found = true
			}
		}
		if !found {
			t.Fatalf("case %d: stale identity not rejected, findings=%v", i, findings)
		}
		res := DecisionForConflicts(findings)
		if res.Decision == DecisionNone {
			t.Fatalf("case %d: stale identity produced no decision", i)
		}
	}
}

// TestDeterminismShuffled proves detection and decision output is byte-identical
// across repeated runs with shuffled input order.
func TestDeterminismShuffled(t *testing.T) {
	build := func(order []int) ConflictInput {
		in := cleanInput()
		in.CurrentDependencies["b"] = []string{"dep-2"}
		in.Units[1].RepositoryIdentity = RepositoryIdentity{Revision: "head-0"}
		units := make([]ConflictUnit, 0, len(in.Units))
		for _, idx := range order {
			units = append(units, in.Units[idx])
		}
		in.Units = units
		return in
	}

	var want string
	orders := [][]int{{0, 1}, {1, 0}, {1, 0}, {0, 1}, {0, 1}, {1, 0}}
	for i, order := range orders {
		findings := DetectConflicts(build(order))
		got := DecisionForConflicts(findings).Canonicalize()
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("order %v: canonical %q, want %q", order, got, want)
		}
	}
}

// TestDecisionTotalCoverage proves every non-empty finding set yields a non-zero
// decision and non-applicable assignments.
func TestDecisionTotalCoverage(t *testing.T) {
	classes := []ConflictClass{
		ConflictOverlappingFiles, ConflictOverlappingSymbols, ConflictIncompatiblePatches,
		ConflictStaleRepositoryIdentity, ConflictDependencyChanged, ConflictStaleWorkerResult,
	}
	for _, class := range classes {
		res := DecisionForConflicts([]ConflictFinding{{Class: class, Assignments: []string{"a"}}})
		if res.Decision == DecisionNone {
			t.Fatalf("class %q yielded zero decision", class)
		}
		if len(res.NonApplicable) != 1 || res.NonApplicable[0] != "a" {
			t.Fatalf("class %q non-applicable = %v", class, res.NonApplicable)
		}
	}
	if DecisionForConflicts(nil).Decision != DecisionNone {
		t.Fatal("empty findings must yield DecisionNone")
	}
}

// TestRunWithConflictsNoWorkerInvoked proves a conflicting set invokes no worker
// and that every involved assignment gets FailureConflict in caller order.
func TestRunWithConflictsNoWorkerInvoked(t *testing.T) {
	fa := &conflictCountingAgent{}
	c := NewCoordinator(fa)

	in := cleanInput()
	in.Units[1].RepositoryIdentity = RepositoryIdentity{Revision: "head-0"}
	assignments := []WorkAssignment{
		{AssignmentID: "a", TaskID: "t", Capability: agent.Implement, Task: "x", RepositoryIdentity: validIdentity()},
		{AssignmentID: "b", TaskID: "t", Capability: agent.Implement, Task: "x", RepositoryIdentity: validIdentity()},
	}

	report := c.RunWithConflicts(context.Background(), ExecutionPolicy{}, in, assignments)
	if fa.calls != 0 {
		t.Fatalf("worker invoked %d times on conflict, want 0", fa.calls)
	}
	if len(report.Outcomes) != len(assignments) {
		t.Fatalf("outcomes = %d, want %d", len(report.Outcomes), len(assignments))
	}
	for i, o := range report.Outcomes {
		if o.AssignmentID != assignments[i].AssignmentID {
			t.Fatalf("outcome %d id = %q, want %q", i, o.AssignmentID, assignments[i].AssignmentID)
		}
		if o.Failure != FailureConflict {
			t.Fatalf("outcome %d failure = %q, want %q", i, o.Failure, FailureConflict)
		}
		if o.OK() {
			t.Fatalf("outcome %d reported applicable on conflict", i)
		}
	}
}

// TestRunWithConflictsCleanBehavesNormally proves a conflict-free set dispatches
// normally.
func TestRunWithConflictsCleanBehavesNormally(t *testing.T) {
	fa := &conflictCountingAgent{}
	c := NewCoordinator(fa)
	assignments := []WorkAssignment{
		{AssignmentID: "a", TaskID: "t", Capability: agent.Implement, Task: "x", RepositoryIdentity: validIdentity()},
	}
	report := c.RunWithConflicts(context.Background(), ExecutionPolicy{}, cleanInput(), assignments)
	if fa.calls != 1 {
		t.Fatalf("worker calls = %d, want 1", fa.calls)
	}
	if !report.Outcomes[0].OK() {
		t.Fatalf("clean run not OK: %+v", report.Outcomes[0])
	}
}
