package runmetrics

import (
	"fmt"
	"math"
	"sort"

	"github.com/imhttran/agentic-sop/internal/perf"
)

// AggregateProject reads the recorded run artifacts under projectDir's state
// directory and returns the deterministic RM-002 aggregate. It reads only
// recorded artifacts through the existing reader shapes: it makes no model call
// and no network call, and it never mutates the run directory.
//
// Every emitted metric carries five distinct quantities (total runs, eligible
// runs, artifacts present, valid measurements, unavailable measurements) plus an
// explicit numerator, denominator, and coverage percentage. An absent artifact
// yields UNAVAILABLE and reduces coverage; a present artifact with an explicit
// zero is a recorded measurement that counts toward the denominator. Token usage
// is always UNAVAILABLE and appears in no gate, budget, or routing field.
func AggregateProject(projectDir string) (Aggregate, error) {
	ids, err := RunIDs(projectDir)
	if err != nil {
		return Aggregate{}, err
	}
	evidence := make([]runEvidence, 0, len(ids))
	for _, id := range ids {
		evidence = append(evidence, readEvidence(projectDir, id))
	}
	return aggregate(evidence), nil
}

// aggregate folds the per-run evidence into the deterministic aggregate.
func aggregate(ev []runEvidence) Aggregate {
	corpus := len(ev)

	metrics := make([]MetricResult, 0, len(metricOrder))
	for _, m := range metricOrder {
		metrics = append(metrics, metricFor(m, ev, corpus))
	}

	artifacts := artifactPresence(ev)

	unavailable := make([]string, 0, len(metrics))
	for _, m := range metrics {
		if m.Value == UNAVAILABLE {
			unavailable = append(unavailable, string(m.Metric))
		}
	}
	sort.Strings(unavailable)

	return Aggregate{
		CorpusRuns:  corpus,
		Metrics:     metrics,
		Artifacts:   artifacts,
		Unavailable: unavailable,
	}
}

