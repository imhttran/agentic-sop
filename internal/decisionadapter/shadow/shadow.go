// Package shadow implements an adapter-side, observational shadow evaluation
// harness.
//
// The harness compares a primary decision against one or more comparison
// sources (Clef via the provider-neutral decision seam, deterministic expected
// outcomes, recorded SOP outcomes, and Nimble where semantically comparable)
// WITHOUT ever influencing the primary result. Every comparison runs in
// isolation; shadow timeouts, errors, disagreements, and malformed responses
// are captured as labeled recordings only. No shadow output is ever written to
// SOP policy or supplied to any SOP policy decision input or callback.
package shadow

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// Label classifies the outcome of a single shadow evaluation for one case.
// Labels are recordings only: they never propagate to the primary path.
type Label string

const (
	// LabelAgreement means the shadow answer agreed with the reference.
	LabelAgreement Label = "shadow_agreement"
	// LabelDisagreement means the shadow answer disagreed with the reference.
	LabelDisagreement Label = "shadow_disagreement"
	// LabelTimeout means the shadow call exceeded its deadline.
	LabelTimeout Label = "shadow_timeout"
	// LabelError means the shadow call returned a (non-timeout) error.
	LabelError Label = "shadow_error"
	// LabelMalformed means the shadow call returned a response that could not
	// be normalized or was internally inconsistent.
	LabelMalformed Label = "shadow_malformed"
	// LabelInvalidChoice means the shadow answer named a choice that is not in
	// the question's allowed set.
	LabelInvalidChoice Label = "shadow_invalid_choice"
	// LabelIndeterminate means the shadow answer could not be classified into a
	// definite outcome.
	LabelIndeterminate Label = "shadow_indeterminate"
	// LabelSkippedNotComparable means the case was gated out of a comparison
	// source because it was not semantically comparable there.
	LabelSkippedNotComparable Label = "skipped_not_comparable"
)

// Reference is a reference outcome for a single case, used to compute
// agreement. It is supplied adapter-side (deterministic expectation or recorded
// SOP outcome). A reference is keyed by question ID.
type Reference struct {
	// QuestionID identifies the question this reference answers.
	QuestionID string "json:\"question_id\""
	// Choice is the reference choice for a CHOICE question.
	Choice string "json:\"choice\""
}

// Case is one primary decision under observation, together with the reference
// outcomes and comparison sources that apply to it.
type Case struct {
	// ID identifies the case. It keys disagreement recordings.
	ID string
	// Question is the primary question the comparison is anchored on.
	Question decision.Question
	// Reference is the reference outcome, when available. A nil reference means
	// no deterministic or recorded outcome was supplied for this case.
	Reference *Reference
}

// Source evaluates a single case in shadow mode. Implementations run entirely
// off the primary decision path. A source must not mutate primary state.
type Source interface {
	// Name is the stable source identifier (for example "clef", "nimble").
	Name() string
	// Comparable reports whether the source considers the case semantically
	// comparable. Non-comparable cases are excluded from this source's metrics.
	Comparable(c Case) bool
	// Observe obtains the source's outcome for the case. The primary decision
	// result must never depend on this call. Any error is contained by the
	// harness.
	Observe(ctx context.Context, c Case) (Observation, error)
}

// Observation is a source's normalized shadow outcome for one case.
// A zero-valued Observation with empty Choice is treated as indeterminate by
// the harness.
type Observation struct {
	// Choice is the observed choice for a CHOICE question.
	Choice string
	// Confidence is the observed confidence in [0,1], when supplied.
	Confidence *float64
}

// Recorder accumulates per-case and aggregate shadow metrics. It holds no
// reference to any SOP policy writer and never feeds output back into policy.
type Recorder struct {
	sources []Source

	// per-case recordings
	records []CaseRecord

	// aggregate counters
	agreements    int
	disagreements int
	invalid       int
	indeterminate int
	timeouts      int
	failures      int
	malformed     int

	latencies  []time.Duration
	confidence []float64

	// nimble-specific counters (comparability-gated)
	nimbleTotal    int
	nimbleAgree    int
	nimbleDisagree int
}

