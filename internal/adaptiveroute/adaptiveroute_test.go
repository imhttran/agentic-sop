package adaptiveroute

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/model"
)

func out(class model.Class, successes, failures int) []Outcome {
	var o []Outcome
	for i := 0; i < successes; i++ {
		o = append(o, Outcome{Class: class, Capability: agent.Implement, Success: true})
	}
	for i := 0; i < failures; i++ {
		o = append(o, Outcome{Class: class, Capability: agent.Implement, Success: false})
	}
	return o
}

func TestOverrideWins(t *testing.T) {
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Override: model.ClassLarge})
	if d.Class != model.ClassLarge || d.Reason != ReasonOverride {
		t.Fatalf("override decision = %+v", d)
	}
}

func TestNoEvidenceRetainsBaseline(t *testing.T) {
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement})
	if d.Class != model.ClassSmall || d.Reason != ReasonNoEvidence || d.Baseline != model.ClassSmall {
		t.Fatalf("no-evidence decision = %+v", d)
	}
	if !strings.Contains(d.Evidence, "baseline=small") {
		t.Errorf("evidence summary = %q", d.Evidence)
	}
}

func TestBaselineSufficient(t *testing.T) {
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: out(model.ClassSmall, 3, 0)})
	if d.Class != model.ClassSmall || d.Reason != ReasonBaselineSufficient {
		t.Fatalf("sufficient decision = %+v", d)
	}
}

func TestEscalatesWhenBelowThreshold(t *testing.T) {
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: out(model.ClassSmall, 1, 3)})
	if d.Class != model.ClassMedium || d.Reason != ReasonEscalated {
		t.Fatalf("escalation decision = %+v", d)
	}
	if !strings.Contains(d.Evidence, "small=1/4") {
		t.Errorf("evidence summary = %q", d.Evidence)
	}
}

func TestEscalatesToLargeWhenMediumAlsoFails(t *testing.T) {
	ev := append(out(model.ClassSmall, 0, 4), out(model.ClassMedium, 1, 3)...)
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: ev})
	if d.Class != model.ClassLarge || d.Reason != ReasonEscalated {
		t.Fatalf("decision = %+v", d)
	}
}

func TestNeverDowngradesBelowBaseline(t *testing.T) {
	d := Route(Input{Baseline: model.ClassLarge, Capability: agent.Implement, Evidence: out(model.ClassSmall, 5, 0)})
	if d.Class != model.ClassLarge {
		t.Fatalf("policy downgraded below baseline: %+v", d)
	}
}

func TestExhaustedWhenLargestFails(t *testing.T) {
	d := Route(Input{Baseline: model.ClassLarge, Capability: agent.Implement, Evidence: out(model.ClassLarge, 0, 4)})
	if d.Class != model.ClassLarge || d.Reason != ReasonExhausted {
		t.Fatalf("decision = %+v", d)
	}
}

func TestEvidenceScopedToCapability(t *testing.T) {
	ev := []Outcome{
		{Class: model.ClassSmall, Capability: agent.Review, Success: false},
		{Class: model.ClassSmall, Capability: agent.Review, Success: false},
		{Class: model.ClassSmall, Capability: agent.Review, Success: false},
	}
	d := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: ev})
	if d.Class != model.ClassSmall || d.Reason != ReasonNoEvidence {
		t.Fatalf("review evidence must not affect IMPLEMENT routing: %+v", d)
	}
}

func TestExplainableAndDeterministic(t *testing.T) {
	in := Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: out(model.ClassSmall, 1, 3)}
	a, b := Route(in), Route(in)
	if a != b {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
	if a.Reason == "" || a.Evidence == "" {
		t.Errorf("decision must expose a reason and evidence: %+v", a)
	}
}

func TestDefaultsApplied(t *testing.T) {
	if DefaultMinSample != 3 || DefaultThreshold != 0.75 {
		t.Fatalf("unexpected defaults: %d, %v", DefaultMinSample, DefaultThreshold)
	}
	// Two failures is below the default sample size, so the baseline is retained.
	short := Route(Input{Baseline: model.ClassSmall, Capability: agent.Implement, Evidence: out(model.ClassSmall, 0, 2)})
	if short.Class != model.ClassSmall || short.Reason != ReasonNoEvidence {
		t.Errorf("below-min-sample decision = %+v", short)
	}
}
