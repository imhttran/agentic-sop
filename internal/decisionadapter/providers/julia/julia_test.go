package julia

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// countingRunner records Run/Available calls for assertions.
type countingRunner struct {
	outputs    Outputs
	err        error
	available  bool
	runCalls   int
	availCalls int
}

func (r *countingRunner) Name() string { return ProviderName }

func (r *countingRunner) Available(context.Context) bool {
	r.availCalls++
	return r.available
}

func (r *countingRunner) Run(context.Context, Inputs) (Outputs, error) {
	r.runCalls++
	return r.outputs, r.err
}

func TestProviderName(t *testing.T) {
	if got := New(Config{}, &countingRunner{}).Name(); got != "julia" {
		t.Errorf("Name() = %q, want julia", got)
	}
}

func TestProviderAvailable(t *testing.T) {
	tests := map[string]struct {
		available bool
		want      bool
	}{
		"available":   {available: true, want: true},
		"unavailable": {available: false, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := &countingRunner{available: tt.available}
			p := New(Config{}, r)
			if got := p.Available(context.Background()); got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
			if r.runCalls != 0 {
				t.Errorf("Available() invoked Run %d times, want 0", r.runCalls)
			}
		})
	}
}

func TestProviderAvailableZeroValue(t *testing.T) {
	// A provider built with a zero Config has an unconfigured CommandRunner and
	// must report unavailable rather than panic or silently succeed.
	p := New(Config{}, nil)
	if p.Available(context.Background()) {
		t.Error("Available() = true, want false for unconfigured runner")
	}
}

func TestProviderDecideSuccess(t *testing.T) {
	r := &countingRunner{outputs: okOutputs()}
	got, err := New(Config{Model: "julia"}, r).Decide(context.Background(), exampleRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if got.Provider != "julia" || got.Model != "julia" {
		t.Errorf("Decide() = %+v", got)
	}
	if len(got.Answers) != 3 || got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("Decide() answers = %+v", got.Answers)
	}
	if r.runCalls != 1 {
		t.Errorf("Run called %d times, want 1", r.runCalls)
	}
}

func TestProviderDecideModelResolution(t *testing.T) {
	tests := map[string]struct {
		cfgModel    string
		runnerModel string
		want        string
	}{
		"configured model used when runner reports none": {
			cfgModel:    "julia-custom",
			runnerModel: "",
			want:        "julia-custom",
		},
		"runner-reported model wins over configured model": {
			cfgModel:    "julia-custom",
			runnerModel: "julia-runner",
			want:        "julia-runner",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := okOutputs()
			out.Model = tt.runnerModel
			r := &countingRunner{outputs: out}
			got, err := New(Config{Model: tt.cfgModel}, r).Decide(context.Background(), exampleRequest())
			if err != nil {
				t.Fatalf("Decide() error = %v", err)
			}
			if got.Model != tt.want {
				t.Errorf("Decide() Model = %q, want %q", got.Model, tt.want)
			}
		})
	}
}

func TestProviderDecideInvalidRequest(t *testing.T) {
	r := &countingRunner{}
	_, err := New(Config{}, r).Decide(context.Background(), decision.DecisionRequest{})
	if !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
	if r.runCalls != 0 {
		t.Errorf("Run called %d times, want 0 for invalid request", r.runCalls)
	}
	assertProviderError(t, err)
}

func TestProviderDecideRejectsJuliaOptionLimits(t *testing.T) {
	// A one-choice question is valid for the provider-neutral contract but has no
	// valid Julia decision, so Decide must reject it before running inference.
	r := &countingRunner{outputs: okOutputs()}
	req := decision.DecisionRequest{
		State: "s",
		Questions: []decision.Question{{
			ID:       "risk",
			Type:     decision.QuestionChoice,
			Criteria: "c",
			Choices:  []string{"LOW"},
		}},
	}
	_, err := New(Config{}, r).Decide(context.Background(), req)
	if !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
	if r.runCalls != 0 {
		t.Errorf("Run called %d times, want 0 for a request Julia cannot represent", r.runCalls)
	}
	assertProviderError(t, err)
}

func TestProviderDecideUnavailable(t *testing.T) {
	r := &countingRunner{err: errRunnerUnavailable}
	_, err := New(Config{}, r).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	assertProviderError(t, err)
}

func TestProviderDecideMalformedResponse(t *testing.T) {
	r := &countingRunner{err: errRunnerMalformed}
	_, err := New(Config{}, r).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
	assertProviderError(t, err)
}

func TestProviderDecideNonNormalizableOutput(t *testing.T) {
	// A runner that succeeds but returns output the adapter cannot normalize is
	// a malformed response.
	out := okOutputs()
	delete(out.Answers, "risk")
	r := &countingRunner{outputs: out}
	_, err := New(Config{}, r).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
	assertProviderError(t, err)
}

func TestProviderDecideGenericFailure(t *testing.T) {
	r := &countingRunner{err: errors.New("boom")}
	_, err := New(Config{}, r).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("error = %v, want ErrProviderFailure", err)
	}
	assertProviderError(t, err)
}

func TestProviderDecideRunnerFunc(t *testing.T) {
	rf := RunnerFunc{
		AvailableValue: true,
		RunFunc: func(context.Context, Inputs) (Outputs, error) {
			return okOutputs(), nil
		},
	}
	got, err := New(Config{}, rf).Decide(context.Background(), exampleRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if got.Provider != "julia" {
		t.Errorf("Provider = %q, want julia", got.Provider)
	}
}

func assertProviderError(t *testing.T, err error) {
	t.Helper()
	var perr *decision.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *decision.ProviderError", err)
	}
	if perr.Provider != "julia" {
		t.Errorf("ProviderError.Provider = %q, want julia", perr.Provider)
	}
}
