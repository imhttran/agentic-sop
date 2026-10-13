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
		MetricAdapterInvocation,
		MetricAdapter,
		MetricAdapterConfidence,
		MetricAdapterLatency,
		MetricAdapterInvalidChoice,
		MetricAdapterIndeterminate,
		MetricAdapterTimeoutFailure,
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

// TestDecisionAdapterAndConfidenceUnavailable proves the routing-confidence
// dimension is UNAVAILABLE on every fixture (no recorded source), and the
// adapter-agreement dimension is UNAVAILABLE where no usable shadow evidence
// exists (missing artifact, or an unknown schema version).
func TestDecisionAdapterAndConfidenceUnavailable(t *testing.T) {
	// Routing confidence has no recorded source on any fixture.
	for _, name := range []string{"present", "absent", "zero", "malformed", "partial", "decisions", "adapterbad"} {
		agg := fixture(t, name)
		r := findDecision(t, agg, MetricConfidence)
		if r.Status != StatusUnavailable || r.Value != UNAVAILABLE {
			t.Errorf("%s/%s = %+v, want UNAVAILABLE", name, MetricConfidence, r)
		}
		if r.Numerator != 0 || r.Denominator != 0 {
			t.Errorf("%s/%s has a fraction (%d/%d); UNAVAILABLE must have none", name, MetricConfidence, r.Numerator, r.Denominator)
		}
	}
	// Adapter agreement is UNAVAILABLE without usable evidence: no artifact, or an
	// unknown schema version (present but not interpreted).
	for _, name := range []string{"present", "absent", "zero", "malformed", "partial", "adapterbad"} {
		agg := fixture(t, name)
		r := findDecision(t, agg, MetricAdapter)
		if r.Value != UNAVAILABLE {
			t.Errorf("%s/%s = %+v, want UNAVAILABLE", name, MetricAdapter, r)
		}
		if r.Numerator != 0 || r.Denominator != 0 {
			t.Errorf("%s/%s has a fraction (%d/%d); UNAVAILABLE must have none", name, MetricAdapter, r.Numerator, r.Denominator)
		}
	}
}

// TestAdapterMetricsFromEvidence proves the adapter dimensions are computed from a
// recorded shadow-evidence artifact with explicit numerators and denominators that
// reproduce the adapter harness's definitions (comparable observations, referenced
// comparisons, nearest-rank percentiles).
func TestAdapterMetricsFromEvidence(t *testing.T) {
	agg := fixture(t, "decisions")

	inv := findDecision(t, agg, MetricAdapterInvocation)
	if inv.Status != StatusPartial || inv.Numerator != 6 || inv.Denominator != 6 || inv.Value != "6" {
		t.Errorf("adapter invocation = %+v, want num=6 den=6 value=6", inv)
	}
	if inv.CoveragePct != 50.0 {
		t.Errorf("adapter invocation coverage = %v, want 50.0 (1 of 2 runs)", inv.CoveragePct)
	}

	agr := findDecision(t, agg, MetricAdapter)
	if agr.Status != StatusPartial || agr.Numerator != 1 || agr.Denominator != 2 || agr.Value != "50.0" {
		t.Errorf("adapter agreement = %+v, want num=1 den=2 value=50.0", agr)
	}
	wantBreakdown := []BreakdownEntry{
		{Group: "source:clef", Key: "agreement", Count: 1},
		{Group: "source:clef", Key: "disagreement", Count: 0},
		{Group: "source:nimble", Key: "agreement", Count: 0},
		{Group: "source:nimble", Key: "disagreement", Count: 1},
	}
	if !reflect.DeepEqual(agr.Breakdown, wantBreakdown) {
		t.Errorf("adapter agreement breakdown = %+v, want %+v", agr.Breakdown, wantBreakdown)
	}

	conf := findDecision(t, agg, MetricAdapterConfidence)
	if conf.Numerator != 1 || conf.Denominator != 5 || conf.Value != "20.0" {
		t.Errorf("adapter confidence = %+v, want num=1 den=5 value=20.0", conf)
	}

	lat := findDecision(t, agg, MetricAdapterLatency)
	if lat.Numerator != 5 || lat.Denominator != 5 || lat.Value != "10" {
		t.Errorf("adapter latency = %+v, want num=5 den=5 value=10 (p50)", lat)
	}
	wantLat := []BreakdownEntry{
		{Group: "latency", Key: "p50_ms", Count: 10},
		{Group: "latency", Key: "p95_ms", Count: 50},
	}
	if !reflect.DeepEqual(lat.Breakdown, wantLat) {
		t.Errorf("adapter latency breakdown = %+v, want %+v", lat.Breakdown, wantLat)
	}

	for _, tc := range []struct {
		m    Metric
		want string
	}{
		{MetricAdapterInvalidChoice, "20.0"},
		{MetricAdapterIndeterminate, "20.0"},
		{MetricAdapterTimeoutFailure, "20.0"},
	} {
		r := findDecision(t, agg, tc.m)
		if r.Numerator != 1 || r.Denominator != 5 || r.Value != tc.want {
			t.Errorf("%s = %+v, want num=1 den=5 value=%s", tc.m, r, tc.want)
		}
	}
}

