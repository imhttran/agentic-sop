package nimble

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// fakeTransport is an in-memory Transport, so provider tests never need a
// running Ollama instance.
type fakeTransport struct {
	sysResp systemOneResponse
	sysErr  error
	models  []string
	listErr error

	calls int
	last  systemOneRequest
}

func (f *fakeTransport) SystemOne(_ context.Context, req systemOneRequest) (systemOneResponse, error) {
	f.calls++
	f.last = req
	return f.sysResp, f.sysErr
}

func (f *fakeTransport) ListModels(context.Context) ([]string, error) {
	return f.models, f.listErr
}

func okResponse() systemOneResponse {
	return systemOneResponse{
		Model: "nimble",
		Answers: map[string]systemOneAnswer{
			"risk":              {Type: systemOneTypeChoice, Choice: "HIGH", Confidence: fptr(0.83), Probabilities: map[string]float64{"HIGH": 0.95}},
			"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.997)},
			"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE", Confidence: fptr(0.84)},
		},
		Usage: systemOneUsage{InputTokens: 983, OutputTokens: 4},
	}
}

func TestProviderName(t *testing.T) {
	if got := New(Config{}, &fakeTransport{}).Name(); got != "nimble" {
		t.Errorf("Name() = %q, want nimble", got)
	}
}

func TestProviderAvailable(t *testing.T) {
	tests := map[string]struct {
		tr   *fakeTransport
		want bool
	}{
		"ollama unreachable":   {tr: &fakeTransport{listErr: errors.New("connection refused")}, want: false},
		"model absent":         {tr: &fakeTransport{models: []string{"llama3"}}, want: false},
		"model present":        {tr: &fakeTransport{models: []string{"nimble"}}, want: true},
		"model present tagged": {tr: &fakeTransport{models: []string{"nimble:latest"}}, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := New(Config{Model: "nimble"}, tt.tr).Available(context.Background()); got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderDecideSuccess(t *testing.T) {
	tr := &fakeTransport{sysResp: okResponse()}
	got, err := New(Config{Model: "nimble"}, tr).Decide(context.Background(), exampleRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if got.Provider != "nimble" || got.Model != "nimble" {
		t.Errorf("Decide() = %+v", got)
	}
	if len(got.Answers) != 3 {
		t.Errorf("len(Answers) = %d, want 3", len(got.Answers))
	}
	if got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("risk = %+v", got.Answers["risk"])
	}
	if got.Answers["approval_required"].Probability != 0.997 {
		t.Errorf("approval_required = %+v", got.Answers["approval_required"])
	}
	if tr.calls != 1 {
		t.Errorf("SystemOne called %d times, want 1", tr.calls)
	}
	if tr.last.Model != "nimble" {
		t.Errorf("translated model = %q", tr.last.Model)
	}
}

func TestProviderDecideInvalidRequest(t *testing.T) {
	tr := &fakeTransport{}
	_, err := New(Config{}, tr).Decide(context.Background(), decision.DecisionRequest{})
	if !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
	if tr.calls != 0 {
		t.Errorf("SystemOne called %d times, want 0 for invalid request", tr.calls)
	}
	var perr *decision.ProviderError
	if !errors.As(err, &perr) || perr.Provider != "nimble" {
		t.Errorf("error = %v, want *decision.ProviderError from nimble", err)
	}
}

func TestProviderDecideUnavailable(t *testing.T) {
	tr := &fakeTransport{sysErr: errors.New("dial tcp: connection refused")}
	_, err := New(Config{}, tr).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("error = %v, want ErrProviderFailure", err)
	}
}

func TestProviderDecideMalformedResponse(t *testing.T) {
	tr := &fakeTransport{sysResp: systemOneResponse{Model: "nimble", Answers: map[string]systemOneAnswer{}}}
	_, err := New(Config{}, tr).Decide(context.Background(), exampleRequest())
	if !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

// TestProviderDecideEndToEnd wires the real HTTP transport to an httptest
// server, proving the full native /v1/systemone path works without a running
// Ollama instance.
func TestProviderDecideEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"name":"nimble"}]}`)
	})
	mux.HandleFunc("/v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		var req systemOneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode systemone request: %v", err)
		}
		if req.Model != "nimble" {
			t.Errorf("model = %q, want nimble", req.Model)
		}
		_ = json.NewEncoder(w).Encode(okResponse())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, Model: "nimble", Timeout: 2 * time.Second}, nil)
	ctx := context.Background()

	if !p.Available(ctx) {
		t.Fatal("Available() = false, want true")
	}
	got, err := p.Decide(ctx, exampleRequest())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if len(got.Answers) != 3 || got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("Decide() = %+v", got)
	}
}
