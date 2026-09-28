package ollamaagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// These tests cover the bounded, provider-boundary retry in ollamaClient.chat.
// They drive a scripted /api/chat server, so they need no live Ollama and no
// network access, and they set the retry delay to zero so a retry never turns
// into a timing-sensitive sleep.

const (
	retryEmptyBody = `{"message":{"role":"assistant","content":""}}`
	retryOKBody    = `{"message":{"role":"assistant","content":"ok"}}`
	// A native tool call with empty content: a valid turn, never errEmptyResponse.
	retryToolBody = `{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"a.go"}}}]}}`
)

// step is one scripted response. A zero Status means 200 OK. Once the script is
// exhausted the last step repeats, so "always 500" or "always empty" is easy to
// express.
type step struct {
	status int
	body   string
}

// retryFixture is a scripted /api/chat server that records every request, so
// retry behavior and request identity are observable.
type retryFixture struct {
	mu       sync.Mutex
	steps    []step
	calls    int
	requests []ollamaChatRequest
}

func newRetryFixture(t *testing.T, steps ...step) (*retryFixture, string) {
	t.Helper()
	f := &retryFixture{steps: steps}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req ollamaChatRequest
		_ = json.Unmarshal(body, &req)

		f.mu.Lock()
		i := f.calls
		f.calls++
		f.requests = append(f.requests, req)
		st := step{body: "{}"}
		switch {
		case len(f.steps) == 0:
		case i < len(f.steps):
			st = f.steps[i]
		default:
			st = f.steps[len(f.steps)-1]
		}
		f.mu.Unlock()

		if st.status != 0 && st.status != http.StatusOK {
			w.WriteHeader(st.status)
			_, _ = io.WriteString(w, st.body)
			return
		}
		_, _ = io.WriteString(w, st.body)
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *retryFixture) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *retryFixture) request(i int) ollamaChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

// newRetryClient builds a client pointed at url with retries but no wait, so the
// retry tests stay fast and deterministic.
func newRetryClient(url string) *ollamaClient {
	c := newOllamaClient(Config{BaseURL: url, Model: "test-model:latest", Timeout: 2 * time.Second})
	c.retryDelay = 0
	return c
}

var retryMessages = []chatMessage{{Role: "user", Content: "hi"}}

// TestChatRetriesEmptyThenSucceeds asserts an empty turn is retried and the
// following turn's content is returned.
func TestChatRetriesEmptyThenSucceeds(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody}, step{body: retryOKBody})
	c := newRetryClient(url)

	content, calls, err := c.chat(context.Background(), retryMessages)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if len(calls) != 0 {
		t.Errorf("tool calls = %d, want 0", len(calls))
	}
	if f.count() != 2 {
		t.Errorf("http requests = %d, want 2", f.count())
	}
	// The retry must resend the SAME request/messages.
	got, want := f.request(1).Messages, f.request(0).Messages
	if len(got) != len(want) {
		t.Fatalf("retry changed the message count: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("retry changed message %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestChatRetriesTwoEmptiesThenSucceeds asserts the budget allows two retries.
func TestChatRetriesTwoEmptiesThenSucceeds(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody}, step{body: retryEmptyBody}, step{body: retryOKBody})
	c := newRetryClient(url)

	content, _, err := c.chat(context.Background(), retryMessages)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 3 {
		t.Errorf("http requests = %d, want 3", f.count())
	}
}

// TestChatExhaustsEmptyRetries asserts a persistently empty provider is a bounded
// failure, not an unbounded loop.
func TestChatExhaustsEmptyRetries(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody}) // repeats
	c := newRetryClient(url)

	_, _, err := c.chat(context.Background(), retryMessages)
	if !errors.Is(err, errEmptyResponse) {
		t.Fatalf("err = %v, want errEmptyResponse", err)
	}
	if f.count() != maxProviderAttempts {
		t.Errorf("http requests = %d, want %d (initial + at most two retries)", f.count(), maxProviderAttempts)
	}
}

// TestChatRetriesHTTP500 asserts a 5xx is transient and retried.
func TestChatRetriesHTTP500(t *testing.T) {
	f, url := newRetryFixture(t,
		step{status: http.StatusInternalServerError, body: `{"error":"server error"}`},
		step{body: retryOKBody},
	)
	c := newRetryClient(url)

	content, _, err := c.chat(context.Background(), retryMessages)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 2 {
		t.Errorf("http requests = %d, want 2", f.count())
	}
}

