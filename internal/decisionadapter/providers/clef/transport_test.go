package clef

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

// clefSystemOneServer stands up an httptest server and points an
// httpTransport (the production transport) at it. All transport tests use this
// seam so no real Ollama backend or model is ever contacted.
func clefSystemOneServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *httpTransport) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, newHTTPTransport(srv.URL, time.Second)
}

// TestHTTPTransportSystemOneSuccess proves the production transport decodes a
// well-formed SystemOne response from a fake server without a real model.
func TestHTTPTransportSystemOneSuccess(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(systemOneResponse{
			Model:   "clef-flash",
			Answers: map[string]systemOneAnswer{"risk": {Type: systemOneTypeChoice, Choice: "HIGH"}},
			Usage:   systemOneUsage{InputTokens: 1, OutputTokens: 2},
		})
	})

	got, err := tr.SystemOne(context.Background(), systemOneRequest{Model: "clef-flash"})
	if err != nil {
		t.Fatalf("SystemOne() error = %v", err)
	}
	if got.Model != "clef-flash" || got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("response = %+v", got)
	}
}

// TestHTTPTransportSystemOneHTTPFailure asserts a non-200 status surfaces as an
// httpStatusError and is never mistaken for a connectivity failure.
func TestHTTPTransportSystemOneHTTPFailure(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	})

	got, err := tr.SystemOne(context.Background(), systemOneRequest{})
	var status *httpStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want httpStatusError 500", err)
	}
	if strings.TrimSpace(status.Body) != "boom" {
		t.Errorf("status.Body = %q, want boom", status.Body)
	}
	// A backend 5xx that is not a connectivity problem must not be classified
	// as unavailable.
	if isUnavailable(err) {
		t.Error("isUnavailable() = true, want false for backend status error")
	}
	// No usable payload may be returned on failure.
	if got.Model != "" || got.Answers != nil {
		t.Errorf("response = %+v, want zero value on HTTP failure", got)
	}
}

// TestHTTPTransportSystemOneMalformedBody asserts an undecodable body is marked
// malformed and is not misread as connectivity loss.
func TestHTTPTransportSystemOneMalformedBody(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not json")
	})

	got, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if !isMalformedResponse(err) {
		t.Fatalf("error = %v, want malformedResponseError", err)
	}
	if isUnavailable(err) {
		t.Error("isUnavailable() = true, want false for malformed response")
	}
	if got.Model != "" || got.Answers != nil {
		t.Errorf("response = %+v, want zero value on malformed body", got)
	}
}

// TestHTTPTransportSystemOneEmptyBody asserts an empty body is treated as a
// malformed response (never an empty success).
func TestHTTPTransportSystemOneEmptyBody(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		// 200 with an empty body: nothing to decode.
	})

	got, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want malformed response for empty body")
	}
	if !isMalformedResponse(err) {
		t.Fatalf("error = %v, want malformedResponseError", err)
	}
	if got.Answers != nil {
		t.Errorf("response = %+v, want zero value on empty body", got)
	}
}

// TestHTTPTransportSystemOneContextCancellation asserts a cancelled context
// surfaces as an unavailable transport failure, not a success.
func TestHTTPTransportSystemOneContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.URL, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := tr.SystemOne(ctx, systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want cancellation error")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false for cancelled request, error = %v", err)
	}
	if got.Answers != nil {
		t.Errorf("response = %+v, want zero value on cancellation", got)
	}
}

// TestHTTPTransportSystemOneTimeout asserts a client timeout surfaces as an
// unavailable transport failure, not a success.
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

	tr := newHTTPTransport(srv.URL, 20*time.Millisecond)
	got, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want timeout")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false for timeout, error = %v", err)
	}
	if got.Answers != nil {
		t.Errorf("response = %+v, want zero value on timeout", got)
	}
}