// metricFor emits one metric with its five measurement quantities, an explicit
// numerator and denominator, and coverage.
//
// The denominator is the base the numerator is measured over (the runs carrying
// the metric's usable source), and coverage is ValidMeasurements/TotalRuns. Runs
// whose source artifact is absent are UNAVAILABLE and excluded, never folded in
// as zero; a present artifact with a recorded zero is a real measurement.
func metricFor(m Metric, ev []runEvidence, corpus int) MetricResult {
	switch m {
	case MetricAgentSuccess:
		// Artifacts present: report.json or trace.json. Valid: a nonempty terminal
		// decision. Numerator: valid runs recording a terminal success.
		present := count(ev, func(e runEvidence) bool { return e.hasReport || e.hasTrace })
		valid := count(ev, func(e runEvidence) bool { return e.reportDecided || e.traceTerminal })
		num := count(ev, func(e runEvidence) bool {
			return (e.reportDecided || e.traceTerminal) && e.success
		})
		return rate(m, StatusAvailable, corpus, present, valid, num, valid,
			"numerator counts runs with a recorded terminal success; a run with neither report.json nor trace.json is UNAVAILABLE, never a failure")

	case MetricRouting:
		// Artifacts present: model-selection.json. Readable: the file parsed.
		// Usable/valid: a recorded routing selection. Numerator: runs recording a
		// routing selection. The finding-severity sub-signal is read from review.json
		// and recorded in the note; routing is opt-in, so coverage reflects its share.
		present := count(ev, func(e runEvidence) bool { return e.hasModelSel })
		readable := count(ev, func(e runEvidence) bool { return e.modelSelReadable })
		valid := count(ev, func(e runEvidence) bool { return e.modelSelReadable && e.routed })
		return countMetric(m, StatusPartial, corpus, present, valid, valid, readable, "runs",
			[]string{modelSelectionFileName, reviewFileName},
			"numerator counts runs with a recorded routing selection over the runs whose model-selection.json parsed; routing is opt-in, so a run without it is UNAVAILABLE, never a default. Finding severities are read from review.json")

	case MetricFixLoop:
		// Artifacts present/valid: metrics.json (parsed). Numerator: summed
		// fix_cycles + plan_repairs; a recorded zero counts.
		present := count(ev, func(e runEvidence) bool { return e.hasMetrics })
		valid := count(ev, func(e runEvidence) bool { return e.metricsParsed })
		num := 0
		for _, e := range ev {
			if e.metricsParsed {
				num += e.counts.FixCycles + e.counts.PlanRepairs
			}
		}
		return countMetric(m, StatusAvailable, corpus, present, valid, num, valid, "cycles",
			[]string{metricsFileName},
			"numerator sums counts.fix_cycles + counts.plan_repairs over runs whose metrics.json parsed; a run without metrics.json is unmeasured, not converged in zero cycles")

	case MetricRetry:
		// Artifacts present: attempt.txt, continuations.txt, or classification.json.
		// Readable: at least one parsed. Numerator: runs recording a positive retry
		// signal (a nonempty attempt, a continuations count greater than zero, or a
		// parseable classification record). A present continuations.txt of "0" is a
		// recorded zero, not a retry.
		present := count(ev, func(e runEvidence) bool {
			return e.hasAttempt || e.hasContinuatn || e.hasClass
		})
		readable := count(ev, func(e runEvidence) bool {
			return e.hasAttempt || e.continuationsOK || e.classReadable
		})
		num := count(ev, func(e runEvidence) bool {
			return e.hasAttempt || (e.continuationsOK && e.continuations > 0) || e.classReadable
		})
		return countMetric(m, StatusPartial, corpus, present, readable, num, readable, "runs",
			[]string{attemptFileName, continuationsFileName, classificationFileName},
			"numerator counts runs with a positive escalation signal (nonempty attempt.txt, continuations.txt > 0, or classification.json); absence is UNAVAILABLE, never 0 retries, and continuations.txt of 0 is a recorded zero")

	case MetricApproval:
		// Artifacts present: approval.json or report.json. Valid: an approval
		// record or a nonempty report decision. Numerator: runs recording an
		// approval request or a NEEDS_HUMAN terminal decision.
		present := count(ev, func(e runEvidence) bool { return e.hasApproval || e.hasReport })
		valid := count(ev, func(e runEvidence) bool { return e.hasApproval || e.reportDecided })
		num := count(ev, func(e runEvidence) bool {
			return (e.hasApproval || e.reportDecided) && e.approvalRequested
		})
		return rate(m, StatusAvailable, corpus, present, valid, num, valid,
			"numerator counts runs with a recorded approval request or NEEDS_HUMAN decision; absence is UNAVAILABLE, never a recorded no-approval-requested")

	case MetricLatency:
		// Artifacts present/valid: metrics.json (parsed). Numerator: summed
		// total_ms. A present metrics.json with total_ms 0 is a recorded zero; an
		// absent metrics.json is UNAVAILABLE.
		present := count(ev, func(e runEvidence) bool { return e.hasMetrics })
		valid := count(ev, func(e runEvidence) bool { return e.metricsParsed })
		var num int
		for _, e := range ev {
			if e.metricsParsed {
				num += int(e.metricsData.TotalMS)
			}
		}
		return countMetric(m, StatusAvailable, corpus, present, valid, num, valid, "ms",
			[]string{metricsFileName},
			"numerator sums recorded total_ms over runs whose metrics.json parsed; a recorded total_ms 0 is a measurement, an absent metrics.json is UNAVAILABLE")

	case MetricTokens:
		// No recorded source exists for token usage. The value is UNAVAILABLE for
		// every run and is never inferred.
		return MetricResult{
			Metric:                  MetricTokens,
			Status:                  StatusUnavailable,
			Value:                   UNAVAILABLE,
			TotalRuns:               corpus,
			UnavailableMeasurements: corpus,
			Unit:                    "tokens",
			Note:                    "no provider-independent token count is persisted; never inferred from text, byte, or diff size; informational only and never an input to a gate, budget, or routing decision",
		}
	}
	// Unreachable: metricOrder covers every Metric constant.
	return MetricResult{Metric: m, Status: StatusUnavailable, Value: UNAVAILABLE, TotalRuns: corpus}
}

