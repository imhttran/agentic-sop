package clef

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// stubTransport is a fake transport that returns preconfigured results. It lets
// Provider.Decide be driven without any real model backend.
type stubTransport struct {
	resp systemOneResponse
	err  error
}

func (s stubTransport) SystemOne(context.Context, systemOneRequest) (systemOneResponse, error) {
	return s.resp, s.err
}

func (s stubTransport) ListModels(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []string{"clef-flash"}, nil
}

// enabledConfig is a valid enabled Clef configuration for tests.
func enabledConfig() Config {
	return Config{BaseURL: "http://127.0.0.1:0", Model: "clef-flash", Timeout: time.Second, Enable: true}
}

// assertProviderErrorKind asserts err is a *decision.ProviderError for the Clef
// adapter with the wanted kind.
func assertProviderErrorKind(t *testing.T, err error, want decision.ErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want ProviderError kind %q", want)
	}
	var perr *decision.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *decision.ProviderError", err)
	}
	if perr.Provider != ProviderName {
		t.Errorf("ProviderError.Provider = %q, want %q", perr.Provider, ProviderName)
	}
	if perr.Kind != want {
		t.Errorf("ProviderError.Kind = %q, want %q", perr.Kind, want)
	}
}

// assertNoSuccess asserts a failed DecisionResult carries no answers or usage,
// so a failure can never be mistaken for successful evidence.
func assertNoSuccess(t *testing.T, got decision.DecisionResult) {
	t.Helper()
	if len(got.Answers) != 0 {
		t.Errorf("Answers = %+v, want empty on failure", got.Answers)
	}
	if got.Usage.InputTokens != 0 || got.Usage.OutputTokens != 0 {
		t.Errorf("Usage = %+v, want zero on failure", got.Usage)
	}
}

// TestDecideMalformedTransportResponse asserts a malformed transport failure
// maps to ERROR (KindMalformedResponse) and never yields success-shaped output.
func TestDecideMalformedTransportResponse(t *testing.T) {
	p := New(enabledConfig(), stubTransport{err: &malformedResponseError{Err: errors.New("decode")}})

	got, err := p.Decide(context.Background(), choiceRequest())
	assertProviderErrorKind(t, err, decision.KindMalformedResponse)
	assertNoSuccess(t, got)
}

// TestDecideEmptyResponse asserts a 200-with-empty-body failure surfaces through
// the real httpTransport against a fake server and maps to ERROR.
func TestDecideEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 with an empty body.
	}))
	defer srv.Close()

	p := New(enabledConfig(), newHTTPTransport(srv.URL, time.Second))
	got, err := p.Decide(context.Background(), choiceRequest())
	assertProviderErrorKind(t, err, decision.KindMalformedResponse)
	assertNoSuccess(t, got)
}

// TestDecideTimeout asserts a timeout maps to ERROR (KindUnavailable) and never
// yields a success-shaped result.
func TestDecideTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer srv.Close()

	cfg := enabledConfig()
	cfg.Timeout = 20 * time.Millisecond
	p := New(cfg, newHTTPTransport(srv.URL, 20*time.Millisecond))

	got, err := p.Decide(context.Background(), choiceRequest())
	assertProviderErrorKind(t, err, decision.KindUnavailable)
	assertNoSuccess(t, got)
}

// TestDecideCancelledContext asserts a cancelled context maps to ERROR
// (KindUnavailable) and never yields a success-shaped result.
func TestDecideCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := New(enabledConfig(), newHTTPTransport(srv.URL, time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := p.Decide(ctx, choiceRequest())
	assertProviderErrorKind(t, err, decision.KindUnavailable)
	assertNoSuccess(t, got)
}

// TestDecideTransportFailureForChoiceQuestion asserts the stub-driven failure
// paths each produce a non-success result for every question type; transport
// failure never yields successful evidence.
func TestDecideTransportFailureNeverSucceeds(t *testing.T) {
	failures := map[string]error{
		"malformed": &malformedResponseError{Err: errors.New("x")},
		"timeout":   context.DeadlineExceeded,
		"cancelled": context.Canceled,
		"generic":   errors.New("backend exploded"),
	}
	for name, ferr := range failures {
		t.Run(name, func(t *testing.T) {
			p := New(enabledConfig(), stubTransport{err: ferr})
			got, err := p.Decide(context.Background(), choiceRequest())
			if err == nil {
				t.Fatal("Decide() error = nil, want failure, never success")
			}
			assertNoSuccess(t, got)
		})
	}
}

// TestDecideDisabled asserts a disabled provider fails closed with
// KindUnavailable and no success-shaped result.
func TestDecideDisabled(t *testing.T) {
	p := New(Config{}, stubTransport{resp: systemOneResponse{Answers: baseAnswers()}})

	got, err := p.Decide(context.Background(), choiceRequest())
	assertProviderErrorKind(t, err, decision.KindUnavailable)
	assertNoSuccess(t, got)
}

// TestDecideInvalidRequest asserts a malformed request fails closed with
// KindInvalidRequest before any transport call.
func TestDecideInvalidRequest(t *testing.T) {
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Answers: baseAnswers()}})

	bad := decision.DecisionRequest{State: "   ", Questions: choiceRequest().Questions}
	got, err := p.Decide(context.Background(), bad)
	assertProviderErrorKind(t, err, decision.KindInvalidRequest)
	assertNoSuccess(t, got)
}