// CaseRecord is the per-case recording produced by shadow evaluation.
// It is observational only and is never returned to the primary caller.
type CaseRecord struct {
	CaseID     string
	Source     string
	Label      Label
	Primary    string
	Shadow     string
	Reference  string
	Latency    time.Duration
	Confidence *float64
}

// NewRecorder builds a recorder over the supplied comparison sources. Sources
// are evaluated in isolation; a nil source list yields a recorder that only
// records the primary-vs-reference comparison.
func NewRecorder(sources ...Source) *Recorder {
	return &Recorder{sources: sources}
}

// Evaluate runs shadow evaluation for a single case and records the outcome.
// It returns nothing that can influence the primary result: the sole effect is
// accumulating recordings. A panic-free error from a source is contained.
func (r *Recorder) Evaluate(ctx context.Context, c Case, primary string) {
	if c.Reference != nil {
		r.recordReference(c, primary)
	}

	for _, src := range r.sources {
		r.evaluateSource(ctx, src, c, primary)
	}
}

// recordReference records the primary-vs-reference comparison as an
// informational recording. It does NOT contribute to AgreementRate, which is
// defined over shadow (source) comparisons against the reference. This keeps
// the primary decision itself out of the shadow agreement metric.
func (r *Recorder) recordReference(c Case, primary string) {
	ref := c.Reference
	if primary == "" || ref == nil || ref.Choice == "" {
		r.records = append(r.records, CaseRecord{
			CaseID: c.ID, Source: "reference", Label: LabelIndeterminate,
			Primary: primary, Reference: ref.Choice,
		})
		return
	}
	if primary == ref.Choice {
		r.records = append(r.records, CaseRecord{
			CaseID: c.ID, Source: "reference", Label: LabelAgreement,
			Primary: primary, Reference: ref.Choice,
		})
		return
	}
	r.records = append(r.records, CaseRecord{
		CaseID: c.ID, Source: "reference", Label: LabelDisagreement,
		Primary: primary, Reference: ref.Choice,
	})
}

func (r *Recorder) evaluateSource(ctx context.Context, src Source, c Case, primary string) {
	if !src.Comparable(c) {
		r.records = append(r.records, CaseRecord{CaseID: c.ID, Source: src.Name(), Label: LabelSkippedNotComparable, Primary: primary})
		return
	}

	start := time.Now()
	obs, err := observeSafely(ctx, src, c)
	elapsed := time.Since(start)
	r.recordOutcome(c, src.Name(), primary, obs, elapsed, err)
}

// RecordSupplied records a source outcome that was obtained off the shadow path,
// with an explicit latency, reusing the harness classification, counters, and
// containment. It performs no Observe call and no model call: it exists so an
// offline producer can replay a recorded observation set into deterministic
// evidence. When comparable is false the case is gated out of the source exactly
// as evaluateSource gates it. The primary decision result is never affected.
func (r *Recorder) RecordSupplied(c Case, srcName, primary string, comparable bool, obs Observation, latency time.Duration, err error) {
	if !comparable {
		r.records = append(r.records, CaseRecord{CaseID: c.ID, Source: srcName, Label: LabelSkippedNotComparable, Primary: primary})
		return
	}
	r.recordOutcome(c, srcName, primary, obs, latency, err)
}

