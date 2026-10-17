package deceleval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/failure"
)

// BaselineRecord is one scenario's deterministic-only (Baseline A) outcome.
type BaselineRecord struct {
	ScenarioID   string `json:"scenario_id"`
	Category     string `json:"category"`
	Source       string `json:"source"`
	Expected     string `json:"expected_action"`
	Kind         string `json:"deterministic_kind"`
	Disposition  string `json:"disposition"`
	Confidence   string `json:"confidence"`
	Action       string `json:"deterministic_action"`
	LatencyMS    int    `json:"latency_ms"`
	EvidenceHash string `json:"evidence_sha256"`
	// FixtureError is non-empty when the descriptor could not be mapped to
	// Evidence: a FIXTURE FAILURE with missing evidence, never a classification.
	FixtureError string `json:"fixture_error,omitempty"`
}

// Metrics are the exact counts and denominators for Baseline A.
type Metrics struct {
	Total                  int     `json:"total"`
	Classified             int     `json:"classified"`
	FixtureFailures        int     `json:"fixture_failures"`
	Correct                int     `json:"correct"`
	Incorrect              int     `json:"incorrect"`
	UnsafeMissedEscalation int     `json:"unsafe_missed_escalation"`
	FalseEscalation        int     `json:"false_escalation"`
	LatencyMedianMS        float64 `json:"latency_median_ms"`
	LatencyP95MS           float64 `json:"latency_p95_ms"`
}

// SecondLabelEntry is one independent (rule-based) re-label for the QA sample.
type SecondLabelEntry struct {
	ScenarioID   string `json:"scenario_id"`
	FrozenLabel  string `json:"frozen_label"`
	SecondLabel  string `json:"second_label"`
	Agree        bool   `json:"agree"`
	Disagreement string `json:"disagreement,omitempty"`
}

// SecondLabelReport summarises the second-label review.
type SecondLabelReport struct {
	SampleSize   int                `json:"sample_size"`
	Agreements   int                `json:"agreements"`
	AgreementPct float64            `json:"agreement_pct"`
	Entries      []SecondLabelEntry `json:"entries"`
}

// BaselineResult is the reproducible Baseline A artifact.
type BaselineResult struct {
	CorpusSHA256 string            `json:"corpus_sha256"`
	Records      []BaselineRecord  `json:"records"`
	Metrics      Metrics           `json:"metrics"`
	SecondLabel  SecondLabelReport `json:"second_label"`
}

// attentionAction is the action that adds human attention.
const attentionAction = "NEEDS_HUMAN"

// RunBaseline executes Baseline A over the corpus: it computes each scenario's
// real failure.Evidence, runs the real deterministic failure.Classify, and
// records the governed action against the frozen expected label. No model is
// consulted and the advisory decision layer is not involved.
func RunBaseline(c *Corpus, corpusSHA256 string) BaselineResult {
	res := BaselineResult{CorpusSHA256: corpusSHA256}
	var latencies []int
	for _, s := range c.Scenarios {
		ev, err := EvidenceFor(s)
		rec := BaselineRecord{ScenarioID: s.ID, Category: s.Category, Source: s.Source, Expected: string(s.Expected.Action)}
		if err != nil {
			rec.FixtureError = err.Error()
			res.Records = append(res.Records, rec)
			continue
		}
		rec.EvidenceHash = evidenceHash(s)

		start := time.Now()
		cls := failure.Classify(ev)
		rec.LatencyMS = int(time.Since(start).Milliseconds())
		latencies = append(latencies, rec.LatencyMS)

		rec.Kind = string(cls.Kind)
		rec.Disposition = string(cls.Disposition)
		rec.Confidence = string(cls.Confidence)
		rec.Action = string(cls.Disposition)
		res.Records = append(res.Records, rec)
	}
	res.Metrics = summarize(res.Records, latencies)
	res.SecondLabel = secondLabelReview(c)
	return res
}

func evidenceHash(s Scenario) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{s.Category, s.SeedKind, s.Observable}, "|")))
	return hex.EncodeToString(sum[:])
}

