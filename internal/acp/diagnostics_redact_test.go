package acp

import (
	"strings"
	"testing"
)

// The authorization value is redacted and the scheme word is kept. Before the
// shared detector the key rule matched "authorization: Bearer" first, redacted
// the word "Bearer" and left the token itself in the diagnostic.
func TestRedactLineRedactsAuthorizationValue(t *testing.T) {
	got := redactLine("Authorization: Bearer abcdefghijkl")

	if strings.Contains(got, "abcdefghijkl") {
		t.Fatalf("bearer token leaked: %q", got)
	}
	if !strings.Contains(got, "Bearer") {
		t.Errorf("scheme word dropped: %q", got)
	}
	if !strings.Contains(got, redactedMarker) {
		t.Errorf("redaction not marked: %q", got)
	}
}

// The ACP keeps its own guard for long opaque blobs, which the other surfaces do
// not want: a commit SHA is not a credential.
func TestRedactLineRedactsOpaqueBlob(t *testing.T) {
	const blob = "0123456789abcdef0123456789abcdef"
	got := redactLine("trace " + blob + " done")

	if strings.Contains(got, blob) {
		t.Fatalf("opaque blob leaked: %q", got)
	}
	if !strings.Contains(got, redactedMarker) {
		t.Errorf("redaction not marked: %q", got)
	}
}