// recordOutcome classifies one obtained source outcome and records it. It is the
// single classification path shared by evaluateSource (live observation) and
// RecordSupplied (offline replay), so the two never diverge.
func (r *Recorder) recordOutcome(c Case, srcName, primary string, obs Observation, elapsed time.Duration, err error) {
	r.latencies = append(r.latencies, elapsed)

	rec := CaseRecord{CaseID: c.ID, Source: srcName, Primary: primary, Latency: elapsed}

	if err != nil {
		label := classifyError(err)
		rec.Label = label
		switch label {
		case LabelTimeout:
			// A timeout is counted once, as a timeout. It is also counted in
			// the timeout/failure numerator via r.timeouts, so it must not be
			// added to r.failures as well.
			r.timeouts++
		case LabelMalformed:
			r.malformed++
			r.failures++
		default:
			r.failures++
		}
		r.records = append(r.records, rec)
		return
	}

	// Malformed: confidence out of range is internally inconsistent.
	if obs.Confidence != nil && (*obs.Confidence < 0 || *obs.Confidence > 1) {
		rec.Label = LabelMalformed
		r.malformed++
		r.failures++
		r.records = append(r.records, rec)
		return
	}

	if obs.Confidence != nil {
		r.confidence = append(r.confidence, *obs.Confidence)
		rec.Confidence = obs.Confidence
	}

	if c.Question.Type == decision.QuestionChoice && obs.Choice != "" && !c.Question.Allows(obs.Choice) {
		rec.Label = LabelInvalidChoice
		r.invalid++
		rec.Shadow = obs.Choice
		r.records = append(r.records, rec)
		return
	}

	if obs.Choice == "" {
		rec.Label = LabelIndeterminate
		r.indeterminate++
		r.records = append(r.records, rec)
		return
	}

	rec.Shadow = obs.Choice

	if srcName == "nimble" {
		r.nimbleTotal++
	}

	if c.Reference == nil || c.Reference.Choice == "" {
		rec.Label = LabelAgreement
		r.records = append(r.records, rec)
		return
	}
	rec.Reference = c.Reference.Choice
	if obs.Choice == c.Reference.Choice {
		rec.Label = LabelAgreement
		r.agreements++
		if srcName == "nimble" {
			r.nimbleAgree++
		}
	} else {
		rec.Label = LabelDisagreement
		r.disagreements++
		if srcName == "nimble" {
			r.nimbleDisagree++
		}
	}
	r.records = append(r.records, rec)
}

// observeSafely invokes a source with panic containment so a misbehaving shadow
// source can never crash the primary path.
func observeSafely(ctx context.Context, src Source, c Case) (obs Observation, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			obs = Observation{}
			err = fmt.Errorf("shadow source %s panicked: %v", src.Name(), rec)
		}
	}()
	return src.Observe(ctx, c)
}

// classifyError maps an error into a shadow containment label. Deadline
// exceeded and context cancellation become timeouts; malformed-response
// sentinels become malformed; everything else is an error.
func classifyError(err error) Label {
	if err == nil {
		return LabelAgreement
	}
	if err == context.DeadlineExceeded || isDeadline(err) {
		return LabelTimeout
	}
	if isMalformed(err) {
		return LabelMalformed
	}
	if err == context.Canceled {
		return LabelTimeout
	}
	return LabelError
}

func isDeadline(err error) bool {
	type timeout interface{ Timeout() bool }
	if t, ok := err.(timeout); ok && t.Timeout() {
		return true
	}
	return false
}

func isMalformed(err error) bool {
	type malformed interface{ Malformed() bool }
	type unwrapper interface{ Unwrap() error }
	for {
		if err == decision.ErrMalformedResponse {
			return true
		}
		if m, ok := err.(malformed); ok && m.Malformed() {
			return true
		}
		uw, ok := err.(unwrapper)
		if !ok {
			break
		}
		next := uw.Unwrap()
		if next == nil || next == err {
			break
		}
		err = next
	}
	return err == decision.ErrMalformedResponse
}

// observations returns the number of shadow source observations recorded. It is
// the denominator for the invalid-choice, indeterminate, and timeout/failure
// rates, which describe shadow source behavior rather than primary cases.
func (r *Recorder) observations() int { return len(r.latencies) }

// AgreementRate returns shadow agreements / (shadow agreements + shadow
// disagreements). Reference (primary-vs-expected) recordings are informational
// and are not included; the rate measures shadow source choice versus reference.
func (r *Recorder) AgreementRate() float64 {
	base := r.agreements + r.disagreements
	if base == 0 {
		return 0
	}
	return float64(r.agreements) / float64(base)
}

