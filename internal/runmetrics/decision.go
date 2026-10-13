package runmetrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Decision telemetry (TEL-002–TEL-005) is the diagnostic aggregation of the
// bounded decisions SOP records during a run: the model tier it selected and the
// layer that selected it (override/router/fallback), the recorded overrides and
// escalations, and the JEV decision-layer invocation count. It extends the
// existing run-metrics aggregate (TEL-006) rather than introducing a second
// collector, and it reads only recorded artifacts through the existing reader
// shapes.
//
// It is diagnostic only: like the RM-002 dimensions, nothing it emits drives a
// gate, a budget, or a routing decision, and a value SOP did not record is
// UNAVAILABLE — never inferred, never estimated from text, byte, or diff size.
//
// The two decision-adapter dimensions (adapter agreement and decision
// confidence) have no recorded source in an agentic-sop run artifact: shadow
// evaluation is a documented mode with no runtime implementation, and a routing
// decision's confidence is not persisted. Both are therefore emitted UNAVAILABLE
// rather than fabricated, matching the honest-coverage rule of RM-001 §5.1.

// Decision metric dimensions. The names are stable identifiers; they are emitted
// after the seven RM-002 metrics so the RM-002 contract and its order are not
// disturbed.
const (
	// MetricModelTier is the model-tier (picker) distribution: the recorded
	// small/medium/large selections with their selecting layer and fallback flag.
	MetricModelTier Metric = "model_tier_distribution"
	// MetricOverride is the recorded manual override rate (--model-class / CLI).
	MetricOverride Metric = "decision_override_rate"
	// MetricEscalation is the recorded escalation rate (recovery class escalation
	// or a NEEDS_HUMAN terminal decision).
	MetricEscalation Metric = "decision_escalation_rate"
	// MetricJEVInvoke is the recorded JEV decision-layer invocation count.
	MetricJEVInvoke Metric = "jev_invocation"

	// Adapter telemetry (TEL-003), sourced from shadow-evidence.json.

	// MetricAdapterInvocation is the count of recorded shadow observations.
	MetricAdapterInvocation Metric = "adapter_invocation"
	// MetricAdapter is decision-adapter agreement with the reference outcome
	// (deterministic expectation or recorded SOP outcome).
	MetricAdapter Metric = "adapter_agreement"
	// MetricAdapterConfidence is the recorded adapter confidence coverage.
	MetricAdapterConfidence Metric = "adapter_confidence"
	// MetricAdapterLatency is the recorded adapter observation latency (p50/p95).
	MetricAdapterLatency Metric = "adapter_latency"
	// MetricAdapterInvalidChoice is the recorded invalid-choice rate.
	MetricAdapterInvalidChoice Metric = "adapter_invalid_choice"
	// MetricAdapterIndeterminate is the recorded indeterminate rate.
	MetricAdapterIndeterminate Metric = "adapter_indeterminate"
	// MetricAdapterTimeoutFailure is the recorded timeout/failure rate.
	MetricAdapterTimeoutFailure Metric = "adapter_timeout_failure"

	// MetricConfidence is the recorded routing decision confidence.
	MetricConfidence Metric = "decision_confidence"
)

