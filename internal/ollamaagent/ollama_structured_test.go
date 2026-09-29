package ollamaagent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// These tests cover the schema-constrained chat path (chatStructured) shared by
// chatProvider.ChatStructured. They reuse the scripted retry fixture, so they need
// no live Ollama and no network access, and they set the retry delay to zero so a
// retry never turns into a timing-sensitive sleep.

// structuredSchema is a small JSON schema used to prove the schema travels in the
// request's "format" field.
var structuredSchema = json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}},"required":["status"]}`)

// compactSchema compacts a schema the same way the JSON encoder does, so a
// recorded RawMessage can be compared byte-for-byte.
func compactSchema(t *testing.T, schema json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, schema); err != nil {
		t.Fatalf("compact schema: %v", err)
	}
	return buf.String()
}

// TestChatStructuredSendsSchemaInFormat asserts a structured request carries the
// JSON schema in the request's "format" field (Ollama's schema-constrained mode),
// not the generic "json" string.
func TestChatStructuredSendsSchemaInFormat(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryOKBody})
	c := newRetryClient(url)

	content, _, err := c.chatStructured(context.Background(), retryMessages, structuredSchema)
	if err != nil {
		t.Fatalf("chatStructured: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 1 {
		t.Fatalf("http requests = %d, want 1", f.count())
	}
	got := string(f.request(0).Format)
	if got == `"json"` {
		t.Fatal("structured request used the generic json mode, want the schema")
	}
	if want := compactSchema(t, structuredSchema); got != want {
		t.Errorf("format = %s, want the schema %s", got, want)
	}
}

// TestChatStructuredEmptySchemaDegradesToJSONMode asserts an empty schema degrades
// to the generic json mode rather than sending a malformed format.
func TestChatStructuredEmptySchemaDegradesToJSONMode(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryOKBody})
	c := newRetryClient(url)

	if _, _, err := c.chatStructured(context.Background(), retryMessages, nil); err != nil {
		t.Fatalf("chatStructured: %v", err)
	}
	if got := string(f.request(0).Format); got != `"json"` {
		t.Errorf("format = %s, want json", got)
	}
}

// TestChatStructuredBenefitsFromProviderRetry asserts the structured path reuses
// the same provider-boundary retry as the generic path: a transient empty turn is
// retried there, not surfaced as a JEV-level failure.
func TestChatStructuredBenefitsFromProviderRetry(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryEmptyBody}, step{body: retryOKBody})
	c := newRetryClient(url)

	content, _, err := c.chatStructured(context.Background(), retryMessages, structuredSchema)
	if err != nil {
		t.Fatalf("chatStructured: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if f.count() != 2 {
		t.Errorf("http requests = %d, want 2", f.count())
	}
}

// TestChatStructuredDoesNotRetryDeterministicStatus asserts a 4xx other than 429
// is not retried on the structured path either.
func TestChatStructuredDoesNotRetryDeterministicStatus(t *testing.T) {
	f, url := newRetryFixture(t, step{status: http.StatusBadRequest, body: `{"error":"bad schema"}`})
	c := newRetryClient(url)

	if _, _, err := c.chatStructured(context.Background(), retryMessages, structuredSchema); err == nil {
		t.Fatal("expected an error")
	}
	if f.count() != 1 {
		t.Errorf("http requests = %d, want 1 (no retry)", f.count())
	}
}

// TestChatProviderChatStructuredSendsSchema asserts the JEV-facing provider's
// structured operation forwards the schema to Ollama's "format" field.
func TestChatProviderChatStructuredSendsSchema(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryOKBody})
	p := &chatProvider{client: newRetryClient(url)}

	content, err := p.ChatStructured(context.Background(), "", "prompt", structuredSchema)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q, want %q", content, "ok")
	}
	if got := string(f.request(0).Format); got != compactSchema(t, structuredSchema) {
		t.Errorf("format = %s, want the schema", got)
	}
}

// TestChatProviderChatKeepsJSONMode asserts the generic Chat entry point still
// serializes format as the "json" string after the structured path was added.
func TestChatProviderChatKeepsJSONMode(t *testing.T) {
	f, url := newRetryFixture(t, step{body: retryOKBody})
	p := &chatProvider{client: newRetryClient(url)}

	if _, err := p.Chat(context.Background(), "", "prompt"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := string(f.request(0).Format); got != `"json"` {
		t.Errorf("format = %s, want json", got)
	}
	if strings.Contains(string(f.request(0).Format), "properties") {
		t.Error("generic chat must not send a schema")
	}
}