// rate builds a metric whose value is a percentage: Numerator/Denominator over
// the valid runs, rendered to one decimal. When there is no valid measurement the
// value is UNAVAILABLE.
func rate(m Metric, status Status, corpus, present, valid, num, den int, note string) MetricResult {
	r := base(m, status, corpus, present, valid, num, den, "%", note)
	if den > 0 {
		r.Value = fmt.Sprintf("%.1f", round1(float64(num)/float64(den)*100))
	}
	return r
}

// countMetric builds a metric whose value is a run count (numerator), with its
// unit and sources.
func countMetric(m Metric, status Status, corpus, present, valid, num, den int, unit string, sources []string, note string) MetricResult {
	r := base(m, status, corpus, present, valid, num, den, unit, note)
	r.Sources = sources
	if den > 0 {
		r.Value = fmt.Sprintf("%d", num)
	}
	return r
}

// base fills the common fields and applies the fail-closed rules: zero corpus or
// zero denominator yields UNAVAILABLE with a zero numerator and denominator.
// Coverage is ValidMeasurements/TotalRuns and is never inflated by absence.
func base(m Metric, status Status, corpus, present, valid, num, den int, unit, note string) MetricResult {
	r := MetricResult{
		Metric:                  m,
		Status:                  status,
		Value:                   UNAVAILABLE,
		Unit:                    unit,
		TotalRuns:               corpus,
		EligibleRuns:            present,
		ArtifactsPresent:        present,
		ValidMeasurements:       valid,
		UnavailableMeasurements: maxInt(corpus-valid, 0),
		Numerator:               num,
		Denominator:             den,
		CoveragePct:             coverage(valid, corpus),
		Note:                    note,
	}
	if corpus == 0 || den == 0 {
		r.Numerator = 0
		r.Denominator = 0
		if corpus == 0 {
			r.Note = "no recorded runs in the corpus"
		}
	}
	return r
}

// coverage renders valid/corpus as a percentage, rounded to one decimal.
func coverage(valid, corpus int) float64 {
	if corpus <= 0 || valid <= 0 {
		return 0
	}
	return round1(float64(valid) / float64(corpus) * 100)
}

// round1 rounds to one decimal place, correct for negative and non-representable
// inputs (math.Round rounds half away from zero).
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func count(ev []runEvidence, ok func(runEvidence) bool) int {
	n := 0
	for _, e := range ev {
		if ok(e) {
			n++
		}
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// artifactPresence reports each recorded artifact's presence across the corpus,
// distinguishing present, absent, and present-with-zero. The output is sorted by
// artifact name so it is deterministic.
func artifactPresence(ev []runEvidence) []ArtifactPresence {
	specs := []struct {
		name string
		has  func(runEvidence) bool
		zero func(runEvidence) bool
	}{
		{reportFileName, func(e runEvidence) bool { return e.hasReport }, nil},
		{metricsFileName, func(e runEvidence) bool { return e.hasMetrics }, func(e runEvidence) bool {
			return e.hasMetrics && e.counts == (perf.Counts{}) && e.metricsData.TotalMS == 0
		}},
		{modelSelectionFileName, func(e runEvidence) bool { return e.hasModelSel }, nil},
		{classificationFileName, func(e runEvidence) bool { return e.hasClass }, nil},
		{approvalFileName, func(e runEvidence) bool { return e.hasApproval }, nil},
		{traceFileName, func(e runEvidence) bool { return e.hasTrace }, nil},
		{attemptFileName, func(e runEvidence) bool { return e.hasAttempt }, nil},
		{continuationsFileName, func(e runEvidence) bool { return e.hasContinuatn }, nil},
		{reviewFileName, func(e runEvidence) bool { return e.hasReview }, nil},
	}

	out := make([]ArtifactPresence, 0, len(specs))
	for _, s := range specs {
		presentRuns := count(ev, s.has)
		zeroRuns := 0
		if s.zero != nil {
			zeroRuns = count(ev, s.zero)
		}
		state := ArtifactPresent
		switch {
		case presentRuns == 0:
			state = ArtifactAbsent
		case zeroRuns > 0:
			state = ArtifactZero
		}
		out = append(out, ArtifactPresence{
			Name:        s.name,
			State:       state,
			PresentRuns: presentRuns,
			ZeroRuns:    zeroRuns,
			Denominator: presentRuns,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// String renders a metric name.
func (m Metric) String() string { return string(m) }
