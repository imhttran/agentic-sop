package nimble

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

func systemOneServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *HTTPTransport) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, NewHTTPTransport(srv.URL, time.Second)
}

func TestHTTPTransportSystemOneSuccess(t *testing.T) {
	_, tr := systemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(systemOneResponse{
			Model:   "nimble",
			Answers: map[string]systemOneAnswer{"risk": {Type: systemOneTypeChoice, Choice: "HIGH"}},
			Usage:   systemOneUsage{InputTokens: 1, OutputTokens: 2},
		})
	})

	got, err := tr.SystemOne(context.Background(), systemOneRequest{Model: "nimble"})
	if err != nil {
		t.Fatalf("SystemOne() error = %v", err)
	}
	if got.Model != "nimble" || got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("response = %+v", got)
	}
}

func TestHTTPTransportSystemOneHTTPFailure(t *testing.T) {
	_, tr := systemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	})

	_, err := tr.SystemOne(context.Background(), systemOneRequest{})
	var status *httpStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want httpStatusError 500", err)
	}
	// A backend 5xx that is not a connectivity problem must not be classified
	// as unavailable.
	if isUnavailable(err) {
		t.Error("isUnavailable() = true, want false for backend status error")
	}
}

func TestHTTPTransportSystemOneMalformedBody(t *testing.T) {
	_, tr := systemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not json")
	})

	_, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if !isMalformedResponse(err) {
		t.Fatalf("error = %v, want malformedResponseError", err)
	}
	if isUnavailable(err) {
		t.Error("isUnavailable() = true, want false for malformed response")
	}
}

func TestHTTPTransportSystemOneContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.URL, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tr.SystemOne(ctx, systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want cancellation error")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false for cancelled request, error = %v", err)
	}
}

func TestHTTPTransportSystemOneTimeout(t *testing.T) {
	// The handler briefly delays so Close can complete once the client has
	// already given up; the client timeout fires first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.URL, 20*time.Millisecond)
	_, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want timeout")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false for timeout, error = %v", err)
	}
}

func TestHTTPTransportListModels(t *testing.T) {
	_, tr := systemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %q, want /api/tags", r.URL.Path)
		}
		io.WriteString(w, `{"models":[{"name":"nimble:latest"},{"name":"llama3"}]}`)
	})

	names, err := tr.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(names) != 2 || names[0] != "nimble:latest" {
		t.Errorf("names = %v", names)
	}
}

func TestHTTPTransportConnectionRefusedIsUnavailable(t *testing.T) {
	// Point at a closed port to force a connection error.
	tr := NewHTTPTransport("http://127.0.0.1:1", 200*time.Millisecond)
	_, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want connection error")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false, error = %v", err)
	}
	if got := classifyTransportError(err); got != decision.KindUnavailable {
		t.Errorf("classifyTransportError() = %q, want %q", got, decision.KindUnavailable)
	}
}

func TestClassifyTransportError(t *testing.T) {
	if got := classifyTransportError(&malformedResponseError{Err: errors.New("x")}); got != decision.KindMalformedResponse {
		t.Errorf("malformed: got %q", got)
	}
	if got := classifyTransportError(context.DeadlineExceeded); got != decision.KindUnavailable {
		t.Errorf("deadline: got %q", got)
	}
	if got := classifyTransportError(errors.New("weird backend")); got != decision.KindProviderFailure {
		t.Errorf("generic: got %q", got)
	}
	if got := classifyTransportError(&httpStatusError{StatusCode: 500, Body: strings.TrimSpace("oops")}); got != decision.KindProviderFailure {
		t.Errorf("status: got %q", got)
	}
}
