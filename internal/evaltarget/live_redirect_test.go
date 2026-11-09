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
	client := &http.Client{
		CheckRedirect: noRedirect,
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
