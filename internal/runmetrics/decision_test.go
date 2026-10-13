package runmetrics

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// findDecision returns the emitted decision metric m from an aggregate.
func findDecision(t *testing.T, agg Aggregate, m Metric) MetricResult {
	t.Helper()
	for _, r := range agg.Decisions {
		if r.Metric == m {
			return r
		}
	}
	t.Fatalf("decision metric %s not emitted", m)
	return MetricResult{}
}

// TestDecisionMetricOrder pins the deterministic emission order of the decision
// dimensions, which are appended after the seven RM-002 metrics.
func TestDecisionMetricOrder(t *testing.T) {
	agg := fixture(t, "decisions")
	want := []Metric{
		MetricModelTier,
		MetricOverride,
		MetricEscalation,
		MetricJEVInvoke,
		MetricAdapter,
		MetricConfidence,
	}
	got := make([]Metric, 0, len(agg.Decisions))
	for _, r := range agg.Decisions {
		got = append(got, r.Metric)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decision order = %v, want %v", got, want)
	}
	// The seven RM-002 metrics and their order are unchanged.
	if len(agg.Metrics) != 7 {
		t.Fatalf("RM-002 metrics = %d, want 7", len(agg.Metrics))
	}
}

// TestDecisionTierDistribution verifies the model-tier, selecting-source, and
// fallback breakdown is computed over the recorded selections only.
func TestDecisionTierDistribution(t *testing.T) {
	agg := fixture(t, "decisions")
	r := findDecision(t, agg, MetricModelTier)
	if r.Status != StatusPartial {
		t.Errorf("tier status = %q, want PARTIAL", r.Status)
	}
	if r.Numerator != 2 || r.Denominator != 2 || r.Value != "2" {
		t.Errorf("tier = %+v, want num=2 den=2 value=2", r)
	}
	want := []BreakdownEntry{
		{Group: "tier", Key: "small", Count: 1},
		{Group: "tier", Key: "large", Count: 1},
		{Group: "source", Key: "cli", Count: 1},
		{Group: "source", Key: "router", Count: 1},
		{Group: "fallback", Key: "applied", Count: 1},
	}
	if !reflect.DeepEqual(r.Breakdown, want) {
		t.Errorf("breakdown = %+v, want %+v", r.Breakdown, want)
	}
}

// TestDecisionOverrideRate verifies the manual override rate counts only runs
// whose model-selection.json records the CLI source, over the runs that recorded
// a source.
func TestDecisionOverrideRate(t *testing.T) {
	agg := fixture(t, "decisions")
	r := findDecision(t, agg, MetricOverride)
	if r.Numerator != 1 || r.Denominator != 2 || r.Value != "50.0" {
		t.Errorf("override = %+v, want num=1 den=2 value=50.0", r)
	}
}

// TestDecisionEscalationRate verifies the escalation rate counts a recorded
// recovery escalation (routing.json source "escalation") or a NEEDS_HUMAN
// terminal decision, over the runs that recorded either source.
func TestDecisionEscalationRate(t *testing.T) {
	agg := fixture(t, "decisions")
	r := findDecision(t, agg, MetricEscalation)
	if r.Numerator != 1 || r.Denominator != 2 || r.Value != "50.0" {
		t.Errorf("escalation = %+v, want num=1 den=2 value=50.0", r)
	}
}

// TestDecisionJEVInvocation verifies the JEV invocation count sums the recorded
// metrics.json JEV.Invocations and treats a present metrics.json without a JEV
// section as a recorded zero, not UNAVAILABLE.
func TestDecisionJEVInvocation(t *testing.T) {
	agg := fixture(t, "decisions")
	r := findDecision(t, agg, MetricJEVInvoke)
	if r.Status != StatusAvailable {
		t.Errorf("jev status = %q, want AVAILABLE", r.Status)
	}
	if r.Numerator != 3 || r.Denominator != 1 || r.Value != "3" {
		t.Errorf("jev = %+v, want num=3 den=1 value=3", r)
	}

	// On the present fixture metrics.json parsed but carries no JEV section: a
	// recorded zero invocation, distinct from an absent metrics.json.
	pres := findDecision(t, fixture(t, "present"), MetricJEVInvoke)
	if pres.Status != StatusAvailable || pres.Value != "0" || pres.Denominator != 1 {
		t.Errorf("present jev = %+v, want AVAILABLE value=0 den=1", pres)
	}
	// On the absent fixture metrics.json is absent: UNAVAILABLE, never zero.
	if ab := findDecision(t, fixture(t, "absent"), MetricJEVInvoke); ab.Value != UNAVAILABLE {
		t.Errorf("absent jev = %+v, want UNAVAILABLE", ab)
	}
}

// TestDecisionAdapterAndConfidenceUnavailable proves the two dimensions with no
// recorded source are UNAVAILABLE on every fixture, with zero numerator and
// denominator, and are never fabricated as an agreement or a confidence.
func TestDecisionAdapterAndConfidenceUnavailable(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero", "malformed", "partial", "decisions"} {
		agg := fixture(t, name)
		for _, m := range []Metric{MetricAdapter, MetricConfidence} {
			r := findDecision(t, agg, m)
			if r.Status != StatusUnavailable || r.Value != UNAVAILABLE {
				t.Errorf("%s/%s = %+v, want UNAVAILABLE", name, m, r)
			}
			if r.Numerator != 0 || r.Denominator != 0 {
				t.Errorf("%s/%s has a fraction (%d/%d); UNAVAILABLE must have none", name, m, r.Numerator, r.Denominator)
			}
			if r.CoveragePct != 0 {
				t.Errorf("%s/%s coverage = %v, want 0", name, m, r.CoveragePct)
			}
		}
	}
}

// TestDecisionAbsentIsUnavailable proves a run with no model-selection.json and
// no routing.json is UNAVAILABLE for the tier and override dimensions, never a
// "default" tier or a zero override.
func TestDecisionAbsentIsUnavailable(t *testing.T) {
	agg := fixture(t, "absent")
	for _, m := range []Metric{MetricModelTier, MetricOverride} {
		if r := findDecision(t, agg, m); r.Value != UNAVAILABLE {
			t.Errorf("absent/%s = %+v, want UNAVAILABLE", m, r)
		}
	}
}

// TestDecisionReproducibleAndRender proves two aggregations over identical inputs
// produce byte-identical JSON and byte-identical rendered text, and that the
// renderer includes the decision section. The aggregator is deterministic and
// carries no wall-clock field.
func TestDecisionReproducibleAndRender(t *testing.T) {
	dir := filepath.Join("testdata", "decisions")
	first, err := AggregateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AggregateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.MarshalIndent(first, "", "  ")
	b, _ := json.MarshalIndent(second, "", "  ")
	if string(a) != string(b) {
		t.Errorf("two aggregations differ\n%s\n%s", a, b)
	}
	text := RenderText(first)
	if !strings.Contains(text, "Decision telemetry") {
		t.Errorf("render missing decision section:\n%s", text)
	}
	if RenderText(first) != RenderText(second) {
		t.Error("rendered text is not deterministic")
	}
}
