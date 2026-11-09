package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// truncationServer serves one fixed JSON body, so a provider adapter's handling
// of a stop/finish reason is exercised deterministically without a model.
func truncationServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOpenAICompatibleRejectsLengthFinishReason proves a response the provider
// marked as stopped at its output-token limit is never accepted as a complete
// answer, even when it carries content. The provider boundary reports a typed
// truncation error the lifecycle can route through its bounded recovery.
func TestOpenAICompatibleRejectsLengthFinishReason(t *testing.T) {
	srv := truncationServer(t, `{"choices":[{"message":{"role":"assistant","content":"partial answer"},"finish_reason":"length"}]}`)

	c, err := NewLlamaCpp(srv.URL, "m", "", time.Second)
	if err != nil {
		t.Fatalf("NewLlamaCpp: %v", err)
	}
	resp, err := c.Generate(context.Background(), validRequest(Review))
	if err == nil {
		t.Fatalf("Generate accepted a length-truncated response as complete: %+v", resp)
	}
	var truncated *TruncationError
	if !errors.As(err, &truncated) {
		t.Fatalf("error = %T %v, want *TruncationError", err, err)
	}
	if truncated.Provider != ProviderLlamaCpp || truncated.Reason != "length" {
		t.Errorf("truncation = %+v, want provider %q reason length", truncated, ProviderLlamaCpp)
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error = %q, want it to identify the response as incomplete", err)
	}
}

// TestOllamaRejectsLengthDoneReason is the native Ollama counterpart: done_reason
// "length" is an explicit truncation, not an ordinary completion.
func TestOllamaRejectsLengthDoneReason(t *testing.T) {
	srv := truncationServer(t, `{"message":{"role":"assistant","content":"partial answer"},"done_reason":"length"}`)

	o, err := NewOllama(srv.URL, "m", time.Second)
	if err != nil {
		t.Fatalf("NewOllama: %v", err)
	}
	resp, err := o.Generate(context.Background(), validRequest(Plan))
	if err == nil {
		t.Fatalf("Generate accepted a length-truncated response as complete: %+v", resp)
	}
	var truncated *TruncationError
	if !errors.As(err, &truncated) {
		t.Fatalf("error = %T %v, want *TruncationError", err, err)
	}
	if truncated.Provider != ProviderOllama || truncated.Reason != "length" {
		t.Errorf("truncation = %+v, want provider %q reason length", truncated, ProviderOllama)
	}
}

// TestNormalStopReasonsRemainAccepted guards requirement 6: an ordinary stop, an
// absent reason, and a non-length nonstandard reason are not reclassified as
// truncation.
func TestNormalStopReasonsRemainAccepted(t *testing.T) {
	t.Run("openai-compatible", func(t *testing.T) {
		for _, reason := range []string{"stop", "tool_calls", "content_filter", ""} {
			srv := truncationServer(t, `{"choices":[{"message":{"role":"assistant","content":"complete"},"finish_reason":"`+reason+`"}]}`)
			c, err := NewLlamaCpp(srv.URL, "m", "", time.Second)
			if err != nil {
				t.Fatalf("NewLlamaCpp: %v", err)
			}
			resp, err := c.Generate(context.Background(), validRequest(Review))
			if err != nil {
				t.Fatalf("finish_reason %q was rejected: %v", reason, err)
			}
			if resp.Content != "complete" {
				t.Errorf("finish_reason %q content = %q, want complete", reason, resp.Content)
			}
		}
	})

	t.Run("ollama", func(t *testing.T) {
		for _, reason := range []string{"stop", ""} {
			srv := truncationServer(t, `{"message":{"role":"assistant","content":"complete"},"done_reason":"`+reason+`"}`)
			o, err := NewOllama(srv.URL, "m", time.Second)
			if err != nil {
				t.Fatalf("NewOllama: %v", err)
			}
			resp, err := o.Generate(context.Background(), validRequest(Plan))
			if err != nil {
				t.Fatalf("done_reason %q was rejected: %v", reason, err)
			}
			if resp.Content != "complete" {
				t.Errorf("done_reason %q content = %q, want complete", reason, resp.Content)
			}
		}
	})
}
