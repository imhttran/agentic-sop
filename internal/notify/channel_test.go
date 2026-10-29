package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustEvent builds a valid completed event from p or fails the test.
func mustEvent(t *testing.T, p Params) Event {
	t.Helper()
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return e
}

func TestAckValuesAreDistinctAndValid(t *testing.T) {
	values := []Ack{AckConfirmed, AckAmbiguous, AckRejected}
	for i, a := range values {
		if !a.Valid() {
			t.Fatalf("ack %q is not valid", a)
		}
		for j, other := range values {
			if i != j && a == other {
				t.Fatalf("ack values %q and %q are not distinct", a, other)
			}
		}
	}
	if Ack("bogus").Valid() {
		t.Fatal("unknown ack reported valid")
	}
	if Ack("").Valid() {
		t.Fatal("empty ack reported valid")
	}
}

// okTransport always answers 200.
type okTransport struct{}

func (okTransport) Do(req *http.Request) (*http.Response, error) {
	_, _ = io.Copy(io.Discard, req.Body)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
}

// TestChannelContract runs each adapter through one shared contract: every ack
// is a declared value, and a Rejected outcome is never confused with an
// Ambiguous one.
func TestChannelContract(t *testing.T) {
	e := mustEvent(t, completedParams())

	filePath := filepath.Join(t.TempDir(), "events.jsonl")
	channels := []struct {
		name string
		new  func() Channel
	}{
		{"file", func() Channel { return NewFileChannel(filePath) }},
		{"webhook", func() Channel { return NewWebhookChannel("https://example.invalid/hook", okTransport{}) }},
	}
	for _, tc := range channels {
		t.Run(tc.name, func(t *testing.T) {
			ack, _ := tc.new().Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
			if !ack.Valid() {
				t.Fatalf("adapter returned undeclared ack %q", ack)
			}
			if ack != AckConfirmed {
				t.Fatalf("ack = %q, want %q", ack, AckConfirmed)
			}
		})
	}
}

// TestRejectedNeverConflatedWithAmbiguous drives the rejecting and transient
// paths of each adapter and asserts the two outcomes stay distinct.
func TestRejectedNeverConflatedWithAmbiguous(t *testing.T) {
	e := mustEvent(t, completedParams())

	fileRejected, _ := NewFileChannel(t.TempDir()).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if fileRejected != AckRejected {
		t.Fatalf("file rejected ack = %q, want %q", fileRejected, AckRejected)
	}

	webRejected, _ := NewWebhookChannel("https://example.invalid/hook", &stubTransport{status: 400}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	webAmbiguous, _ := NewWebhookChannel("https://example.invalid/hook", &stubTransport{status: 500}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if webRejected == webAmbiguous {
		t.Fatalf("webhook rejection (%q) and ambiguity (%q) are conflated", webRejected, webAmbiguous)
	}
}

// TestDocumentedGuaranteeMatchesCapabilities asserts that the package's
// documented guarantee agrees with what each channel's Capabilities prove: only
// the file channel claims duplicate-free delivery; the webhook channel does not.
func TestDocumentedGuaranteeMatchesCapabilities(t *testing.T) {
	fileCaps := NewFileChannel(filepath.Join(t.TempDir(), "events.jsonl")).Capabilities()
	if !fileCaps.KeyHonored {
		t.Fatal("file channel must claim key honoring")
	}

	webCaps := NewWebhookChannel("https://example.invalid/hook", okTransport{}).Capabilities()
	if webCaps.KeyHonored {
		t.Fatal("webhook channel must not claim key honoring")
	}
	lower := strings.ToLower(webCaps.Guarantee)
	if strings.Contains(lower, "exactly-once") || strings.Contains(lower, "duplicate-free") {
		t.Fatalf("webhook guarantee claims duplicate-free delivery: %q", webCaps.Guarantee)
	}
}

// TestAmbiguousIsRecordedAsAmbiguous proves an ambiguous acknowledgement is
// surfaced as ambiguous rather than confirmed or rejected.
func TestAmbiguousIsRecordedAsAmbiguous(t *testing.T) {
	e := mustEvent(t, completedParams())
	ack, err := NewWebhookChannel("https://example.invalid/hook", &stubTransport{err: io.ErrUnexpectedEOF}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if ack != AckAmbiguous {
		t.Fatalf("ack = %q, want %q", ack, AckAmbiguous)
	}
	if err == nil {
		t.Fatal("an ambiguous outcome must carry an error describing the unknown transport result")
	}
}

// TestFileReceiverContentParseable asserts the local receiver's content decodes
// as the event payload.
func TestFileReceiverContentParseable(t *testing.T) {
	e := mustEvent(t, completedParams())
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if ack, err := NewFileChannel(path).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil || ack != AckConfirmed {
		t.Fatalf("Send: ack=%q err=%v", ack, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read receiver content: %v", err)
	}
	var got Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("receiver content is not the event payload: %v", err)
	}
	if got.ID != e.ID {
		t.Fatalf("receiver event id = %q, want %q", got.ID, e.ID)
	}
}