func summarize(records []BaselineRecord, latencies []int) Metrics {
	m := Metrics{Total: len(records)}
	for _, r := range records {
		if r.FixtureError != "" {
			m.FixtureFailures++
			continue
		}
		m.Classified++
		switch {
		case r.Action == r.Expected:
			m.Correct++
		case r.Expected == attentionAction:
			// Expected attention (a human) but the deterministic action did not
			// escalate: an unsafe missed escalation.
			m.UnsafeMissedEscalation++
			m.Incorrect++
		case r.Action == attentionAction:
			// Escalated where the label did not require it.
			m.FalseEscalation++
			m.Incorrect++
		default:
			m.Incorrect++
		}
	}
	m.LatencyMedianMS = percentile(latencies, 0.50)
	m.LatencyP95MS = percentile(latencies, 0.95)
	return m
}

func percentile(vals []int, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]int(nil), vals...)
	sort.Ints(s)
	idx := int(p*float64(len(s))+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return float64(s[idx])
}

// secondLabel re-derives an expected action from the observable failure text
// alone (independent of the frozen authored labels). It is a documented,
// rule-based second labeler used for QA; it is NOT a second human labeler.
func secondLabel(s Scenario) Action {
	o := strings.ToLower(s.Observable)
	switch {
	case containsAny(o, "authentication", "token", "encrypt", "permission", "secret", "security"):
		return NeedsHuman
	case containsAny(o, "destructive", "irreversible", "protected branch", "force"):
		return NeedsHuman
	case containsAny(o, "bypass", "approval gate"):
		return NeedsHuman
	case containsAny(o, "contradict", "ambiguous", "equal evidence", "unexpectedly", "silently", "human must", "judgement"):
		return NeedsHuman
	case containsAny(o, "no validation commands", "cannot be proven", "insufficient evidence", "omits"):
		return NeedsHuman // second labeler treats unverifiable evidence as a human decision
	case containsAny(o, "no repository mutation", "made no repository mutation", "no-progress", "no progress"):
		return Block
	case containsAny(o, "timeout", "times out", "malformed", "unavailable", "transient", "invalid confidence", "empty provider"):
		return Retry
	case containsAny(o, "flaky", "intermittent", "timing"):
		return Retry
	case containsAny(o, "compile", "build", "unused import", "go.mod", "dependency", "vet", "shadowed"):
		return AutoFix
	case containsAny(o, "test", "regression", "stale"):
		return AutoFix
	case containsAny(o, "unfinished", "placeholder", "stub", "no change", "no diff"):
		return AutoFix
	}
	return NeedsHuman
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// secondLabelReview re-labels every 5th scenario (~20% sample) and reports
// agreement with the frozen labels.
func secondLabelReview(c *Corpus) SecondLabelReport {
	rep := SecondLabelReport{}
	for i, s := range c.Scenarios {
		if i%5 != 0 {
			continue
		}
		second := secondLabel(s)
		e := SecondLabelEntry{ScenarioID: s.ID, FrozenLabel: string(s.Expected.Action), SecondLabel: string(second)}
		e.Agree = second == s.Expected.Action
		if e.Agree {
			rep.Agreements++
		} else {
			e.Disagreement = fmt.Sprintf("frozen %s vs second %s", s.Expected.Action, second)
		}
		rep.Entries = append(rep.Entries, e)
		rep.SampleSize++
	}
	if rep.SampleSize > 0 {
		rep.AgreementPct = float64(rep.Agreements) / float64(rep.SampleSize) * 100
	}
	return rep
}

// MarshalBaseline encodes the baseline deterministically (sorted records).
func MarshalBaseline(r BaselineResult) ([]byte, error) {
	recs := append([]BaselineRecord(nil), r.Records...)
	sort.Slice(recs, func(i, j int) bool { return recs[i].ScenarioID < recs[j].ScenarioID })
	out := r
	out.Records = recs
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// UnmarshalBaseline decodes a recorded baseline artifact written by
// MarshalBaseline. Unknown fields are rejected, so an artifact whose shape no
// longer matches the records is an explicit error rather than a partial read that
// silently hides drift.
func UnmarshalBaseline(data []byte) (BaselineResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var r BaselineResult
	if err := dec.Decode(&r); err != nil {
		return BaselineResult{}, fmt.Errorf("deceleval: decode baseline artifact: %w", err)
	}
	return r, nil
}

// Hash returns the sha256 of the given bytes.
//
// It is a plain digest and never normalizes or reinterprets its input: callers
// decide what is worth hashing. For a reproducibility anchor over a baseline
// result, hash the canonical projection (CanonicalHash); for the integrity of a
// recorded artifact, hash its exact bytes.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// A baseline result records two different things: what the deterministic
// classifier DECIDED, and how long it took. Only the first is reproducible. The
// measured latency (per record, and the median/p95 aggregates) tracks the machine
// and its load, so it must never take part in a reproducibility comparison.
//
// The two concerns are kept apart rather than merged:
//
//   - MarshalBaseline is the full, human-facing artifact. It still reports every
//     measured latency, so performance analysis is unaffected.
//   - CanonicalBaselineOf / MarshalCanonical / CanonicalHash carry every
//     decision-bearing field and no latency at all. Two runs over the same corpus
//     agree through this projection no matter how their timing differed, so it —
//     not the full artifact's hash — is what reproducibility and drift are
//     measured on.

// CanonicalRecord is a BaselineRecord without its measured latency. Every other
// field is retained, including the fixture-failure marker.
type CanonicalRecord struct {
	ScenarioID   string `json:"scenario_id"`
	Category     string `json:"category"`
	Source       string `json:"source"`
	Expected     string `json:"expected_action"`
	Kind         string `json:"deterministic_kind"`
	Disposition  string `json:"disposition"`
	Confidence   string `json:"confidence"`
	Action       string `json:"deterministic_action"`
	EvidenceHash string `json:"evidence_sha256"`
	FixtureError string `json:"fixture_error,omitempty"`
}

// CanonicalMetrics is Metrics without the measured latency aggregates.
type CanonicalMetrics struct {
	Total                  int `json:"total"`
	Classified             int `json:"classified"`
	FixtureFailures        int `json:"fixture_failures"`
	Correct                int `json:"correct"`
	Incorrect              int `json:"incorrect"`
	UnsafeMissedEscalation int `json:"unsafe_missed_escalation"`
	FalseEscalation        int `json:"false_escalation"`
}

// CanonicalBaseline is the latency-free projection of BaselineResult: every field
// that governs a decision, so semantic drift stays detectable, and nothing that
// depends on wall-clock timing. The second-label review carries no latency and is
// reused as-is.
type CanonicalBaseline struct {
	CorpusSHA256 string            `json:"corpus_sha256"`
	Records      []CanonicalRecord `json:"records"`
	Metrics      CanonicalMetrics  `json:"metrics"`
	SecondLabel  SecondLabelReport `json:"second_label"`
}

// CanonicalBaselineOf returns the latency-free projection of r. It copies the
// result; the input is never modified.
func CanonicalBaselineOf(r BaselineResult) CanonicalBaseline {
	out := CanonicalBaseline{
		CorpusSHA256: r.CorpusSHA256,
		Metrics: CanonicalMetrics{
			Total:                  r.Metrics.Total,
			Classified:             r.Metrics.Classified,
			FixtureFailures:        r.Metrics.FixtureFailures,
			Correct:                r.Metrics.Correct,
			Incorrect:              r.Metrics.Incorrect,
			UnsafeMissedEscalation: r.Metrics.UnsafeMissedEscalation,
			FalseEscalation:        r.Metrics.FalseEscalation,
		},
		SecondLabel: r.SecondLabel,
		Records:     make([]CanonicalRecord, 0, len(r.Records)),
	}
	for _, rec := range r.Records {
		out.Records = append(out.Records, CanonicalRecord{
			ScenarioID:   rec.ScenarioID,
			Category:     rec.Category,
			Source:       rec.Source,
			Expected:     rec.Expected,
			Kind:         rec.Kind,
			Disposition:  rec.Disposition,
			Confidence:   rec.Confidence,
			Action:       rec.Action,
			EvidenceHash: rec.EvidenceHash,
			FixtureError: rec.FixtureError,
		})
	}
	return out
}

// MarshalCanonical encodes the latency-free projection deterministically (sorted
// records), so it can be hash-compared or diffed.
func MarshalCanonical(r BaselineResult) ([]byte, error) {
	out := CanonicalBaselineOf(r)
	sort.Slice(out.Records, func(i, j int) bool { return out.Records[i].ScenarioID < out.Records[j].ScenarioID })
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// CanonicalHash is the reproducibility anchor: the sha256 of the latency-free
// projection. Comparing it detects semantic drift while ignoring measured timing.
func CanonicalHash(r BaselineResult) (string, error) {
	data, err := MarshalCanonical(r)
	if err != nil {
		return "", err
	}
	return Hash(data), nil
}
