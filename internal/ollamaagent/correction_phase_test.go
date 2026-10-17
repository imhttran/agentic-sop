package ollamaagent

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

func needHarness(t *testing.T, dir string) (*Harness, string) {
	t.Helper()
	h := New(testConfig("http://127.0.0.1:1"), dir)
	fp, err := h.tools.RepositoryFingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return h, fp
}

// TestCorrectionNeedReportsMissingRequiredCommand: a claim whose criteria are read-backed but
// whose required validation never executed is now correctable, and the feedback names it.
func TestCorrectionNeedReportsMissingRequiredCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	h, fp := needHarness(t, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go build ./...", "go vet ./..."}}

	p := satisfProof(fp)
	p.reads[canonicalPath(t, dir, "a.go")] = true
	id := commandIdentity("go build ./...")
	p.ran[id] = true
	p.commands[id] = true // go vet ./... never ran

	need, offered := h.correctionNeed(context.Background(), req, &executionState{}, p, evidenceFinal("c1", []any{"a.go"}))
	if !offered {
		t.Fatalf("a missing required command must be correctable: %+v", need)
	}
	if len(need.NotExecuted) != 1 || need.NotExecuted[0] != "go vet ./..." {
		t.Fatalf("not executed = %v, want [go vet ./...]", need.NotExecuted)
	}
	if len(need.Failed) != 0 || len(need.MissingCriteria) != 0 || len(need.InvalidPaths) != 0 {
		t.Fatalf("unexpected need: %+v", need)
	}
	if !correctableRejection(RejectValidationNotExecuted) {
		t.Error("VALIDATION_NOT_EXECUTED must be correctable")
	}
}

// TestCorrectionNeedReportsFailedRequiredCommand: a required command that ran and failed is
// named as failed, never as succeeded or missing.
func TestCorrectionNeedReportsFailedRequiredCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	h, fp := needHarness(t, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	p := satisfProof(fp)
	p.reads[canonicalPath(t, dir, "a.go")] = true
	id := commandIdentity("go vet ./...")
	p.ran[id] = true
	p.commands[id] = false

	need, offered := h.correctionNeed(context.Background(), req, &executionState{}, p, evidenceFinal("c1", []any{"a.go"}))
	if !offered {
		t.Fatalf("a failed required command must be correctable: %+v", need)
	}
	if len(need.Failed) != 1 || need.Failed[0] != "go vet ./..." || len(need.NotExecuted) != 0 {
		t.Fatalf("failed = %v, notExecuted = %v", need.Failed, need.NotExecuted)
	}
	if !correctableRejection(RejectValidationFailed) {
		t.Error("VALIDATION_FAILED must be correctable")
	}
}

// TestCorrectionFeedbackReportsInvalidDirectoryCitation: a directory citation is reported with
// its path and reason, alongside the eligible read files.
func TestCorrectionFeedbackReportsInvalidDirectoryCitation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/keep.go", "package pkg\n")
	writeFile(t, dir, "a.go", "package a\n")
	h, fp := needHarness(t, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	p := satisfProof(fp)
	p.reads[canonicalPath(t, dir, "a.go")] = true
	id := commandIdentity("go vet ./...")
	p.ran[id] = true
	p.commands[id] = true

	need, offered := h.correctionNeed(context.Background(), req, &executionState{}, p, evidenceFinal("c1", []any{"pkg"}))
	if !offered {
		t.Fatalf("a directory citation must be correctable: %+v", need)
	}
	if len(need.InvalidPaths) != 1 || need.InvalidPaths[0].Path != "pkg" || need.InvalidPaths[0].Reason != "directory" {
		t.Fatalf("invalid paths = %+v, want pkg/directory", need.InvalidPaths)
	}

	msg := alreadySatisfiedEvidenceCorrection(need, p.observedReads(dir), p.succeededCommands())
	if !strings.Contains(msg, "path=pkg (directory)") {
		t.Errorf("feedback does not name the directory citation: %q", msg)
	}
	if !strings.Contains(msg, "- a.go") {
		t.Errorf("feedback does not offer the eligible read file: %q", msg)
	}
}

// TestCorrectionAcceptedWithValidReadEvidenceAndCommands: the acceptance predicate is
// unchanged -- read-backed criteria plus every required command succeeded is accepted.
func TestCorrectionAcceptedWithValidReadEvidenceAndCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "b.go", "package b\n")
	h, fp := needHarness(t, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1", "c2"}, ValidationCommands: []string{"go build ./...", "go vet ./..."}}

	p := satisfProof(fp)
	p.reads[canonicalPath(t, dir, "a.go")] = true
	p.reads[canonicalPath(t, dir, "b.go")] = true
	for _, cmd := range []string{"go build ./...", "go vet ./..."} {
		id := commandIdentity(cmd)
		p.ran[id] = true
		p.commands[id] = true
	}
	final := map[string]any{
		"status":   string(agent.OutcomeCompleted),
		"evidence": map[string]any{"acceptance": []any{map[string]any{"criterion": "c1", "paths": []any{"a.go"}}, map[string]any{"criterion": "c2", "paths": []any{"b.go"}}}},
	}
	if ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, p, final); !ok {
		t.Fatalf("valid corrected claim rejected: %s", rej.String())
	}
	if _, offered := h.correctionNeed(context.Background(), req, &executionState{}, p, final); offered {
		t.Fatal("a valid claim has nothing to correct")
	}
}

