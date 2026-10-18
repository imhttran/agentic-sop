package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func manualPartial() *outcomeArtifact {
	return &outcomeArtifact{
		Status:   outcome.Partial,
		Criteria: []outcome.CriterionResult{{ID: "criterion-1", Required: true, Description: "docs are clear", Evidence: "no recorded acceptance decision"}},
		Mappings: []outcome.CriterionMapping{{CriterionID: "criterion-1", Required: true, Kind: outcome.EvidenceManual, Description: "docs are clear", ManualEvidence: "reviewer reads docs", Attempt: 1}},
	}
}

// TestOutcomeAcceptRecordsDecision proves --accept appends one traceable decision,
// leaves outcome.json byte-identical, and the next derived outcome is VERIFIED.
func TestOutcomeAcceptRecordsDecision(t *testing.T) {
	dir := t.TempDir()
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", manualPartial())
	before, _ := os.ReadFile(outcomeArtifactPath(dir, "T1"))

	code, stdout, stderr := runCLI(t, dir, "outcome", "T1", "--accept", "criterion-1", "--evidence", "read docs/x.md", "--by", "operator")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "recorded acceptance decision") {
		t.Errorf("stdout = %q", stdout)
	}
	after, _ := os.ReadFile(outcomeArtifactPath(dir, "T1"))
	if !bytes.Equal(before, after) {
		t.Error("outcome.json was rewritten")
	}
	decisions, err := readAcceptanceDecisions(dir, "T1")
	if err != nil || len(decisions) != 1 {
		t.Fatalf("decisions = %v err=%v, want one", decisions, err)
	}
	d := decisions[0]
	if d.Task != "T1" || d.CriterionID != "criterion-1" || d.Approver != "operator" || d.Evidence != "read docs/x.md" || d.RecordedAt == "" || d.Decision != outcome.DecisionAccept {
		t.Errorf("decision not traceable: %+v", d)
	}

	spec := &taskfile.Spec{ID: "T1", AcceptanceCriteria: []string{"docs are clear"}, AcceptanceMapping: []string{"criterion 1: manual: reviewer reads docs"}, Attempt: 1}
	if res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass}); res.Status != outcome.Verified {
		t.Fatalf("derived status = %s reasons=%v, want VERIFIED", res.Status, res.Reasons)
	}
}

// TestOutcomeAcceptRejectsInvalidDecision proves an undesignated approver, missing
// evidence, or a criterion not mapped to manual evidence writes nothing.
func TestOutcomeAcceptRejectsInvalidDecision(t *testing.T) {
	dir := t.TempDir()
	writeDesignatedApprovers(t, dir, "operator")
	writeOutcomeArtifactFor(t, dir, "T1", manualPartial())
	for name, args := range map[string][]string{
		"undesignated": {"--accept", "criterion-1", "--evidence", "e", "--by", "mallory"},
		"no evidence":  {"--accept", "criterion-1", "--by", "operator"},
		"not manual":   {"--accept", "criterion-2", "--evidence", "e", "--by", "operator"},
	} {
		if code, _, _ := runCLI(t, dir, append([]string{"outcome", "T1"}, args...)...); code == exitOK {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := os.Stat(acceptanceDecisionPath(dir, "T1")); err == nil {
		t.Fatal("an invalid decision was written")
	}
}

// TestVerifyLeavesHistoricalOutcomeUnchanged proves a pre-SOP-003 outcome.json is
// printed without compatibility/validation lines and left byte-identical.
func TestVerifyLeavesHistoricalOutcomeUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeOutcomeArtifactFor(t, dir, "T1", waivablePartial())
	before, _ := os.ReadFile(outcomeArtifactPath(dir, "T1"))
	code, stdout, _ := runCLI(t, dir, "verify", "T1")
	if code != exitOK || strings.Contains(stdout, "validation:") || strings.Contains(stdout, "compatibility:") {
		t.Fatalf("code=%d stdout=%s", code, stdout)
	}
	after, _ := os.ReadFile(outcomeArtifactPath(dir, "T1"))
	if !bytes.Equal(before, after) {
		t.Fatal("verify rewrote a historical outcome.json")
	}
}
