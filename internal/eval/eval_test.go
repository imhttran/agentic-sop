package eval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/quality"
)

func TestRunAggregates(t *testing.T) {
	cases := []Case{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	runner := func(_ context.Context, c Case) Outcome {
		switch c.ID {
		case "a":
			return Outcome{Case: c, Decision: quality.Pass, Cycles: 1, Duration: time.Second}
		case "b":
			return Outcome{Case: c, Decision: quality.Fail, Findings: 2}
		case "c":
			return Outcome{Case: c, Decision: quality.NeedsHuman}
		default:
			return Outcome{Case: c, Err: errors.New("boom")}
		}
	}

	m, outs := Run(context.Background(), cases, runner)
	if m.Total != 4 || m.Passed != 1 || m.Failed != 1 || m.NeedsHuman != 1 || m.Errored != 1 {
		t.Errorf("metrics = %+v", m)
	}
	if m.Findings != 2 || m.Cycles != 1 {
		t.Errorf("aggregates = %+v", m)
	}
	if len(outs) != 4 {
		t.Errorf("outcomes = %d, want 4", len(outs))
	}
	if got := m.SuccessRate(); got != 0.25 {
		t.Errorf("SuccessRate = %v, want 0.25", got)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m, outs := Run(ctx, []Case{{ID: "a"}}, func(context.Context, Case) Outcome {
		t.Error("runner must not run after cancellation")
		return Outcome{}
	})
	if m.Total != 1 || len(outs) != 0 {
		t.Errorf("metrics=%+v outcomes=%d", m, len(outs))
	}
}

func TestSuccessRateEmpty(t *testing.T) {
	if got := (Metrics{}).SuccessRate(); got != 0 {
		t.Errorf("SuccessRate = %v, want 0", got)
	}
}
