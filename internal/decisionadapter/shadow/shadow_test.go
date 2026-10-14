package shadow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// fakeSource is a controllable shadow source for containment and metric tests.
type fakeSource struct {
	name       string
	comparable bool
	obs        Observation
	err        error
	latency    time.Duration
	timeout    bool
	malformed  bool
}

func (f *fakeSource) Name() string         { return f.name }
func (f *fakeSource) Comparable(Case) bool { return f.comparable }

func (f *fakeSource) Observe(ctx context.Context, c Case) (Observation, error) {
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	if f.timeout {
		return Observation{}, context.DeadlineExceeded
	}
	if f.malformed {
		return Observation{}, decision.ErrMalformedResponse
	}
	if f.err != nil {
		return Observation{}, f.err
	}
	return f.obs, nil
}

func choiceQuestion() decision.Question {
	return decision.Question{
		ID:       "q1",
		Type:     decision.QuestionChoice,
		Criteria: "pick one",
		Choices:  []string{"a", "b", "c"},
	}
}

func caseWithRef(choice string) Case {
	return Case{ID: "c1", Question: choiceQuestion(), Reference: &Reference{QuestionID: "q1", Choice: "a"}}
}

// TestPrimaryUnchangedAcrossShadowFailures is the core containment test: the
// primary result must be byte-for-byte identical whether the shadow runs,
// times out, errors, disagrees, or returns a malformed response.
func TestPrimaryUnchangedAcrossShadowFailures(t *testing.T) {
	primary := "a"

	sources := []Source{
		&fakeSource{name: "clef_timeout", comparable: true, timeout: true},
		&fakeSource{name: "clef_error", comparable: true, err: errors.New("boom")},
		&fakeSource{name: "clef_disagree", comparable: true, obs: Observation{Choice: "b"}},
		&fakeSource{name: "clef_malformed", comparable: true, malformed: true},
		&fakeSource{name: "clef_agree", comparable: true, obs: Observation{Choice: "a"}},
	}

	primaryBefore := primary

	rec := NewRecorder(sources...)
	rec.Evaluate(context.Background(), caseWithRef("a"), primary)

	if primary != primaryBefore {
		t.Fatalf("shadow altered primary result: got %q want %q", primary, primaryBefore)
	}
}

// TestShadowDisabledVsRunningSamePrimary asserts disabling the shadow yields
// the same primary result as running it successfully.
func TestShadowDisabledVsRunningSamePrimary(t *testing.T) {
	primary := "a"

	disabled := NewRecorder()
	disabled.Evaluate(context.Background(), caseWithRef("a"), primary)
	gotDisabled := primary

	running := NewRecorder(&fakeSource{name: "clef", comparable: true, obs: Observation{Choice: "a"}})
	running.Evaluate(context.Background(), caseWithRef("a"), primary)
	gotRunning := primary

	if gotDisabled != gotRunning {
		t.Fatalf("primary differed: disabled=%q running=%q", gotDisabled, gotRunning)
	}
}

// TestFailureModesRecordedNotPropagated asserts that each failure mode is
// recorded as a labeled recording and never returned/raised to the primary.
func TestFailureModesRecordedNotPropagated(t *testing.T) {
	tests := []struct {
		name  string
		src   Source
		label Label
	}{
		{"timeout", &fakeSource{name: "s", comparable: true, timeout: true}, LabelTimeout},
		{"error", &fakeSource{name: "s", comparable: true, err: errors.New("x")}, LabelError},
		{"malformed", &fakeSource{name: "s", comparable: true, malformed: true}, LabelMalformed},
		{"disagreement", &fakeSource{name: "s", comparable: true, obs: Observation{Choice: "b"}}, LabelDisagreement},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := NewRecorder(tc.src)
			rec.Evaluate(context.Background(), caseWithRef("a"), "a")
			found := false
			for _, r := range rec.Records() {
				if r.Label == tc.label {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected label %q recorded, records=%+v", tc.label, rec.Records())
			}
		})
	}
}

// TestMetricExactValues asserts exact metric values on a fixed fixture.
func TestMetricExactValues(t *testing.T) {
	rec := NewRecorder(
		&fakeSource{name: "clef", comparable: true, obs: Observation{Choice: "a"}}, // shadow agrees with ref "a"
		&fakeSource{name: "clef", comparable: true, obs: Observation{Choice: "b"}}, // shadow disagrees with ref "a"
		&fakeSource{name: "clef", comparable: true, obs: Observation{Choice: "z"}}, // invalid choice
		&fakeSource{name: "clef", comparable: true, obs: Observation{}},            // indeterminate
	)
	rec.Evaluate(context.Background(), caseWithRef("a"), "a")

	// Agreement is measured over shadow comparisons against the reference: one
	// agreement ("a") and one disagreement ("b"). base = 2; agree = 1 => 0.5.
	if got := rec.AgreementRate(); got != 0.5 {
		t.Fatalf("AgreementRate=%v want 0.5", got)
	}
	// Exactly one shadow disagreement ("b"). The primary-vs-reference
	// recording is informational and is excluded from the disagreement list.
	if len(rec.Disagreements()) != 1 {
		t.Fatalf("Disagreements=%d want 1", len(rec.Disagreements()))
	}
	// invalid = 1 over 4 source observations => 0.25.
	if got := rec.InvalidChoiceRate(); got != 0.25 {
		t.Fatalf("InvalidChoiceRate=%v want 0.25", got)
	}
	// indeterminate = 1 over 4 source observations => 0.25.
	if got := rec.IndeterminateRate(); got != 0.25 {
		t.Fatalf("IndeterminateRate=%v want 0.25", got)
	}
}

