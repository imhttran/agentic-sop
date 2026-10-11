package runmetrics

import "testing"

// TestMalformedArtifactsAreUnavailable proves a present-but-malformed artifact is
// counted as present but is not a valid measurement: the metric is UNAVAILABLE,
// never a recorded zero. Missing, malformed, and zero are three distinct states.
func TestMalformedArtifactsAreUnavailable(t *testing.T) {
	agg := fixture(t, "malformed")
	if agg.CorpusRuns != 1 {
		t.Fatalf("corpus = %d, want 1", agg.CorpusRuns)
	}

	// report.json is present but unparseable: present, not a valid measurement.
	as := findMetric(t, agg, MetricAgentSuccess)
	if as.ArtifactsPresent != 1 {
		t.Errorf("agent success artifacts_present = %d, want 1", as.ArtifactsPresent)
	}
	if as.ValidMeasurements != 0 {
		t.Errorf("agent success valid_measurements = %d, want 0", as.ValidMeasurements)
	}
	if as.Value != UNAVAILABLE {
		t.Errorf("agent success = %q, want UNAVAILABLE", as.Value)
	}
	if as.UnavailableMeasurements != 1 {
		t.Errorf("agent success unavailable = %d, want 1", as.UnavailableMeasurements)
	}

	// model-selection.json present but unparseable: UNAVAILABLE, not a 0 count.
	if r := findMetric(t, agg, MetricRouting); r.ArtifactsPresent != 1 || r.Value != UNAVAILABLE {
		t.Errorf("routing = %+v, want present=1 value=UNAVAILABLE", r)
	}
	// metrics.json present but unparseable: latency and fix loop UNAVAILABLE.
	if r := findMetric(t, agg, MetricLatency); r.ArtifactsPresent != 1 || r.Value != UNAVAILABLE {
		t.Errorf("latency = %+v, want present=1 value=UNAVAILABLE", r)
	}
	if r := findMetric(t, agg, MetricFixLoop); r.ArtifactsPresent != 1 || r.Value != UNAVAILABLE {
		t.Errorf("fix loop = %+v, want present=1 value=UNAVAILABLE", r)
	}
	// continuations.txt is malformed, so retry has no readable source at all.
	if r := findMetric(t, agg, MetricRetry); r.ArtifactsPresent != 0 || r.Value != UNAVAILABLE {
		t.Errorf("retry = %+v, want present=0 value=UNAVAILABLE", r)
	}
	// approval: report.json present but unparseable.
	if r := findMetric(t, agg, MetricApproval); r.ArtifactsPresent != 1 || r.Value != UNAVAILABLE {
		t.Errorf("approval = %+v, want present=1 value=UNAVAILABLE", r)
	}
}

// TestPartiallyPopulatedArtifact proves an artifact present with an empty or
// incomplete payload is present but not a usable measurement for the metric it
// would feed, while a different, valid artifact is still measured.
func TestPartiallyPopulatedArtifact(t *testing.T) {
	agg := fixture(t, "partial")

	// report.json = {} : present and parseable, but records no decision, so the
	// success rate has no valid measurement and is UNAVAILABLE.
	as := findMetric(t, agg, MetricAgentSuccess)
	if as.ArtifactsPresent != 1 {
		t.Errorf("agent success artifacts_present = %d, want 1", as.ArtifactsPresent)
	}
	if as.ValidMeasurements != 0 || as.Value != UNAVAILABLE {
		t.Errorf("agent success = %+v, want valid=0 value=UNAVAILABLE", as)
	}
	// approval draws on report.json too: a report with no NEEDS_HUMAN decision is
	// not a valid approval measurement.
	if r := findMetric(t, agg, MetricApproval); r.Value != UNAVAILABLE {
		t.Errorf("approval = %+v, want UNAVAILABLE", r)
	}

	// metrics.json is valid: latency and fix loop are recorded measurements.
	lat := findMetric(t, agg, MetricLatency)
	if lat.Value != "500" || lat.ValidMeasurements != 1 || lat.Denominator != 1 {
		t.Errorf("latency = %+v, want value=500 valid=1 den=1", lat)
	}
	fix := findMetric(t, agg, MetricFixLoop)
	if fix.Value != "2" || fix.ValidMeasurements != 1 {
		t.Errorf("fix loop = %+v, want value=2 valid=1", fix)
	}
}

// TestMeasurementCategoryInvariants asserts the five quantities stay ordered and
// reconcilable for every metric across every fixture: total >= artifacts present
// >= valid measurements >= 0, and unavailable = total - valid. Coverage is the
// valid-measurement share of the corpus. This is the guard against conflating a
// missing artifact with a zero-valued measurement.
func TestMeasurementCategoryInvariants(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero", "malformed", "partial"} {
		agg := fixture(t, name)
		for _, r := range agg.Metrics {
			if r.TotalRuns != agg.CorpusRuns {
				t.Errorf("%s/%s: total_runs = %d, want corpus %d", name, r.Metric, r.TotalRuns, agg.CorpusRuns)
			}
			if r.ArtifactsPresent < r.ValidMeasurements {
				t.Errorf("%s/%s: artifacts_present %d < valid %d", name, r.Metric, r.ArtifactsPresent, r.ValidMeasurements)
			}
			if r.ArtifactsPresent > r.TotalRuns {
				t.Errorf("%s/%s: artifacts_present %d > total %d", name, r.Metric, r.ArtifactsPresent, r.TotalRuns)
			}
			if r.UnavailableMeasurements != r.TotalRuns-r.ValidMeasurements {
				t.Errorf("%s/%s: unavailable %d != total %d - valid %d",
					name, r.Metric, r.UnavailableMeasurements, r.TotalRuns, r.ValidMeasurements)
			}
			// A UNAVAILABLE metric never carries a numerator or denominator.
			if r.Value == UNAVAILABLE && (r.Numerator != 0 || r.Denominator != 0) {
				t.Errorf("%s/%s: UNAVAILABLE with num=%d den=%d", name, r.Metric, r.Numerator, r.Denominator)
			}
			// A measured metric reports a positive denominator.
			if r.Value != UNAVAILABLE && r.Denominator <= 0 {
				t.Errorf("%s/%s: measured with denominator %d", name, r.Metric, r.Denominator)
			}
		}
	}
}

// TestTokenUsageNotPolicy proves token usage never becomes a gate/budget/routing
// input: it is UNAVAILABLE, has no numerator or denominator, and appears only in
// its own dimension within the unavailable set.
func TestTokenUsageNotPolicy(t *testing.T) {
	for _, name := range []string{"present", "absent", "zero", "malformed", "partial"} {
		agg := fixture(t, name)
		tok := findMetric(t, agg, MetricTokens)
		if tok.Status != StatusUnavailable {
			t.Errorf("%s: token status = %q, want UNAVAILABLE", name, tok.Status)
		}
		if tok.Value != UNAVAILABLE || tok.Numerator != 0 || tok.Denominator != 0 {
			t.Errorf("%s: token = %+v, want UNAVAILABLE with zero fraction", name, tok)
		}
		if tok.Unit == "%" || tok.Unit == "runs" {
			t.Errorf("%s: token unit = %q, must not be a gating unit", name, tok.Unit)
		}
	}
}