// TestDecideUnsupportedOperation asserts an operation the adapter cannot
// express returns UNSUPPORTED rather than a success-shaped answer. A SCORE
// question without enough candidate descriptions passes request validation but
// cannot be represented on the SystemOne transport, so it exercises the genuine
// unsupported-operation path (an unknown question type is instead rejected by
// request validation as an invalid request before translation).
func TestDecideUnsupportedOperation(t *testing.T) {
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Answers: baseAnswers()}})

	req := decision.DecisionRequest{
		State:     "deploying a schema migration during business hours",
		Questions: []decision.Question{{ID: "confidence_score", Type: decision.QuestionScore}},
	}
	got, err := p.Decide(context.Background(), req)
	if err == nil {
		t.Fatal("Decide() error = nil, want UNSUPPORTED")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("errors.Is(err, ErrUnsupported) = false, want true; err=%v", err)
	}
	var perr *decision.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *decision.ProviderError", err)
	}
	if perr.Kind != decision.KindInvalidRequest {
		t.Errorf("Kind = %q, want %q", perr.Kind, decision.KindInvalidRequest)
	}
	assertNoSuccess(t, got)
}

// TestDecideUnsupportedScoreWithoutCandidates asserts a SCORE question the
// adapter cannot express returns UNSUPPORTED, not success and not a transport
// failure.
func TestDecideUnsupportedScoreWithoutCandidates(t *testing.T) {
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Answers: baseAnswers()}})

	req := decision.DecisionRequest{
		State:     "deploying a schema migration during business hours",
		Questions: []decision.Question{{ID: "confidence_score", Type: decision.QuestionScore, Choices: []string{"only"}}},
	}
	got, err := p.Decide(context.Background(), req)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}
	assertNoSuccess(t, got)
}

// TestDecideIndeterminateStaysIndeterminate asserts an indeterminate response
// (missing answer) maps to ERROR (KindMalformedResponse) and is never collapsed
// into a success-shaped answer.
func TestDecideIndeterminateStaysIndeterminate(t *testing.T) {
	answers := baseAnswers()
	delete(answers, "risk")
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Model: "clef-flash", Answers: answers}})

	got, err := p.Decide(context.Background(), multiRequest())
	assertProviderErrorKind(t, err, decision.KindMalformedResponse)
	assertNoSuccess(t, got)
}

// TestDecideIndeterminateNilNoul asserts a null-like boolean (nil Noul) stays
// indeterminate rather than being read as a false/negative success.
func TestDecideIndeterminateNilBoolean(t *testing.T) {
	answers := baseAnswers()
	answers["approval_required"] = systemOneAnswer{Type: systemOneTypeBoolean}
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Model: "clef-flash", Answers: answers}})

	got, err := p.Decide(context.Background(), multiRequest())
	assertProviderErrorKind(t, err, decision.KindMalformedResponse)
	assertNoSuccess(t, got)
}

// TestDecideIndeterminateNilScore asserts a null-like score stays indeterminate.
func TestDecideIndeterminateNilScore(t *testing.T) {
	answers := baseAnswers()
	answers["confidence_score"] = systemOneAnswer{Type: systemOneTypeScore}
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{Model: "clef-flash", Answers: answers}})

	got, err := p.Decide(context.Background(), multiRequest())
	assertProviderErrorKind(t, err, decision.KindMalformedResponse)
	assertNoSuccess(t, got)
}

// TestDecideSuccessViaFakeTransport asserts a well-formed fake-transport
// response normalizes into a successful result without contacting a real model.
func TestDecideSuccessViaFakeTransport(t *testing.T) {
	p := New(enabledConfig(), stubTransport{resp: systemOneResponse{
		Model:   "clef-flash",
		Answers: baseAnswers(),
		Usage:   systemOneUsage{InputTokens: 983, OutputTokens: 4},
	}})

	got, err := p.Decide(context.Background(), multiRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if got.Provider != ProviderName || got.Model != "clef-flash" {
		t.Errorf("Provider/Model = %q/%q", got.Provider, got.Model)
	}
	if got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("risk answer = %+v", got.Answers["risk"])
	}
	if got.Usage.InputTokens != 983 {
		t.Errorf("Usage = %+v", got.Usage)
	}
}

// TestDecideHTTPStatusError asserts a non-200 backend status maps to ERROR
// (KindProviderFailure), never success.
func TestDecideHTTPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()

	p := New(enabledConfig(), newHTTPTransport(srv.URL, time.Second))
	got, err := p.Decide(context.Background(), choiceRequest())
	assertProviderErrorKind(t, err, decision.KindProviderFailure)
	assertNoSuccess(t, got)
}

// TestAvailableWithFakeTransport asserts availability is decided from the fake
// transport's model list without an inference request.
func TestAvailableWithFakeTransport(t *testing.T) {
	p := New(enabledConfig(), stubTransport{})
	if !p.Available(context.Background()) {
		t.Error("Available() = false, want true when the fake lists the model")
	}

	missing := New(enabledConfig(), modelListTransport{models: []string{"other:latest"}})
	if missing.Available(context.Background()) {
		t.Error("Available() = true, want false when the model is absent")
	}
}

// modelListTransport is a fake transport reporting a fixed model list.
type modelListTransport struct{ models []string }

func (m modelListTransport) SystemOne(context.Context, systemOneRequest) (systemOneResponse, error) {
	return systemOneResponse{}, errors.New("unexpected SystemOne call")
}

func (m modelListTransport) ListModels(context.Context) ([]string, error) {
	return m.models, nil
}