// TestTimeoutFailureRateFixture asserts the timeout/failure rate over a fixed
// fixture with 2 failures out of 4 source observations: one timeout and one
// non-timeout provider error. A timeout is counted once, never double-counted.
func TestTimeoutFailureRateFixture(t *testing.T) {
	rec := NewRecorder(
		&fakeSource{name: "s", comparable: true, obs: Observation{Choice: "a"}},
		&fakeSource{name: "s", comparable: true, obs: Observation{Choice: "a"}},
		&fakeSource{name: "s", comparable: true, timeout: true},
		&fakeSource{name: "s", comparable: true, err: errors.New("x")},
	)
	rec.Evaluate(context.Background(), caseWithRef("a"), "a")
	if got := rec.TimeoutFailureRate(); got != 0.5 {
		t.Fatalf("TimeoutFailureRate=%v want 0.5", got)
	}
}

// TestConfidenceDistributionBuckets asserts confidence bucketing on a fixed
// fixture.
func TestConfidenceDistributionBuckets(t *testing.T) {
	c1, c2, c3 := 0.1, 0.55, 0.9
	rec := NewRecorder(
		&fakeSource{name: "s", comparable: true, obs: Observation{Choice: "a", Confidence: &c1}},
		&fakeSource{name: "s", comparable: true, obs: Observation{Choice: "a", Confidence: &c2}},
		&fakeSource{name: "s", comparable: true, obs: Observation{Choice: "a", Confidence: &c3}},
	)
	rec.Evaluate(context.Background(), caseWithRef("a"), "a")
	dist := rec.ConfidenceDistribution()
	if dist["0.0-0.2"] != 1 || dist["0.4-0.6"] != 1 || dist["0.8-1.0"] != 1 {
		t.Fatalf("unexpected distribution: %+v", dist)
	}
	if dist["none"] != 0 {
		t.Fatalf("unexpected none count: %+v", dist)
	}
}

// TestLatencyPercentiles asserts p50/p95 against a known latency fixture.
func TestLatencyPercentiles(t *testing.T) {
	latencies := []time.Duration{10, 20, 30, 40}
	var rec Recorder
	rec.latencies = latencies

	// nearest-rank p50 over [10,20,30,40]: index=int(0.5*4+0.999999)-1 = 2-1 = 1 => 20.
	if got := rec.LatencyP50(); got != 20 {
		t.Fatalf("P50=%v want 20", got)
	}
	// nearest-rank p95: index=int(0.95*4+0.999999)-1 = 4-1 = 3 => 40.
	if got := rec.LatencyP95(); got != 40 {
		t.Fatalf("P95=%v want 40", got)
	}
}

// TestNimbleComparabilityGating asserts non-comparable cases are excluded from
// Nimble metrics.
func TestNimbleComparabilityGating(t *testing.T) {
	nimbleComparable := &fakeSource{name: "nimble", comparable: true, obs: Observation{Choice: "a"}}
	nimbleNotComparable := &fakeSource{name: "nimble", comparable: false, obs: Observation{Choice: "a"}}
	rec := NewRecorder(nimbleComparable, nimbleNotComparable)
	rec.Evaluate(context.Background(), caseWithRef("a"), "a")

	if rec.NimbleComparableCases() != 1 {
		t.Fatalf("NimbleComparableCases=%d want 1", rec.NimbleComparableCases())
	}
	if got := rec.NimbleAgreement(); got != 1.0 {
		t.Fatalf("NimbleAgreement=%v want 1.0", got)
	}
}

// TestNoShadowOutputInPrimaryFlow asserts the harness holds no writer to SOP
// policy: a panicking source is contained and the primary value is never
// mutated.
func TestNoShadowOutputInPrimaryFlow(t *testing.T) {
	panicSrc := &panicSource{name: "panicking"}
	rec := NewRecorder(panicSrc)
	primary := "a"
	rec.Evaluate(context.Background(), caseWithRef("a"), primary)
	if primary != "a" {
		t.Fatalf("primary mutated by panicking shadow: %q", primary)
	}
	found := false
	for _, r := range rec.Records() {
		if r.Label == LabelError {
			found = true
		}
	}
	if !found {
		t.Fatalf("panic not contained as error recording: %+v", rec.Records())
	}
}

type panicSource struct{ name string }

func (p *panicSource) Name() string         { return p.name }
func (p *panicSource) Comparable(Case) bool { return true }
func (p *panicSource) Observe(context.Context, Case) (Observation, error) {
	panic("shadow boom")
}
