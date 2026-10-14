package nimble

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

func scoreReq() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "a schema migration during business hours",
		Questions: []decision.Question{{
			ID:       "change-risk",
			Type:     decision.QuestionChoice,
			Criteria: "Assess the operational risk of the change.",
			Choices:  []string{"LOW", "MEDIUM", "HIGH"},
		}},
	}
}

func scoreProvider(t *testing.T, url string) *Provider {
	t.Helper()
	return New(Config{Transport: TransportScore, ScoreURL: url, Timeout: 2 * time.Second}, nil)
}

// TestScoreTransportValidChoice proves a standalone /score response normalizes
// into the decision contract: the chosen choice and the maximum score confidence.
func TestScoreTransportValidChoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/score" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"bespokelabs/Bespoke-Nimble-9B","backend":"mlx","output":{"change-risk":"HIGH"},"fields":{"change-risk":{"value":"HIGH","scores":{"LOW":0.03,"MEDIUM":0.29,"HIGH":0.68}}}}`)
	}))
	defer srv.Close()

	res, err := scoreProvider(t, srv.URL+"/score").Decide(context.Background(), scoreReq())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.Provider != ProviderName || res.Model != "bespokelabs/Bespoke-Nimble-9B" {
		t.Fatalf("result = %+v, want nimble/Bespoke-Nimble-9B", res)
	}
	a, ok := res.Answers["change-risk"]
	if !ok || a.Choice != "HIGH" {
		t.Fatalf("answer = %+v, want choice HIGH", res.Answers)
	}
	if a.Confidence == nil || *a.Confidence < 0.67 || *a.Confidence > 0.69 {
		t.Errorf("confidence = %v, want ~0.68", a.Confidence)
	}
}

// TestScoreTransportMalformed proves an undecodable or contract-violating /score
// response is a malformed response, never a decision.
func TestScoreTransportMalformed(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"bad json", `{not json`},
		{"missing field", `{"model":"m","fields":{}}`},
		{"choice not allowed", `{"model":"m","fields":{"change-risk":{"value":"HUGE","scores":{"HUGE":1.0}}}}`},
		{"score out of range", `{"model":"m","fields":{"change-risk":{"value":"HIGH","scores":{"HIGH":1.5}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			_, err := scoreProvider(t, srv.URL+"/score").Decide(context.Background(), scoreReq())
			if err == nil {
				t.Fatalf("expected a malformed response error, got nil")
			}
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Errorf("err = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

// TestScoreTransportUnavailable proves a refused connection is normalized to
// ErrUnavailable, never a decision.
func TestScoreTransportUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/score"
	srv.Close()

	_, err := scoreProvider(t, url).Decide(context.Background(), scoreReq())
	if err == nil || !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestScoreTransportTimeout proves a slow service does not stall the caller: the
// bounded request fails closed as unavailable.
func TestScoreTransportTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, `{"model":"m","fields":{"change-risk":{"value":"LOW","scores":{"LOW":1.0}}}}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := scoreProvider(t, srv.URL+"/score").Decide(ctx, scoreReq())
	if err == nil || !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestBuildScoreRequest proves the translation to the /score enum schema, and
// that a non-choice question is rejected for this transport.
func TestBuildScoreRequest(t *testing.T) {
	r, err := BuildScoreRequest(scoreReq())
	if err != nil {
		t.Fatalf("BuildScoreRequest: %v", err)
	}
	if r.Context != "a schema migration during business hours" {
		t.Errorf("context = %q", r.Context)
	}
	f, ok := r.Schema["change-risk"]
	if !ok || f.Type != "enum" || strings.Join(f.Choices, "|") != "LOW|MEDIUM|HIGH" || strings.TrimSpace(f.Description) == "" {
		t.Errorf("schema field = %+v, want enum with choices and a description", f)
	}

	boolean := decision.DecisionRequest{
		State:     "x",
		Questions: []decision.Question{{ID: "b", Type: decision.QuestionBoolean}},
	}
	if _, err := BuildScoreRequest(boolean); err == nil || !errors.Is(err, decision.ErrInvalidRequest) {
		t.Errorf("boolean question err = %v, want ErrInvalidRequest", err)
	}
}

// TestConfigFromEnvScoreTransport proves NIMBLE_TRANSPORT / NIMBLE_SCORE_URL read
// through, and that the default stays the SystemOne transport.
func TestConfigFromEnvScoreTransport(t *testing.T) {
	t.Setenv("NIMBLE_TRANSPORT", "")
	t.Setenv("NIMBLE_SCORE_URL", "")
	if c := ConfigFromEnv(); c.Transport != TransportSystemOne || strings.TrimSpace(c.ScoreURL) == "" {
		t.Errorf("default transport = %q / %q, want systemone + a default score URL", c.Transport, c.ScoreURL)
	}
	t.Setenv("NIMBLE_TRANSPORT", "score")
	t.Setenv("NIMBLE_SCORE_URL", "http://127.0.0.1:9001/score")
	c := ConfigFromEnv()
	if c.Transport != TransportScore || c.ScoreURL != "http://127.0.0.1:9001/score" {
		t.Errorf("env transport = %q / %q, want score + the configured URL", c.Transport, c.ScoreURL)
	}
}
