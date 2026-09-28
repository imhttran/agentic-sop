package jev

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestPolicyAllowsReadOnlyOperations asserts each allowed inspection operation
// is permitted.
func TestPolicyAllowsReadOnlyOperations(t *testing.T) {
	p := DefaultPolicy()
	allowed := []Operation{OpReadFile, OpSearchFiles, OpListFiles, OpInspectDiff, OpInspectCtx}
	for _, op := range allowed {
		if err := p.Allows(op); err != nil {
			t.Errorf("Allows(%q) = %v, want nil", op, err)
		}
	}
}

// TestPolicyDeniesMutatingOperations asserts each denied operation is rejected
// deterministically, without side effects.
func TestPolicyDeniesMutatingOperations(t *testing.T) {
	p := DefaultPolicy()
	denied := []Operation{OpWriteFile, OpDeleteFile, OpGitReset, OpGitClean, OpCommit, OpPush, OpMerge}
	for _, op := range denied {
		if err := p.Allows(op); err == nil {
			t.Errorf("Allows(%q) = nil, want denial", op)
		}
	}
}

// TestPolicyDeniesUnknownOperation confirms an unknown operation fails closed.
func TestPolicyDeniesUnknownOperation(t *testing.T) {
	if err := DefaultPolicy().Allows(Operation("teleport")); err == nil {
		t.Fatal("expected unknown operation to be denied")
	}
	if err := DefaultPolicy().Allows(""); err == nil {
		t.Fatal("expected empty operation to be denied")
	}
}

// TestPolicyClassifyCommand asserts destructive Git and arbitrary commands are
// denied while read-only Git inspection is permitted.
func TestPolicyClassifyCommand(t *testing.T) {
	p := DefaultPolicy()

	allowed := [][]string{
		{"git", "diff"},
		{"git", "status"},
		{"git", "log"},
		{"git", "-C", "/tmp", "diff"},
	}
	for _, argv := range allowed {
		if err := p.ClassifyCommand(argv); err != nil {
			t.Errorf("ClassifyCommand(%v) = %v, want nil", argv, err)
		}
	}

	denied := [][]string{
		{"git", "reset", "--hard"},
		{"git", "clean", "-fd"},
		{"git", "commit", "-m", "x"},
		{"git", "push"},
		{"git", "merge", "main"},
		{"rm", "-rf", "."},
		{"bash", "-c", "rm -rf ."},
		{},
	}
	for _, argv := range denied {
		if err := p.ClassifyCommand(argv); err == nil {
			t.Errorf("ClassifyCommand(%v) = nil, want denial", argv)
		}
	}
}

// TestPolicyDenialsAreDeterministic asserts the same input always yields the
// same denial, so a denied request is rejected deterministically.
func TestPolicyDenialsAreDeterministic(t *testing.T) {
	p := DefaultPolicy()
	cases := []Operation{OpWriteFile, OpGitReset, Operation("unknown"), ""}
	for _, op := range cases {
		first := p.Allows(op)
		second := p.Allows(op)
		if first == nil || second == nil {
			t.Fatalf("Allows(%q) = nil, want denial", op)
		}
		if first.Error() != second.Error() {
			t.Errorf("Allows(%q) not deterministic: %q vs %q", op, first, second)
		}
	}
}

// TestPolicyDenialNamesOperation asserts denials name the denied operation and
// state that JEV is read-only.
func TestPolicyDenialNamesOperation(t *testing.T) {
	err := DefaultPolicy().Allows(OpWriteFile)
	if err == nil {
		t.Fatal("expected write_file to be denied")
	}
	if !strings.Contains(err.Error(), string(OpWriteFile)) {
		t.Errorf("error = %q, want it to name %q", err, OpWriteFile)
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want it to state JEV is read-only", err)
	}
}

// TestImplementFixPolicyUnchanged is the regression guard: the JEV read-only
// policy does not alter the existing capability classification, and IMPLEMENT
// and FIX remain repository-mutating capabilities.
func TestImplementFixPolicyUnchanged(t *testing.T) {
	if !agent.IsRepositoryMutation(agent.Implement) {
		t.Error("IMPLEMENT must remain a repository-mutating capability")
	}
	if !agent.IsRepositoryMutation(agent.Fix) {
		t.Error("FIX must remain a repository-mutating capability")
	}
	for _, capability := range []agent.Capability{agent.Plan, agent.DesignTests, agent.DiagnoseFailure, agent.Review} {
		if agent.IsRepositoryMutation(capability) {
			t.Errorf("%s must remain non-mutating", capability)
		}
	}

	// Granting the JEV read-only policy does not grant mutation authority: the
	// mutation operations IMPLEMENT/FIX rely on remain denied for JEV.
	p := DefaultPolicy()
	if err := p.Allows(OpWriteFile); err == nil {
		t.Error("JEV must not gain write authority")
	}
	if err := p.Allows(OpCommit); err == nil {
		t.Error("JEV must not gain commit authority")
	}
}
