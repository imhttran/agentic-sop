package planner

import (
	"strings"
	"testing"
)

// reqEvDoc builds a minimal valid rendered plan whose single stage optionally
// carries a Required Evidence section.
func reqEvDoc(section string) string {
	return "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\ns\n\n" +
		"## S001 — A\n\nDo A.\n\n### Dependencies\n\n- none\n\n" +
		"### Deliverables\n\n- done\n\n" + section +
		"### Acceptance Criteria\n\n- ok\n"
}

func parseValidate(t *testing.T, doc string) *Plan {
	t.Helper()
	p, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return p
}

func TestRequiredEvidenceRoundTrip(t *testing.T) {
	p := parseValidate(t, reqEvDoc("### Required Evidence\n\n- corpus:kb\n- source:docs/\n\n"))
	if len(p.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(p.Stages))
	}
	got := p.Stages[0].RequiredEvidence
	if len(got) != 2 || got[0] != "corpus:kb" || got[1] != "source:docs/" {
		t.Fatalf("RequiredEvidence = %v, want [corpus:kb source:docs/]", got)
	}
	rendered := p.RenderMarkdown()
	if !strings.Contains(rendered, "### Required Evidence") {
		t.Fatalf("render missing section: %q", rendered)
	}
	re := parseValidate(t, rendered)
	rg := re.Stages[0].RequiredEvidence
	if len(rg) != 2 || rg[0] != "corpus:kb" || rg[1] != "source:docs/" {
		t.Fatalf("round-trip RequiredEvidence = %v", rg)
	}
}

func TestRequiredEvidenceDeduplicatesDeterministically(t *testing.T) {
	p := parseValidate(t, reqEvDoc("### Required Evidence\n\n- corpus:kb\n- corpus:kb\n- source:a\n\n"))
	got := p.Stages[0].RequiredEvidence
	if len(got) != 2 || got[0] != "corpus:kb" || got[1] != "source:a" {
		t.Fatalf("dedup = %v, want [corpus:kb source:a]", got)
	}
}

func TestRequiredEvidenceAbsentIsCompatible(t *testing.T) {
	p := parseValidate(t, reqEvDoc(""))
	if len(p.Stages[0].RequiredEvidence) != 0 {
		t.Fatalf("absent section produced %v", p.Stages[0].RequiredEvidence)
	}
	if strings.Contains(p.RenderMarkdown(), "Required Evidence") {
		t.Fatal("render emitted a Required Evidence section for a plan without one")
	}
}

func TestRequiredEvidenceInvalidDeclarationsRejected(t *testing.T) {
	invalid := map[string]string{
		"empty identity":   "### Required Evidence\n\n- corpus:\n\n",
		"unknown kind":     "### Required Evidence\n\n- bogus:x\n\n",
		"missing colon":    "### Required Evidence\n\n- noColon\n\n",
		"whitespace id":    "### Required Evidence\n\n- source:a b\n\n",
		"unknown kind src": "### Required Evidence\n\n- repo:x\n\n",
	}
	for name, section := range invalid {
		t.Run(name, func(t *testing.T) {
			p, err := PlanFromMarkdown(reqEvDoc(section))
			if err != nil {
				t.Fatalf("PlanFromMarkdown: %v", err)
			}
			if err := p.Validate(); err == nil {
				t.Fatalf("Validate accepted an invalid declaration (%s), want error", section)
			}
		})
	}
}

func TestRequiredEvidencePreservesCapabilitiesAndDependencies(t *testing.T) {
	doc := "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\ns\n\n## S001 — A\n\nDo.\n\n" +
		"### Dependencies\n\n- none\n\n### Deliverables\n\n- d\n\n" +
		"### Required Evidence\n\n- corpus:kb\n\n### Acceptance Criteria\n\n- ok\n"
	p := parseValidate(t, doc)
	if len(p.Stages[0].RequiredEvidence) != 1 {
		t.Fatalf("RequiredEvidence = %v", p.Stages[0].RequiredEvidence)
	}
	// The Requires capability field is untouched by the new field.
	if len(p.Stages[0].Requires) != 0 {
		t.Fatalf("Requires unexpectedly populated: %v", p.Stages[0].Requires)
	}
}
