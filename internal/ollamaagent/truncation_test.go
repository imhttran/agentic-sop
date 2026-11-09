package ollamaagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// scriptedOllama serves scripted assistant content together with an explicit
// done_reason, and counts the calls it received. It is the deterministic
// fake-provider fixture for the in-process tool loop: no model is invoked.
type scriptedOllama struct {
	mu         sync.Mutex
	content    string
	doneReason string
	calls      int
}

func newScriptedOllama(t *testing.T, content, doneReason string) (*scriptedOllama, *httptest.Server) {
	t.Helper()
	f := &scriptedOllama{content: content, doneReason: doneReason}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.calls++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Message:    chatMessage{Role: "assistant", Content: f.content},
			DoneReason: f.doneReason,
		})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *scriptedOllama) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// TestTruncatedToolCallIsNotDispatched proves a turn the provider truncated at
// its output-token limit cannot dispatch a tool call, even when the partial
// content happens to parse as a complete call: the mutation is never performed
// and the invocation is reported as truncated rather than successful.
func TestTruncatedToolCallIsNotDispatched(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	const call = `{"tool":"create_file","args":{"path":"truncated.txt","content":"partial"}}`
	fake, srv := newScriptedOllama(t, call, "length")

	resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Generate returned a transport error instead of a structured failure: %v", err)
	}
	if !resp.Truncated {
		t.Errorf("Response.Truncated = false, want true; content=%s", resp.Content)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeFailed {
		t.Fatalf("outcome = %+v, want failed", resp.Outcome)
	}
	if !strings.Contains(resp.Outcome.Reason, "incomplete") {
		t.Errorf("reason = %q, want it to identify the response as incomplete", resp.Outcome.Reason)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "truncated.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("truncated turn dispatched a tool call: truncated.txt exists (stat err %v)", statErr)
	}
	if fake.count() != 1 {
		t.Errorf("provider calls = %d, want 1 (a deterministic truncation is not retried)", fake.count())
	}
}

// TestTruncatedFinalIsNotAcceptedAsSuccess proves a well-formed-looking success
// outcome is still rejected when the provider says the turn was cut off at the
// output-token limit: the model's claim never outranks the provider's stop
// reason.
func TestTruncatedFinalIsNotAcceptedAsSuccess(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	const final = `{"status":"completed","summary":"done","changes_expected":true}`
	_, srv := newScriptedOllama(t, final, "length")

	resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !resp.Truncated {
		t.Errorf("Response.Truncated = false, want true")
	}
	if resp.Outcome == nil || resp.Outcome.Status == agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want a non-completion for truncated output", resp.Outcome)
	}
}

// TestNormalDoneReasonStillCompletes is the regression guard: an ordinary
// completion reason keeps the existing behavior, and the truncation flag stays
// unset.
func TestNormalDoneReasonStillCompletes(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	const final = `{"status":"completed","summary":"done","changes_expected":false}`
	_, srv := newScriptedOllama(t, final, "stop")

	resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Truncated {
		t.Errorf("Response.Truncated = true for an ordinary stop")
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", resp.Outcome)
	}
}