// TestChatRetriesHTTP429 asserts rate limiting is transient and retried.
func TestChatRetriesHTTP429(t *testing.T) {
	f, url := newRetryFixture(t,
		step{status: http.StatusTooManyRequests, body: `{"error":"slow down"}`},
		step{body: retryOKBody},
	)
	c := newRetryClient(url)

	content, _, err := c.chat(context.Background(), retryMessages)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 2 {
		t.Errorf("http requests = %d, want 2", f.count())
	}
}

// TestChatDoesNotRetryDeterministicHTTPStatus asserts 400/401/403/404 are not
// retried: they are deterministic client/protocol errors.
func TestChatDoesNotRetryDeterministicHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		f, url := newRetryFixture(t, step{status: status, body: `{"error":"nope"}`})
		c := newRetryClient(url)

		_, _, err := c.chat(context.Background(), retryMessages)
		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		var httpErr *ollamaHTTPError
		if !errors.As(err, &httpErr) || httpErr.statusCode != status {
			t.Fatalf("status %d: err = %v, want an *ollamaHTTPError with that status", status, err)
		}
		if f.count() != 1 {
			t.Errorf("status %d: http requests = %d, want 1 (no retry)", status, f.count())
		}
	}
}

// TestChatDoesNotRetryMalformedJSON asserts a malformed successful response fails
// closed immediately rather than being retried.
func TestChatDoesNotRetryMalformedJSON(t *testing.T) {
	f, url := newRetryFixture(t, step{body: "not json"})
	c := newRetryClient(url)

	_, _, err := c.chat(context.Background(), retryMessages)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if f.count() != 1 {
		t.Errorf("http requests = %d, want 1 (no retry)", f.count())
	}
}

// TestChatDoesNotRetryCanceledContext asserts a canceled context stops
// immediately and is never retried.
func TestChatDoesNotRetryCanceledContext(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody})
	c := newRetryClient(url)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.chat(ctx, retryMessages)
	if err == nil {
		t.Fatal("expected an error for a canceled context")
	}
	if f.count() > 1 {
		t.Errorf("http requests = %d, want at most 1 (no retry after cancel)", f.count())
	}
}

// TestChatNativeToolCallIsNotRetried asserts a native tool call with empty content
// is a valid turn that must never be treated as an empty response.
func TestChatNativeToolCallIsNotRetried(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryToolBody})
	c := newRetryClient(url)

	content, calls, err := c.chat(context.Background(), retryMessages)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
	if len(calls) != 1 || calls[0].Name != "read_file" {
		t.Fatalf("calls = %+v, want one read_file call", calls)
	}
	if f.count() != 1 {
		t.Errorf("http requests = %d, want 1 (a tool call is a valid turn)", f.count())
	}
}

// TestChatProviderBenefitsFromProviderRetry asserts the JEV chat adapter reuses
// the same provider-boundary retry: an empty turn is retried there too.
func TestChatProviderBenefitsFromProviderRetry(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody}, step{body: retryOKBody})
	p := &chatProvider{client: newRetryClient(url)}

	content, err := p.Chat(context.Background(), "", "prompt")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 2 {
		t.Errorf("http requests = %d, want 2", f.count())
	}
}

// TestRetryableProviderErrorClassification pins the retry policy to typed errors
// rather than message parsing.
func TestRetryableProviderErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"empty response", errEmptyResponse, true},
		{"http 429", &ollamaHTTPError{statusCode: http.StatusTooManyRequests}, true},
		{"http 500", &ollamaHTTPError{statusCode: http.StatusInternalServerError}, true},
		{"http 503", &ollamaHTTPError{statusCode: http.StatusServiceUnavailable}, true},
		{"http 400", &ollamaHTTPError{statusCode: http.StatusBadRequest}, false},
		{"http 401", &ollamaHTTPError{statusCode: http.StatusUnauthorized}, false},
		{"http 403", &ollamaHTTPError{statusCode: http.StatusForbidden}, false},
		{"http 404", &ollamaHTTPError{statusCode: http.StatusNotFound}, false},
		{"provider timeout", &ollamaTimeoutError{timeout: time.Second}, true},
		{"transport", &ollamaTransportError{baseURL: "x", err: errors.New("connection reset")}, true},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"canceled wrapped in transport", &ollamaTransportError{baseURL: "x", err: context.Canceled}, false},
		{"deadline wrapped in transport", &ollamaTransportError{baseURL: "x", err: context.DeadlineExceeded}, false},
		{"plain parse error", errors.New("parse ollama response: bad"), false},
	}
	for _, tc := range cases {
		if got := retryableProviderError(tc.err); got != tc.want {
			t.Errorf("%s: retryableProviderError = %v, want %v", tc.name, got, tc.want)
		}
	}
}