// decisionMetricOrder fixes the emission order of the decision dimensions, so
// output is deterministic.
var decisionMetricOrder = []Metric{
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

// aggregateDecisions folds the per-run evidence into the decision-telemetry
// metrics, in fixed order.
func aggregateDecisions(ev []runEvidence, corpus int) []MetricResult {
	out := make([]MetricResult, 0, len(decisionMetricOrder))
	for _, m := range decisionMetricOrder {
		out = append(out, decisionMetricFor(m, ev, corpus))
	}
	return out
}

// decisionMetricFor emits one decision metric with the same five measurement
// quantities and numerator/denominator/coverage contract the RM-002 metrics use.
func decisionMetricFor(m Metric, ev []runEvidence, corpus int) MetricResult {
	switch m {
	case MetricModelTier:
		// Source: model-selection.json. Present when the file exists; valid when it
		// parsed and recorded a selection; the tier/source/fallback breakdown is
		// computed over the valid runs only. A run without model-selection.json is
		// UNAVAILABLE, never a "default" tier.
		present := count(ev, func(e runEvidence) bool { return e.hasModelSel })
		valid := count(ev, func(e runEvidence) bool { return e.modelSelReadable && e.routed })
		r := countMetric(m, StatusPartial, corpus, present, valid, valid, valid, "runs",
			[]string{modelSelectionFileName},
			"numerator counts runs with a recorded model selection; the breakdown is computed over those runs only. Routing is opt-in, so a run without model-selection.json is UNAVAILABLE, never a default tier")
		r.Breakdown = tierBreakdown(ev)
		return r

	case MetricOverride:
		// Source: model-selection.json source=="cli" (a --model-class override).
		// Present when the file exists; valid when it parsed and recorded a source.
		present := count(ev, func(e runEvidence) bool { return e.hasModelSel })
		valid := count(ev, func(e runEvidence) bool { return e.modelSelReadable && e.selSource != "" })
		num := count(ev, func(e runEvidence) bool { return e.modelSelReadable && e.selSource == sourceCLI })
		return rate(m, StatusPartial, corpus, present, valid, num, valid,
			"numerator counts runs whose model-selection.json records source \"cli\" (an explicit --model-class override); a run without a recorded source is UNAVAILABLE")

	case MetricEscalation:
		// Sources: routing.json source=="escalation" (a recovery class escalation)
		// or report.json decision==NEEDS_HUMAN (a human escalation). Present when
		// either artifact exists; valid when a routing source or a terminal decision
		// was recorded. Numerator counts a recorded escalation of either kind.
		present := count(ev, func(e runEvidence) bool { return e.hasRouting || e.hasReport })
		valid := count(ev, func(e runEvidence) bool {
			return (e.routingReadable && e.routingSource != "") || e.reportDecided
		})
		num := count(ev, func(e runEvidence) bool {
			return (e.routingReadable && e.routingSource == sourceEscalation) ||
				(e.reportDecided && e.approvalRequested)
		})
		return rate(m, StatusPartial, corpus, present, valid, num, valid,
			"numerator counts a recorded recovery escalation (routing.json source \"escalation\") or a NEEDS_HUMAN terminal decision; absence is UNAVAILABLE, never a recorded zero escalation")

	case MetricJEVInvoke:
		// Source: metrics.json JEV.Invocations. Present when metrics.json exists;
		// valid when it parsed; numerator sums the recorded invocation count. A
		// present metrics.json with no JEV section is a recorded zero invocation.
		present := count(ev, func(e runEvidence) bool { return e.hasMetrics })
		valid := count(ev, func(e runEvidence) bool { return e.metricsParsed })
		num := 0
		for _, e := range ev {
			if e.metricsParsed && e.metricsData.JEV != nil {
				num += e.metricsData.JEV.Invocations
			}
		}
		return countMetric(m, StatusAvailable, corpus, present, valid, num, valid, "calls",
			[]string{metricsFileName},
			"numerator sums the recorded JEV.Invocations over runs whose metrics.json parsed; a present metrics.json with no JEV section is a recorded zero, an absent metrics.json is UNAVAILABLE")

	case MetricAdapterInvocation:
		// Source: shadow-evidence.json. Present when the file exists; valid when it
		// parsed at the known schema version. The numerator is the recorded observation
		// count across valid runs; a run without evidence contributes no observation
		// (UNAVAILABLE), never a zero.
		present := count(ev, func(e runEvidence) bool { return e.hasShadow })
		valid := count(ev, func(e runEvidence) bool { return e.shadowReadable })
		obs := shadowObservations(ev)
		return countMetric(m, StatusPartial, corpus, present, valid, len(obs), len(obs), "observations",
			[]string{shadowEvidenceFileName},
			"numerator counts recorded shadow observations across runs whose shadow-evidence.json parsed; a run without evidence is UNAVAILABLE and contributes no observation")

	case MetricAdapter:
		// Agreement: the shadow choice equals the reference (deterministic expectation or
		// recorded SOP outcome). Only comparable observations that recorded a reference
		// form the denominator, exactly as the adapter harness defines AgreementRate; a
		// recorded no-reference agreement is not counted. Agreement is evidence, not
		// proof of correctness.
		present := count(ev, func(e runEvidence) bool { return e.hasShadow })
		valid := count(ev, func(e runEvidence) bool { return e.shadowReadable })
		num, den := 0, 0
		bySource := map[string][2]int{}
		for _, o := range comparableObservations(ev) {
			switch {
			case o.Outcome == outcomeAgreement && o.Reference != "":
				num++
				den++
				inc(bySource, o.Source, 0)
			case o.Outcome == outcomeDisagreement:
				den++
				inc(bySource, o.Source, 1)
			}
		}
		r := rate(m, StatusPartial, corpus, present, valid, num, den,
			"numerator counts comparable shadow observations whose choice equals the recorded reference over those that recorded one; a no-reference record is not an agreement")
		r.Breakdown = sourceAgreementBreakdown(bySource)
		return r

	case MetricAdapterConfidence:
		// Confidence coverage: comparable observations that recorded a confidence, over
		// all comparable observations. A recorded confidence is bucketed; a missing one
		// is not a zero confidence.
		present := count(ev, func(e runEvidence) bool { return e.hasShadow })
		valid := count(ev, func(e runEvidence) bool { return e.shadowReadable })
		obs := comparableObservations(ev)
		num := 0
		buckets := map[string]int{}
		for _, o := range obs {
			if o.Confidence != nil {
				num++
				buckets[confidenceBucket(*o.Confidence)]++
			}
		}
		r := rate(m, StatusPartial, corpus, present, valid, num, len(obs),
			"numerator counts comparable observations that recorded a confidence; a missing confidence is not a zero. Buckets are fixed [0,1] ranges")
		r.Breakdown = confidenceBreakdown(buckets)
		return r

	case MetricAdapterLatency:
		// Latency: nearest-rank p50/p95 over comparable observations' recorded latency.
		// The numerator is the summed latency over the denominator (the comparable
		// observations); the value reports p50 and the breakdown reports p50 and p95.
		present := count(ev, func(e runEvidence) bool { return e.hasShadow })
		valid := count(ev, func(e runEvidence) bool { return e.shadowReadable })
		lat := make([]int, 0, len(ev))
		for _, o := range comparableObservations(ev) {
			lat = append(lat, int(o.LatencyMS))
		}
		r := base(m, StatusPartial, corpus, present, valid, len(lat), len(lat), "ms",
			"denominator is the comparable observations; the value is the nearest-rank p50 and the breakdown reports p50/p95. Latency is diagnostic only")
		if len(lat) > 0 {
			sort.Ints(lat)
			p50 := lat[nearestRankIndex(len(lat), 0.50)]
			p95 := lat[nearestRankIndex(len(lat), 0.95)]
			r.Value = strconv.Itoa(p50)
			r.Breakdown = []BreakdownEntry{
				{Group: "latency", Key: "p50_ms", Count: p50},
				{Group: "latency", Key: "p95_ms", Count: p95},
			}
		}
		return r

	case MetricAdapterInvalidChoice:
		return adapterOutcomeRate(m, ev, corpus, "invalid-choice rate: comparable observations whose choice is not in the question's allowed set",
			func(o shadowCase) bool { return o.Outcome == outcomeInvalidChoice })

	case MetricAdapterIndeterminate:
		return adapterOutcomeRate(m, ev, corpus, "indeterminate rate: comparable observations classified indeterminate (an empty-choice observation)",
			func(o shadowCase) bool { return o.Outcome == outcomeIndeterminate })

	case MetricAdapterTimeoutFailure:
		return adapterOutcomeRate(m, ev, corpus, "timeout/failure rate: comparable observations ending in a timeout, provider error, or malformed response",
			func(o shadowCase) bool {
				switch o.Outcome {
				case outcomeTimeout, outcomeError, outcomeMalformed:
					return true
				default:
					return false
				}
			})

	case MetricConfidence:
		// No recorded source: a routing decision's confidence is not persisted (the
		// routing artifact records the class, source, reasons, and typed signals,
		// not a confidence). UNAVAILABLE for every run.
		return MetricResult{
			Metric:                  MetricConfidence,
			Status:                  StatusUnavailable,
			Value:                   UNAVAILABLE,
			TotalRuns:               corpus,
			UnavailableMeasurements: corpus,
			Unit:                    "confidence",
			Note:                    "a routing decision's confidence is not persisted in any run artifact; the value is UNAVAILABLE and is never inferred from a signal or a class",
		}
	}
	// Unreachable: decisionMetricOrder covers every decision Metric constant.
	return MetricResult{Metric: m, Status: StatusUnavailable, Value: UNAVAILABLE, TotalRuns: corpus}
}

// tierBreakdown builds the deterministic breakdown of the recorded model
// selections: by tier (small/medium/large), by selecting source, and the count of
// fallback selections. Only known tiers and recorded sources contribute; an
// unknown tier or an empty source is excluded rather than bucketed as other.
func tierBreakdown(ev []runEvidence) []BreakdownEntry {
	tierCounts := map[string]int{}
	sourceCounts := map[string]int{}
	fallback := 0
	for _, e := range ev {
		if !e.modelSelReadable || !e.routed {
			continue
		}
		if validTier(e.selClass) {
			tierCounts[e.selClass]++
		}
		if e.selSource != "" {
			sourceCounts[e.selSource]++
		}
		if e.selFallback {
			fallback++
		}
	}

	out := make([]BreakdownEntry, 0, len(tierCounts)+len(sourceCounts)+1)
	out = append(out, breakdownEntries("tier", tierCounts, tierOrder)...)
	out = append(out, breakdownEntries("source", sourceCounts, nil)...)
	if fallback > 0 {
		out = append(out, BreakdownEntry{Group: "fallback", Key: "applied", Count: fallback})
	}
	return out
}

// breakdownEntries renders a labeled, deterministic set of group counts. When
// order is non-empty it fixes the order (used for the known tiers); otherwise the
// keys are sorted so output is stable.
func breakdownEntries(group string, counts map[string]int, order []string) []BreakdownEntry {
	keys := make([]string, 0, len(counts))
	if len(order) > 0 {
		for _, k := range order {
			if _, ok := counts[k]; ok {
				keys = append(keys, k)
			}
		}
	} else {
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	out := make([]BreakdownEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, BreakdownEntry{Group: group, Key: k, Count: counts[k]})
	}
	return out
}

// tierOrder fixes the tier breakdown order to the model-class order.
var tierOrder = []string{"small", "medium", "large"}

// validTier reports whether s is a known model class recorded in a selection.
func validTier(s string) bool {
	switch s {
	case "small", "medium", "large":
		return true
	default:
		return false
	}
}

// sourceCLI and sourceEscalation mirror the closed routing/model source values the
// artifacts record, so the aggregator matches them by value rather than importing
// a provider- or model-specific package.
const (
	sourceCLI        = "cli"
	sourceEscalation = "escalation"
)

// RenderText renders a deterministic, human-readable summary of an aggregate: the
// seven RM-002 dimensions followed by the decision-telemetry dimensions, each
// with its value, coverage, numerator/denominator, and any breakdown. It performs
// no I/O and no model call, and two aggregates over identical inputs render
// byte-identical text.
func RenderText(agg Aggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run metrics (corpus %d runs)\n\n", agg.CorpusRuns)
	for _, r := range agg.Metrics {
		renderMetric(&b, r)
	}
	b.WriteString("\nDecision telemetry\n\n")
	for _, r := range agg.Decisions {
		renderMetric(&b, r)
	}
	return b.String()
}

// RenderDecisions renders only the decision-telemetry dimensions of an aggregate:
// the model-tier/override/escalation/JEV dimensions followed by the adapter
// dimensions, each with its value, coverage, numerator/denominator, breakdown, and
// UNAVAILABLE markers. It is the human-readable body of `sop report --decisions`
// and is deterministic: two aggregates over identical inputs render identically.
func RenderDecisions(agg Aggregate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Decision telemetry (corpus %d runs)\n\n", agg.CorpusRuns)
	for _, r := range agg.Decisions {
		renderMetric(&b, r)
	}
	return b.String()
}

// renderMetric writes one metric's summary line and breakdown.
func renderMetric(b *strings.Builder, r MetricResult) {
	if r.Value == UNAVAILABLE {
		fmt.Fprintf(b, "%s: %s (coverage %.1f%%)\n", r.Metric, UNAVAILABLE, r.CoveragePct)
	} else {
		fmt.Fprintf(b, "%s: %s %s (coverage %.1f%%, %d/%d)\n",
			r.Metric, r.Value, strings.TrimSpace(r.Unit), r.CoveragePct, r.Numerator, r.Denominator)
	}
	for _, e := range r.Breakdown {
		fmt.Fprintf(b, "  %s %s: %d\n", e.Group, e.Key, e.Count)
	}
}

// Adapter outcome labels. They mirror internal/shadow in sop-decision-adapters; the
// aggregator matches them by value rather than importing a provider package.
const (
	outcomeAgreement     = "shadow_agreement"
	outcomeDisagreement  = "shadow_disagreement"
	outcomeTimeout       = "shadow_timeout"
	outcomeError         = "shadow_error"
	outcomeMalformed     = "shadow_malformed"
	outcomeInvalidChoice = "shadow_invalid_choice"
	outcomeIndeterminate = "shadow_indeterminate"
)

// shadowObservations flattens the recorded shadow observations across runs whose
// evidence parsed at the known schema version.
func shadowObservations(ev []runEvidence) []shadowCase {
	var out []shadowCase
	for _, e := range ev {
		if e.shadowReadable {
			out = append(out, e.shadowCases...)
		}
	}
	return out
}

// comparableObservations are the observations a source actually evaluated. A
// non-comparable record is excluded, matching the adapter harness denominator.
func comparableObservations(ev []runEvidence) []shadowCase {
	obs := shadowObservations(ev)
	out := make([]shadowCase, 0, len(obs))
	for _, o := range obs {
		if o.Comparable {
			out = append(out, o)
		}
	}
	return out
}

// adapterOutcomeRate emits an adapter rate metric: the numerator counts comparable
// observations matching match; the denominator is the comparable observations.
func adapterOutcomeRate(m Metric, ev []runEvidence, corpus int, note string, match func(shadowCase) bool) MetricResult {
	present := count(ev, func(e runEvidence) bool { return e.hasShadow })
	valid := count(ev, func(e runEvidence) bool { return e.shadowReadable })
	obs := comparableObservations(ev)
	num := 0
	for _, o := range obs {
		if match(o) {
			num++
		}
	}
	return rate(m, StatusPartial, corpus, present, valid, num, len(obs), note)
}

func inc(m map[string][2]int, key string, i int) {
	v := m[key]
	v[i]++
	m[key] = v
}

// sourceAgreementBreakdown renders agreement/disagreement counts per source in a
// deterministic order.
func sourceAgreementBreakdown(bySource map[string][2]int) []BreakdownEntry {
	keys := make([]string, 0, len(bySource))
	for k := range bySource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []BreakdownEntry
	for _, k := range keys {
		out = append(out,
			BreakdownEntry{Group: "source:" + k, Key: "agreement", Count: bySource[k][0]},
			BreakdownEntry{Group: "source:" + k, Key: "disagreement", Count: bySource[k][1]},
		)
	}
	return out
}

func confidenceBucket(c float64) string {
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

// confidenceBreakdown renders the fixed confidence buckets in a deterministic
// order, initializing all five so an empty bucket is an explicit zero.
func confidenceBreakdown(buckets map[string]int) []BreakdownEntry {
	order := []string{"0.0-0.2", "0.2-0.4", "0.4-0.6", "0.6-0.8", "0.8-1.0"}
	out := make([]BreakdownEntry, 0, len(order))
	for _, k := range order {
		out = append(out, BreakdownEntry{Group: "confidence", Key: k, Count: buckets[k]})
	}
	return out
}

// nearestRankIndex returns the zero-based sorted index for percentile p using the
// nearest-rank method (the adapter harness's definition).
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
