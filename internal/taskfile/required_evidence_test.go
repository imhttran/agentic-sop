package taskfile

import (
	"strings"
	"testing"
)

// Required evidence (RAG-SOP-003) must survive a Spec Render -> Parse round trip
// and must be absent (no section) when the Spec declares none, so an existing
// task file is byte-identical to before.

func TestRequiredEvidenceTaskfileRoundTrip(t *testing.T) {
	spec := &Spec{ID: "T1", Title: "t", Deliverables: []string{"d"}, RequiredEvidence: []string{"corpus:kb", "source:docs/"}}
	rendered := spec.Render()

	if !strings.Contains(rendered, "Required evidence") {
		t.Fatalf("render missing required-evidence section: %q", rendered)
	}
	parsed, err := Parse([]byte(rendered))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := parsed.RequiredEvidence
	if len(got) != 2 || got[0] != "corpus:kb" || got[1] != "source:docs/" {
		t.Fatalf("round-trip RequiredEvidence = %v", got)
	}
}

func TestRequiredEvidenceTaskfileAbsentIsCompatible(t *testing.T) {
	spec := &Spec{ID: "T1", Title: "t", Deliverables: []string{"d"}}
	rendered := spec.Render()

	if strings.Contains(rendered, "Required evidence") {
		t.Fatalf("render emitted a required-evidence section for a spec without one: %q", rendered)
	}
	parsed, err := Parse([]byte(rendered))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(parsed.RequiredEvidence) != 0 {
		t.Fatalf("absent section produced %v", parsed.RequiredEvidence)
	}
}
