package ollamaagent

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestVerifySatisfiedRejectsDirectoryEvidence: a cited path that exists but is a
// directory is never criterion evidence. The outcome stays a rejection, with a
// directory-specific diagnostic.
func TestVerifySatisfiedRejectsDirectoryEvidence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/keep.go", "package pkg\n")
	h := New(testConfig("http://127.0.0.1:1"), dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, satisfProof("fp"), evidenceFinal("c1", []any{"pkg"}))
	if ok {
		t.Fatal("a directory citation must not be accepted")
	}
	if rej.Code != RejectDirectoryEvidence {
		t.Fatalf("code = %s, want %s (detail=%s)", rej.Code, RejectDirectoryEvidence, rej.String())
	}
}

// TestEvidenceCorrectionListsObservedEvidence: the correction names the harness-owned
// eligible evidence (files actually read) and the successfully executed validation
// commands, and states the two are not interchangeable. Nothing unobserved is listed.
func TestEvidenceCorrectionListsObservedEvidence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	p := satisfProof("fp")
	p.reads[canonicalPath(t, dir, "a.go")] = true
	id := commandIdentity("go build ./...")
	p.ran[id] = true
	p.commands[id] = true
	p.commandText[id] = "go build ./..."

	msg := alreadySatisfiedEvidenceCorrection(correctionNeed{MissingCriteria: []string{"c1"}}, p.observedReads(dir), p.succeededCommands())
	if !strings.Contains(msg, "- a.go") {
		t.Errorf("correction does not list the observed read: %q", msg)
	}
	if !strings.Contains(msg, "- go build ./...") {
		t.Errorf("correction does not list the succeeded command: %q", msg)
	}
	if !strings.Contains(msg, "NOT file evidence for a criterion") {
		t.Errorf("correction does not state the file/command distinction: %q", msg)
	}
	if strings.Contains(msg, "pkg") {
		t.Errorf("correction lists evidence never observed: %q", msg)
	}
}

// TestEvidenceCorrectionEmptyObservations: with nothing read and nothing executed the
// correction says so rather than inventing evidence.
func TestEvidenceCorrectionEmptyObservations(t *testing.T) {
	p := satisfProof("fp")
	msg := alreadySatisfiedEvidenceCorrection(correctionNeed{MissingCriteria: []string{"c1"}}, p.observedReads(t.TempDir()), p.succeededCommands())
	if got := strings.Count(msg, "(none)"); got != 4 {
		t.Fatalf("want both lists empty, got %d markers: %q", got, msg)
	}
}

// TestVerifySatisfiedAcceptsReadFilesAndCommands: the positive path -- every criterion
// mapped to a file the harness observed being read, and every required validation
// command executed successfully.
func TestVerifySatisfiedAcceptsReadFilesAndCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	h := New(testConfig("http://127.0.0.1:1"), dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	baseline, err := h.tools.RepositoryFingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := satisfProof(baseline)
	p.reads[canonicalPath(t, dir, "a.go")] = true
	id := commandIdentity("go vet ./...")
	p.ran[id] = true
	p.commands[id] = true

	ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, p, evidenceFinal("c1", []any{"a.go"}))
	if !ok {
		t.Fatalf("read-file plus executed-command evidence rejected: %s", rej.String())
	}
}

// TestNEXT002DirectoryCitationRejected reproduces the observed NEXT-002 condition: the
// model executed the internal/commandpolicy package tests but never read a file under
// that path, then cited the directory as criterion evidence. Rejection is mandatory.
func TestNEXT002DirectoryCitationRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "internal/commandpolicy/commandpolicy.go", "package commandpolicy\n")
	h := New(testConfig("http://127.0.0.1:1"), dir)
	const criterion = "Every generated validation command is executable by the agent command harness"
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{criterion}, ValidationCommands: []string{"go build ./..."}}

	p := satisfProof("fp")
	// The model ran the package tests (and the build), so both commands succeeded, but it
	// read no file under internal/commandpolicy.
	for _, cmd := range []string{"go test ./internal/toolharness/ ./internal/commandpolicy/ ./internal/archtest/", "go build ./..."} {
		id := commandIdentity(cmd)
		p.ran[id] = true
		p.commands[id] = true
		p.commandText[id] = cmd
	}

	ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, p, evidenceFinal(criterion, []any{"internal/commandpolicy"}))
	if ok {
		t.Fatal("citing an unread package directory must never be accepted, even after running its tests")
	}
	if rej.Code != RejectDirectoryEvidence {
		t.Fatalf("code = %s, want %s (detail=%s)", rej.Code, RejectDirectoryEvidence, rej.String())
	}
}
