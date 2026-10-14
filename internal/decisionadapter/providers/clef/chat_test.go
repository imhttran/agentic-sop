package clef

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

func chatReq() decision.DecisionRequest {
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

func chatProvider(t *testing.T, base string) *Provider {
	t.Helper()
	return New(Config{Enable: true, Transport: TransportChat, BaseURL: base, Model: "clef-4bit", Timeout: 2 * time.Second}, nil)
}

func chatServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"model":"clef-4bit","choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, content)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"object":"list","data":[{"id":"clef-4bit"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestChatTransportValidChoice proves an OpenAI chat completion normalizes into
// the decision contract.
func TestChatTransportValidChoice(t *testing.T) {
	srv := chatServer(t, `{"answers":{"change-risk":{"choice":"HIGH","confidence":0.95}}}`)
	defer srv.Close()

	res, err := chatProvider(t, srv.URL).Decide(context.Background(), chatReq())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.Provider != ProviderName || res.Model != "clef-4bit" {
		t.Fatalf("result = %+v, want clef/clef-4bit", res)
	}
	a, ok := res.Answers["change-risk"]
	if !ok || a.Choice != "HIGH" || a.Confidence == nil || *a.Confidence != 0.95 {
		t.Fatalf("answer = %+v, want HIGH/0.95", res.Answers)
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

// TestChatTransportMalformed proves non-JSON content, an unknown choice, and a
// missing/invalid confidence are malformed responses, never decisions.
func TestChatTransportMalformed(t *testing.T) {
	cases := []struct{ name, content string }{
		{"not json", `I think it is HIGH`},
		{"choice not allowed", `{"answers":{"change-risk":{"choice":"HUGE","confidence":0.5}}}`},
		{"confidence missing", `{"answers":{"change-risk":{"choice":"HIGH"}}}`},
		{"confidence out of range", `{"answers":{"change-risk":{"choice":"HIGH","confidence":1.5}}}`},
		{"question omitted", `{"answers":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := chatServer(t, tc.content)
			defer srv.Close()
			_, err := chatProvider(t, srv.URL).Decide(context.Background(), chatReq())
			if err == nil || !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("err = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

// TestChatTransportDisabled proves a disabled provider serves no decision.
func TestChatTransportDisabled(t *testing.T) {
	srv := chatServer(t, `{"answers":{"change-risk":{"choice":"HIGH","confidence":0.9}}}`)
	defer srv.Close()
	p := New(Config{Transport: TransportChat, BaseURL: srv.URL, Model: "clef-4bit"}, nil)
	if _, err := p.Decide(context.Background(), chatReq()); err == nil || !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("disabled err = %v, want ErrUnavailable", err)
	}
}

// TestChatTransportUnavailable and timeout prove connectivity failures fail closed.
func TestChatTransportUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := srv.URL
	srv.Close()
	if _, err := chatProvider(t, base).Decide(context.Background(), chatReq()); err == nil || !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("unavailable err = %v, want ErrUnavailable", err)
	}
}

func TestChatTransportTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := chatProvider(t, srv.URL).Decide(ctx, chatReq()); err == nil || !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("timeout err = %v, want ErrUnavailable", err)
	}
}

// TestBuildChatRequest proves the request carries the choice and rejects a
// non-choice question.
func TestBuildChatRequest(t *testing.T) {
	r, err := BuildChatRequest("clef-4bit", chatReq())
	if err != nil {
		t.Fatalf("BuildChatRequest: %v", err)
	}
	if r.Model != "clef-4bit" || len(r.Messages) != 2 || r.Temperature != 0 {
		t.Errorf("chat request = %+v", r)
	}
	boolean := decision.DecisionRequest{State: "x", Questions: []decision.Question{{ID: "b", Type: decision.QuestionBoolean}}}
	if _, err := BuildChatRequest("clef-4bit", boolean); err == nil || !errors.Is(err, decision.ErrInvalidRequest) {
		t.Errorf("boolean err = %v, want ErrInvalidRequest", err)
	}
}

// TestChatAvailable proves chat-mode availability uses the OpenAI model listing.
func TestChatAvailable(t *testing.T) {
	srv := chatServer(t, `{}`)
	defer srv.Close()
	if !chatProvider(t, srv.URL).Available(context.Background()) {
		t.Error("clef-4bit present in /v1/models must be available")
	}
}

// TestConfigFromEnvChatTransport proves CLEF_TRANSPORT reads through and the
// default stays SystemOne.
func TestConfigFromEnvChatTransport(t *testing.T) {
	t.Setenv("CLEF_TRANSPORT", "")
	if c := ConfigFromEnv(); c.Transport != TransportSystemOne {
		t.Errorf("default transport = %q, want systemone", c.Transport)
	}
	t.Setenv("CLEF_TRANSPORT", "chat")
	if c := ConfigFromEnv(); c.Transport != TransportChat {
		t.Errorf("env transport = %q, want chat", c.Transport)
	}
}

// TestChatTransportSingleKeyForm proves the alternate single-key scoring form
// {"<choice>": <confidence>} the model sometimes emits is normalized too.
func TestChatTransportSingleKeyForm(t *testing.T) {
	srv := chatServer(t, `{"answers":{"change-risk":{"HIGH":0.8}}}`)
	defer srv.Close()
	res, err := chatProvider(t, srv.URL).Decide(context.Background(), chatReq())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	a := res.Answers["change-risk"]
	if a.Choice != "HIGH" || a.Confidence == nil || *a.Confidence != 0.8 {
		t.Fatalf("answer = %+v, want HIGH/0.8", a)
	}
}
