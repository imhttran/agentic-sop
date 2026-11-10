package evaltarget

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestLocalDeciderDoesNotFollowRedirect proves the live decider never follows a
// redirect, even one aimed at another loopback port. A fixed URL alone is not
// evidence; the redirect policy is applied and observed here.
func TestLocalDeciderDoesNotFollowRedirect(t *testing.T) {
	requests := 0
	// The supplied client has the default (follow) policy; the decider must
	// override it so a caller cannot re-enable redirects.
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1:11435/other"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
	}
	d := LocalDecider{Endpoint: LiveEndpointURL, Model: "fixture-model", Prompt: "p", Client: client}
	if _, err := d.Decide(context.Background()); err == nil {
		t.Fatal("a redirect response was accepted")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want exactly 1 (a redirect must not be followed)", requests)
	}
}

// TestLocalDeciderSendsExplicitThinkOption proves the optional Ollama thinking
// control is carried in the request body only when the caller sets it. The live
// campaign needs think=false so a reasoning model does not spend its bounded
// output on hidden reasoning instead of the one JSON response.
func TestLocalDeciderSendsExplicitThinkOption(t *testing.T) {
	think := false
	var body string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			data, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			body = string(data)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"response":"{\"ok\":true}","done":true}`)),
				Request:    req,
			}, nil
		}),
	}
	d := LocalDecider{Endpoint: LiveEndpointURL, Model: "fixture-model", Prompt: "p", Client: client, Think: &think}
	if _, err := d.Decide(context.Background()); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !strings.Contains(body, `"think":false`) {
		t.Fatalf("request body does not carry think=false: %s", body)
	}

	body = ""
	d.Think = nil
	if _, err := d.Decide(context.Background()); err != nil {
		t.Fatalf("Decide without think: %v", err)
	}
	if strings.Contains(body, `"think"`) {
		t.Fatalf("request body carries think when the caller did not set it: %s", body)
	}
}