// TestAdapterUnknownSchemaIsUnavailable proves a present shadow-evidence artifact
// at an unknown schema version is present but UNAVAILABLE, never interpreted.
func TestAdapterUnknownSchemaIsUnavailable(t *testing.T) {
	agg := fixture(t, "adapterbad")
	inv := findDecision(t, agg, MetricAdapterInvocation)
	if inv.ArtifactsPresent != 1 {
		t.Errorf("adapter invocation artifacts_present = %d, want 1 (file present)", inv.ArtifactsPresent)
	}
	if inv.ValidMeasurements != 0 || inv.Value != UNAVAILABLE {
		t.Errorf("adapter invocation = %+v, want valid=0 value=UNAVAILABLE", inv)
	}
	if r := findDecision(t, agg, MetricAdapter); r.Value != UNAVAILABLE {
		t.Errorf("adapter agreement on unknown schema = %+v, want UNAVAILABLE", r)
	}
}

// TestAdapterDeterministic proves the adapter dimensions are deterministic across
// two aggregations over identical evidence.
func TestAdapterDeterministic(t *testing.T) {
	dir := filepath.Join("testdata", "decisions")
	first, err := AggregateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AggregateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first.Decisions)
	b, _ := json.Marshal(second.Decisions)
	if string(a) != string(b) {
		t.Errorf("adapter decision metrics are not deterministic:\n%s\n%s", a, b)
	}
}
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

// TestRenderDecisionsModelSelection proves the Model Selection block reports the
// SMALL/MEDIUM/LARGE counts with one-decimal percentages over the valid recorded
// selections, in the fixed order, plus an explicit Missing line. It is derived from
// the already-computed model_tier_distribution metric (no artifact is reread).
func TestRenderDecisionsModelSelection(t *testing.T) {
	agg := Aggregate{
		CorpusRuns: 120,
		Decisions: []MetricResult{{
			Metric: MetricModelTier, Status: StatusPartial, Value: "100", Unit: "runs",
			TotalRuns: 120, ValidMeasurements: 100, Numerator: 100, Denominator: 100,
			Breakdown: []BreakdownEntry{
				{Group: "tier", Key: "small", Count: 42},
				{Group: "tier", Key: "medium", Count: 38},
				{Group: "tier", Key: "large", Count: 20},
			},
		}},
	}
	out := RenderDecisions(agg)
	if !strings.Contains(out, "Model Selection") {
		t.Fatalf("missing Model Selection block:\n%s", out)
	}
	for _, want := range []string{"42.0%", "38.0%", "20.0%", "Total", "100", "Missing", "20"} {
		if !strings.Contains(out, want) {
			t.Errorf("Model Selection missing %q:\n%s", want, out)
		}
	}
	// Deterministic order SMALL < MEDIUM < LARGE.
	si := strings.Index(out, "SMALL")
	mi := strings.Index(out, "MEDIUM")
	li := strings.Index(out, "LARGE")
	if !(si < mi && mi < li) {
		t.Errorf("tier order not SMALL,MEDIUM,LARGE:\n%s", out)
	}
	if RenderDecisions(agg) != out {
		t.Error("Model Selection render is not deterministic")
	}
}

// TestRenderDecisionsModelSelectionUnavailable proves a zero denominator renders the
// tier percentages as UNAVAILABLE rather than 0%, and reports the whole corpus as
// missing.
func TestRenderDecisionsModelSelectionUnavailable(t *testing.T) {
	agg := Aggregate{
		CorpusRuns: 3,
		Decisions: []MetricResult{{
			Metric: MetricModelTier, Status: StatusPartial, Value: UNAVAILABLE,
			TotalRuns: 3, UnavailableMeasurements: 3,
		}},
	}
	out := RenderDecisions(agg)
	start := strings.Index(out, "Model Selection")
	if start < 0 {
		t.Fatalf("missing Model Selection block:\n%s", out)
	}
	end := strings.Index(out[start:], "\n\n")
	block := out[start : start+end]
	if !strings.Contains(block, "UNAVAILABLE") {
		t.Errorf("zero denominator should render UNAVAILABLE:\n%s", block)
	}
	if strings.Contains(block, "0.0%") {
		t.Errorf("zero denominator must not render 0%%:\n%s", block)
	}
	if !strings.Contains(block, "Missing") || !strings.Contains(block, "3") {
		t.Errorf("missing coverage should be reported:\n%s", block)
	}
}