// Disagreements returns the recorded shadow disagreement cases (copy). The
// informational reference recording is excluded: this list is shadow choice vs
// reference only.
func (r *Recorder) Disagreements() []CaseRecord {
	var out []CaseRecord
	for _, rec := range r.records {
		if rec.Label == LabelDisagreement && rec.Source != "reference" {
			out = append(out, rec)
		}
	}
	return out
}

// InvalidChoiceRate returns invalid / source observations.
func (r *Recorder) InvalidChoiceRate() float64 {
	if r.observations() == 0 {
		return 0
	}
	return float64(r.invalid) / float64(r.observations())
}

// IndeterminateRate returns indeterminate / source observations.
func (r *Recorder) IndeterminateRate() float64 {
	if r.observations() == 0 {
		return 0
	}
	return float64(r.indeterminate) / float64(r.observations())
}

// TimeoutFailureRate returns (timeouts + failures) / source observations.
// A timeout is counted once (as a timeout); non-timeout provider errors and
// malformed responses are counted as failures.
func (r *Recorder) TimeoutFailureRate() float64 {
	obs := r.observations()
	if obs == 0 {
		return 0
	}
	return float64(r.timeouts+r.failures) / float64(obs)
}

// LatencyP50 returns the nearest-rank 50th percentile latency.
func (r *Recorder) LatencyP50() time.Duration { return r.LatencyPercentile(0.50) }

// LatencyP95 returns the nearest-rank 95th percentile latency.
func (r *Recorder) LatencyP95() time.Duration { return r.LatencyPercentile(0.95) }

// LatencyPercentile returns the nearest-rank percentile of recorded latencies.
func (r *Recorder) LatencyPercentile(p float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}
	sortedL := append([]time.Duration(nil), r.latencies...)
	sort.Slice(sortedL, func(i, j int) bool { return sortedL[i] < sortedL[j] })
	return sortedL[nearestRankIndex(len(sortedL), p)]
}

// ConfidenceDistribution buckets recorded confidences into fixed [0,1]
// buckets 0.0-0.2 ... 0.8-1.0 (five buckets) plus a "none" count for cases
// where confidence was absent.
func (r *Recorder) ConfidenceDistribution() map[string]int {
	dist := map[string]int{
		"0.0-0.2": 0,
		"0.2-0.4": 0,
		"0.4-0.6": 0,
		"0.6-0.8": 0,
		"0.8-1.0": 0,
		"none":    0,
	}
	for _, c := range r.confidence {
		dist[bucketKey(c)]++
	}
	return dist
}

func bucketKey(c float64) string {
	switch {
	case c < 0.2:
		return "0.0-0.2"
	case c < 0.4:
		return "0.2-0.4"
	case c < 0.6:
		return "0.4-0.6"
	case c < 0.8:
		return "0.6-0.8"
	default:
		return "0.8-1.0"
	}
}

// NimbleAgreement returns Nimble agreements / comparable Nimble cases.
func (r *Recorder) NimbleAgreement() float64 {
	if r.nimbleTotal == 0 {
		return 0
	}
	return float64(r.nimbleAgree) / float64(r.nimbleTotal)
}

// NimbleComparableCases returns the number of comparability-gated Nimble cases.
func (r *Recorder) NimbleComparableCases() int { return r.nimbleTotal }

// Records returns a copy of all per-case recordings.
func (r *Recorder) Records() []CaseRecord {
	return append([]CaseRecord(nil), r.records...)
}

// Total returns the number of shadow source observations recorded.
func (r *Recorder) Total() int { return r.observations() }

// nearestRankIndex returns the zero-based sorted index for percentile p
// (0<p<=1) using the nearest-rank method.
func nearestRankIndex(n int, p float64) int {
	if n <= 0 {
		return 0
	}
	rk := int(p*float64(n) + 0.999999)
	if rk < 1 {
		rk = 1
	}
	if rk > n {
		rk = n
	}
	return rk - 1
}
