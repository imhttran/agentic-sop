// Package eval runs a corpus of cases through a runner and aggregates the
// metrics a harness benchmark needs: task success, fix cycles, review findings,
// and elapsed time. It is control-plane and never calls a model itself — the
// caller supplies the runner (typically the run lifecycle).
package eval

import (
	"context"
	"time"

	"github.com/imhttran/agentic-sop/internal/quality"
)

// Case identifies one benchmark case.
type Case struct {
	ID   string
	Name string
}

// Outcome is the result of running one case.
type Outcome struct {
	Case     Case
	Decision quality.Decision
	Cycles   int
	Findings int
	Duration time.Duration
	Err      error
}

// Metrics aggregates outcomes across a corpus.
type Metrics struct {
	Total      int
	Passed     int
	Failed     int
	NeedsHuman int
	Errored    int
	// Continued counts cases whose run stopped short of finishing without
	// requiring a human decision (a resumable continuation).
	Continued int
	Findings  int
	Cycles    int
	Duration  time.Duration
}

// Run executes every case via runner and returns the aggregate metrics plus the
// per-case outcomes (in corpus order), stopping early on context cancellation.
func Run(ctx context.Context, cases []Case, runner func(context.Context, Case) Outcome) (Metrics, []Outcome) {
	metrics := Metrics{Total: len(cases)}
	outcomes := make([]Outcome, 0, len(cases))

	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			break
		}
		out := runner(ctx, c)
		outcomes = append(outcomes, out)
		metrics.add(out)
	}
	return metrics, outcomes
}

// add folds one outcome into the metrics.
func (m *Metrics) add(out Outcome) {
	switch {
	case out.Err != nil:
		m.Errored++
	case out.Decision == quality.Pass:
		m.Passed++
	case out.Decision == quality.Fail:
		m.Failed++
	case out.Decision == quality.NeedsHuman:
		m.NeedsHuman++
	case out.Decision == quality.Continue:
		m.Continued++
	}
	m.Findings += out.Findings
	m.Cycles += out.Cycles
	m.Duration += out.Duration
}

// SuccessRate is the fraction of cases that passed, in [0,1]. A corpus of zero
// cases has a rate of 0.
func (m Metrics) SuccessRate() float64 {
	if m.Total == 0 {
		return 0
	}
	return float64(m.Passed) / float64(m.Total)
}