// TestCorrectionPhaseBudgetsAreEnforced: the phase ends on tool-budget or turn-budget
// exhaustion, so it can never become an unbounded reopening of the discovery window.
func TestCorrectionPhaseBudgetsAreEnforced(t *testing.T) {
	p := satisfProof("fp")
	p.openCorrectionPhase()
	for i := 0; i < correctionToolCalls; i++ {
		if !p.correctionPermits(toolharness.ToolReadFile, map[string]any{"path": "a.go"}) {
			t.Fatalf("tool call %d denied inside the phase budget", i+1)
		}
	}
	if p.correctionPermits(toolharness.ToolReadFile, map[string]any{"path": "a.go"}) {
		t.Fatal("the phase must refuse a tool call past its budget")
	}
	if p.correctionOpen {
		t.Fatal("the phase must close on tool-budget exhaustion")
	}

	q := satisfProof("fp")
	q.openCorrectionPhase()
	for i := 0; i < correctionIterations; i++ {
		if !q.correctionTurn() {
			t.Fatalf("turn %d denied inside the phase budget", i+1)
		}
	}
	if q.correctionTurn() {
		t.Fatal("the phase must refuse a turn past its budget")
	}
	if q.correctionOpen {
		t.Fatal("the phase must close on turn-budget exhaustion")
	}
}

// TestCorrectionPhaseDeniesMutation: only read-only tools and non-mutating commands are
// permitted; a mutating command or a write tool is refused and consumes no budget.
func TestCorrectionPhaseDeniesMutation(t *testing.T) {
	p := satisfProof("fp")
	p.openCorrectionPhase()
	if p.correctionPermits(toolharness.ToolRunCommand, map[string]any{"command": "go fmt ./..."}) {
		t.Fatal("a mutating command must be denied during correction")
	}
	if p.correctionPermits(toolharness.ToolWriteFile, map[string]any{"path": "a.go", "content": "x"}) {
		t.Fatal("a mutating tool must be denied during correction")
	}
	if p.correctionToolCalls != 0 {
		t.Fatalf("a denied call must not consume the phase budget: %d", p.correctionToolCalls)
	}
	if !p.correctionPermits(toolharness.ToolRunCommand, map[string]any{"command": "go vet ./..."}) {
		t.Fatal("a safe validation command must be permitted")
	}
	if !p.correctionPermits(toolharness.ToolReadFile, map[string]any{"path": "a.go"}) {
		t.Fatal("a read must be permitted")
	}
	if p.correctionToolCalls != 2 {
		t.Fatalf("permitted calls must consume the budget: %d", p.correctionToolCalls)
	}
}

// TestCorrectionPhaseClosesAndNeverReopens: a closed phase grants neither tools nor turns, so
// the single correction cannot be turned into a second opportunity.
func TestCorrectionPhaseClosesAndNeverReopens(t *testing.T) {
	p := satisfProof("fp")
	p.openCorrectionPhase()
	if !p.correctionTurn() {
		t.Fatal("an open phase must grant a turn")
	}
	p.closeCorrectionPhase()
	if p.correctionPermits(toolharness.ToolReadFile, map[string]any{"path": "a.go"}) {
		t.Fatal("a closed phase must not permit tools")
	}
	if p.correctionTurn() {
		t.Fatal("a closed phase must not grant turns")
	}
}

// TestCorrectionCannotSucceedLeadsToNeedsHuman: when the corrected claim is still invalid,
// exactly one correction is offered and the run then stops for a human rather than looping.
func TestCorrectionCannotSucceedLeadsToNeedsHuman(t *testing.T) {
	dir, req := satisfactionFixture(t)
	req.Capability = agent.Implement
	// The claim is read-backed but the required "go test ./..." never executes, so the first
	// claim is correctable (VALIDATION_NOT_EXECUTED); the second identical claim is not.
	_, srv := newFakeOllama(t, readAdd, readAddTest, satisfiedFinal, satisfiedFinal)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 30
	h := New(cfg, dir)
	ev := &mutationEvidence{}
	if _, _, err := h.CompleteWithEvidence(context.Background(), req, ev); err == nil || ev.alreadySatisfied {
		t.Fatalf("an uncorrected claim must stop for a human: err=%v evidence=%+v", err, ev)
	}
}