// TestHTTPTransportListModels covers the availability probe: it reads model
// names from a fake server and never triggers an inference request.
func TestHTTPTransportListModels(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/tags" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"models":[{"name":"clef-flash:latest"},{"name":"llama3"}]}`)
	})

	names, err := tr.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(names) != 2 || names[0] != "clef-flash:latest" || names[1] != "llama3" {
		t.Errorf("names = %v", names)
	}
}

// TestHTTPTransportListModelsHTTPFailure asserts a non-200 status on the model
// listing is an httpStatusError, not a silent empty list.
func TestHTTPTransportListModelsHTTPFailure(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	names, err := tr.ListModels(context.Background())
	var status *httpStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("error = %v, want httpStatusError 503", err)
	}
	if names != nil {
		t.Errorf("names = %v, want nil on failure", names)
	}
}

// TestHTTPTransportListModelsMalformedBody asserts an undecodable model list is
// marked malformed.
func TestHTTPTransportListModelsMalformedBody(t *testing.T) {
	_, tr := clefSystemOneServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "{not json]")
	})

	if _, err := tr.ListModels(context.Background()); !isMalformedResponse(err) {
		t.Fatalf("error = %v, want malformedResponseError", err)
	}
}

// TestHTTPTransportConnectionRefusedIsUnavailable forces a connection error by
// pointing at a closed port.
func TestHTTPTransportConnectionRefusedIsUnavailable(t *testing.T) {
	tr := newHTTPTransport("http://127.0.0.1:1", 200*time.Millisecond)
	got, err := tr.SystemOne(context.Background(), systemOneRequest{})
	if err == nil {
		t.Fatal("SystemOne() error = nil, want connection error")
	}
	if !isUnavailable(err) {
		t.Fatalf("isUnavailable() = false, error = %v", err)
	}
	if got.Answers != nil {
		t.Errorf("response = %+v, want zero value when the connection is refused", got)
	}
	if kind := classifyTransportError(err); kind != decision.KindUnavailable {
		t.Errorf("classifyTransportError() = %q, want %q", kind, decision.KindUnavailable)
	}
}

// TestClassifyTransportError locks in the transport-failure -> ErrorKind
// mapping used by Provider.Decide.
func TestClassifyTransportError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want decision.ErrorKind
	}{
		"malformed response": {
			err:  &malformedResponseError{Err: errors.New("x")},
			want: decision.KindMalformedResponse,
		},
		"deadline exceeded": {
			err:  context.DeadlineExceeded,
			want: decision.KindUnavailable,
		},
		"context canceled": {
			err:  context.Canceled,
			want: decision.KindUnavailable,
		},
		"generic backend failure": {
			err:  errors.New("weird backend"),
			want: decision.KindProviderFailure,
		},
		"http status": {
			err:  &httpStatusError{StatusCode: 500, Body: strings.TrimSpace("oops")},
			want: decision.KindProviderFailure,
		},
		"nil error": {
			err:  nil,
			want: decision.KindProviderFailure,
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			if got := classifyTransportError(tt.err); got != tt.want {
				t.Errorf("classifyTransportError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsMalformedResponseAndIsUnavailableAreDisjoint asserts the two transport
// error predicates do not overlap, so each failure maps to exactly one kind.
func TestIsMalformedResponseAndIsUnavailableAreDisjoint(t *testing.T) {
	malformed := &malformedResponseError{Err: errors.New("decode")}
	if !isMalformedResponse(malformed) {
		t.Error("isMalformedResponse() = false, want true")
	}
	if isUnavailable(malformed) {
		t.Error("isUnavailable(malformed) = true, want false")
	}

	if isMalformedResponse(context.DeadlineExceeded) {
		t.Error("isMalformedResponse(deadline) = true, want false")
	}
	if !isUnavailable(context.DeadlineExceeded) {
		t.Error("isUnavailable(deadline) = false, want true")
	}

	if isUnavailable(nil) {
		t.Error("isUnavailable(nil) = true, want false")
	}
}
