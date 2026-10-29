package notify

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubTransport is an injected transport that records each request and returns a
// canned response or error. Tests never touch the network.
type stubTransport struct {
	status int
	err    error
	// keys records the Idempotency-Key header of each request in order.
	keys []string
}

func (s *stubTransport) Do(req *http.Request) (*http.Response, error) {
	s.keys = append(s.keys, req.Header.Get(IdempotencyHeader))
	_, _ = io.Copy(io.Discard, req.Body)
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func TestWebhookSendsStableIdempotencyKey(t *testing.T) {
	e := mustEvent(t, completedParams())
	stub := &stubTransport{status: 204}
	c := NewWebhookChannel("https://example.invalid/hook", stub)

	for i := 0; i < 2; i++ {
		if ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil || ack != AckConfirmed {
			t.Fatalf("send %d: ack=%q err=%v", i, ack, err)
		}
	}
	if len(stub.keys) != 2 {
		t.Fatalf("recorded %d keys, want 2", len(stub.keys))
	}
	if stub.keys[0] != e.ID {
		t.Fatalf("first key = %q, want event id %q", stub.keys[0], e.ID)
	}
	if stub.keys[0] != stub.keys[1] {
		t.Fatalf("retry changed the idempotency key: %q != %q", stub.keys[0], stub.keys[1])
	}
}

func TestWebhookClassifiesResponses(t *testing.T) {
	e := mustEvent(t, completedParams())
	cases := []struct {
		name   string
		status int
		err    error
		want   Ack
	}{
		{"200 confirmed", 200, nil, AckConfirmed},
		{"204 confirmed", 204, nil, AckConfirmed},
		{"400 rejected", 400, nil, AckRejected},
		{"422 rejected", 422, nil, AckRejected},
		{"500 ambiguous", 500, nil, AckAmbiguous},
		{"503 ambiguous", 503, nil, AckAmbiguous},
		{"connection error ambiguous", 0, errors.New("connection refused"), AckAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubTransport{status: tc.status, err: tc.err}
			ack, _ := NewWebhookChannel("https://example.invalid/hook", stub).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
			if ack != tc.want {
				t.Fatalf("ack = %q, want %q", ack, tc.want)
			}
		})
	}
}

func TestWebhookRejectedIsDistinctFromAmbiguous(t *testing.T) {
	e := mustEvent(t, completedParams())
	rejected, _ := NewWebhookChannel("https://example.invalid/hook", &stubTransport{status: 400}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	ambiguous, _ := NewWebhookChannel("https://example.invalid/hook", &stubTransport{status: 502}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if rejected == ambiguous {
		t.Fatalf("a definitive rejection (%q) must be distinguishable from an ambiguous outcome (%q)", rejected, ambiguous)
	}
	if rejected != AckRejected {
		t.Fatalf("rejected ack = %q, want %q", rejected, AckRejected)
	}
	if ambiguous != AckAmbiguous {
		t.Fatalf("ambiguous ack = %q, want %q", ambiguous, AckAmbiguous)
	}
}

func TestWebhookCapabilitiesDoNotClaimExactlyOnce(t *testing.T) {
	caps := NewWebhookChannel("https://example.invalid/hook", &stubTransport{status: 200}).Capabilities()
	if caps.KeyHonored {
		t.Fatal("webhook channel claims key honoring, but the receiver may not honour the key")
	}
	if caps.Guarantee == "" {
		t.Fatal("webhook channel makes no guarantee statement")
	}
	lower := strings.ToLower(caps.Guarantee)
	if strings.Contains(lower, "exactly-once") || strings.Contains(lower, "duplicate-free") {
		t.Fatalf("webhook guarantee claims duplicate-free or exactly-once delivery: %q", caps.Guarantee)
	}
}
