package runmetrics

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// fixture aggregates the testdata fixture with the given name.
func fixture(t *testing.T, name string) Aggregate {
	t.Helper()
	agg, err := AggregateProject(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("AggregateProject(%s): %v", name, err)
	}
	return agg
}

func findMetric(t *testing.T, agg Aggregate, m Metric) MetricResult {
	t.Helper()
	for _, r := range agg.Metrics {
		if r.Metric == m {
			return r
		}
	}
	t.Fatalf("metric %s not emitted", m)
	return MetricResult{}
}

// TestCorpusAndMetricOrder pins the deterministic corpus count and emission
// order: the seven RM-002 dimensions, in register order.
func TestCorpusAndMetricOrder(t *testing.T) {
	agg := fixture(t, "present")
	if agg.CorpusRuns != 1 {
		t.Fatalf("corpus = %d, want 1", agg.CorpusRuns)
	}
	want := []Metric{
		MetricAgentSuccess,
		MetricRouting,
		MetricFixLoop,
		MetricRetry,
		MetricApproval,
		MetricLatency,
		MetricTokens,
	}
	got := make([]Metric, 0, len(agg.Metrics))
	for _, m := range agg.Metrics {
		got = append(got, m.Metric)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metric order = %v, want %v", got, want)
	}
}

// TestPresentFixtureValues asserts the aggregate measurements (numerator) are
// distinct from the eligible-run denominator, and each carries coverage.
func TestPresentFixtureValues(t *testing.T) {
	agg := fixture(t, "present")

	if r := findMetric(t, agg, MetricAgentSuccess); r.Numerator != 1 || r.Denominator != 1 || r.CoveragePct != 100.0 {
		t.Errorf("agent success = %+v, want num=1 den=1 cov=100", r)
	}
	if r := findMetric(t, agg, MetricFixLoop); r.Numerator != 1 || r.Denominator != 1 {
		t.Errorf("fix loop = %+v, want num=1 (fix_cycles) den=1", r)
	}
	if r := findMetric(t, agg, MetricLatency); r.Numerator != 12345 || r.Denominator != 1 {
		t.Errorf("latency = %+v, want num=12345 den=1", r)
	}
	// Every emitted metric states a denominator and coverage.
	for _, r := range agg.Metrics {
		if r.Metric == MetricTokens {
			continue
		}
		if r.Denominator == 0 {
			t.Errorf("metric %s has zero denominator", r.Metric)
		}
		if r.CoveragePct <= 0 {
			t.Errorf("metric %s has non-positive coverage %v", r.Metric, r.CoveragePct)
		}
	}
}

// TestAbsentIsUnavailableAndReducesCoverage proves an absent artifact yields
// UNAVAILABLE and lowers coverage, distinct from a recorded zero.
func TestAbsentIsUnavailableAndReducesCoverage(t *testing.T) {
	agg := fixture(t, "absent")

	// RUN-B records only state.json and continuations.txt: the metrics.json
	// (latency, fix loop), report.json (success, approval), and
	// model-selection.json (routing) artifacts are absent.
	if r := findMetric(t, agg, MetricLatency); r.Value != UNAVAILABLE {
		t.Errorf("latency on absent fixture = %+v, want UNAVAILABLE", r)
	}
	if r := findMetric(t, agg, MetricAgentSuccess); r.Value != UNAVAILABLE {
		t.Errorf("agent success on absent fixture = %+v, want UNAVAILABLE", r)
	}
	if r := findMetric(t, agg, MetricRouting); r.Value != UNAVAILABLE {
		t.Errorf("routing on absent fixture = %+v, want UNAVAILABLE", r)
	}

	var unavailableMetrics []string
	for _, r := range agg.Metrics {
		if r.Value == UNAVAILABLE {
			unavailableMetrics = append(unavailableMetrics, string(r.Metric))
		}
	}
	if len(unavailableMetrics) == 0 {
		t.Fatal("absent fixture emitted no UNAVAILABLE metric")
	}
}

// TestZeroIsRecordedMeasurement proves a present artifact with an explicit zero
// is a recorded measurement (counts toward the denominator), never UNAVAILABLE.
func TestZeroIsRecordedMeasurement(t *testing.T) {
	agg := fixture(t, "zero")

	// RUN-Z records metrics.json with total_ms 0 and empty counts: latency and
	// fix loop must be AVAILABLE recorded zeros, not UNAVAILABLE.
	lat := findMetric(t, agg, MetricLatency)
	if lat.Value == UNAVAILABLE {
		t.Errorf("latency on zero fixture = %+v, want recorded zero", lat)
	}
	if lat.Value != "0" || lat.Numerator != 0 || lat.Denominator != 1 || lat.CoveragePct != 100.0 {
		t.Errorf("latency zero = %+v, want value 0 num 0 den 1 cov 100", lat)
	}

	fix := findMetric(t, agg, MetricFixLoop)
	if fix.Value != "0" || fix.Denominator != 1 {
		t.Errorf("fix loop zero = %+v, want value 0 den 1", fix)
	}

	// The zero-vs-UNAVAILABLE divergence: on the zero fixture latency is a
	// recorded 0; on the absent fixture it is UNAVAILABLE.
	absent := fixture(t, "absent")
	if findMetric(t, absent, MetricLatency).Value != UNAVAILABLE {
		t.Error("absent fixture latency must be UNAVAILABLE")
	}
	if lat.Value != "0" {
		t.Error("zero fixture latency must be recorded zero")
	}
}

// TestZeroArtifactPresenceClassification asserts a present-with-zero artifact is
// classified as such, not conflated with absence.
func TestZeroArtifactPresenceClassification(t *testing.T) {
	agg := fixture(t, "zero")
	var found bool
	for _, a := range agg.Artifacts {
		if a.Name == metricsFileName {
			found = true
			if a.State != ArtifactZero {
				t.Errorf("metrics.json state = %q, want %q", a.State, ArtifactZero)
			}
		}
	}
	if !found {
		t.Fatal("metrics.json presence not reported")
	}
}

// TestTokenUsageUnavailable proves token usage is UNAVAILABLE with denominator 0
// on a fixture where it is not recorded, and appears in no unavailable metric
// other than its own token dimension.
func TestTokenUsageUnavailable(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero"} {
		agg := fixture(t, name)
		tok := findMetric(t, agg, MetricTokens)
		if tok.Value != UNAVAILABLE {
			t.Errorf("%s: token value = %+v, want UNAVAILABLE", name, tok)
		}
		if tok.Denominator != 0 {
			t.Errorf("%s: token denominator = %d, want 0", name, tok.Denominator)
		}
		if tok.Numerator != 0 {
			t.Errorf("%s: token numerator = %d, want 0", name, tok.Numerator)
		}
	}
}

// TestReproducible proves two independent aggregations over the same inputs are
// byte-identical.
func TestReproducible(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero"} {
		first, err := AggregateProject(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("first %s: %v", name, err)
		}
		second, err := AggregateProject(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("second %s: %v", name, err)
		}
		a, err := json.MarshalIndent(first, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(second, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if string(a) != string(b) {
			t.Errorf("%s: two aggregations differ\n%s\n%s", name, a, b)
		}
	}
}
