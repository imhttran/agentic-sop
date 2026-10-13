package ollamaagent

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestRAG025BZeroMutationAcceptedThenMissingEvidenceRejected is the SOP-HARD-05
// regression for RAG-025B: an existing, independently verified implementation is
// accepted with ZERO repository mutations, while a claim with missing criterion
// evidence is still rejected (never a silent pass).
func TestRAG025BZeroMutationAcceptedThenMissingEvidenceRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	cfg := testConfig("http://127.0.0.1:1")
	h := New(cfg, dir)
	req := agent.Request{Capability: agent.Implement, AcceptanceCriteria: []string{"c1"}, ValidationCommands: []string{"go vet ./..."}}

	fp, err := h.tools.RepositoryFingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	canonical := canonicalPath(t, dir, "a.go")

	proof := satisfProof(fp)
	proof.reads[canonical] = true
	id := commandIdentity("go vet ./...")
	proof.ran[id] = true
	proof.commands[id] = true

	// RAG-025B accept: zero mutations, complete evidence, validation executed.
	ok, rej := h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, proof, evidenceFinal("c1", []any{"a.go"}))
	if !ok {
		t.Fatalf("zero-mutation ALREADY_SATISFIED rejected: %s", rej.String())
	}

	// RAG-025B reject: missing criterion evidence is never accepted.
	ok, rej = h.verifySatisfiedDetailed(context.Background(), req, &executionState{}, satisfProof(fp),
		map[string]any{"status": string(agent.OutcomeCompleted), "evidence": map[string]any{"acceptance": []any{}}})
	if ok || rej.Code != RejectMissingCriterionEvidence {
		t.Fatalf("missing evidence: ok=%v code=%s, want rejection MISSING_CRITERION_EVIDENCE", ok, rej.Code)
	}
}
