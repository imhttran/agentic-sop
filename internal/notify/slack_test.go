package notify

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// bodyTransport is an injected transport that records the request body of each
// request so a test can assert the encoded payload. Tests never touch the
// network.
type bodyTransport struct {
	status int
	err    error
	keys   []string
	bodies [][]byte
}

func (b *bodyTransport) Do(req *http.Request) (*http.Response, error) {
	b.keys = append(b.keys, req.Header.Get(IdempotencyHeader))
	body, _ := io.ReadAll(req.Body)
	b.bodies = append(b.bodies, body)
	if b.err != nil {
		return nil, b.err
	}
	return &http.Response{
		StatusCode: b.status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func TestEncodeSlackPayloadHasTextAndEventID(t *testing.T) {
	e := mustEvent(t, completedParams())
	body, err := EncodeSlackPayload(e)
	if err != nil {
		t.Fatalf("EncodeSlackPayload: %v", err)
	}
	var msg struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if strings.TrimSpace(msg.Text) == "" {
		t.Fatal("payload has an empty top-level text field")
	}
	if !strings.Contains(msg.Text, e.ID) {
		t.Fatalf("text does not carry the event id %q: %q", e.ID, msg.Text)
	}
}

func TestWebhookNativePayloadUnchanged(t *testing.T) {
	e := mustEvent(t, completedParams())
	stub := &bodyTransport{status: 200}
	c := NewWebhookChannel("https://example.invalid/hook", stub)
	if _, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(stub.bodies) != 1 {
		t.Fatalf("recorded %d bodies, want 1", len(stub.bodies))
	}
	var got Event
	if err := json.Unmarshal(stub.bodies[0], &got); err != nil {
		t.Fatalf("native payload is not an event: %v", err)
	}
	if got.ID != e.ID || got.Kind != e.Kind {
		t.Fatalf("native payload changed: %+v", got)
	}
}

func TestWebhookSlackPayloadClassifiesResponses(t *testing.T) {
	e := mustEvent(t, completedParams())
	cases := []struct {
		name   string
		status int
		err    error
		want   Ack
	}{
		{"200 confirmed", 200, nil, AckConfirmed},
		{"400 rejected", 400, nil, AckRejected},
		{"500 ambiguous", 500, nil, AckAmbiguous},
		{"timeout ambiguous", 0, errors.New("context deadline exceeded"), AckAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &bodyTransport{status: tc.status, err: tc.err}
			c := NewWebhookChannel("https://example.invalid/hook", stub)
			c.Payload = PayloadSlack
			ack, _ := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
			if ack != tc.want {
				t.Fatalf("ack = %q, want %q", ack, tc.want)
			}
		})
	}
}

func TestWebhookSlackPayloadSendsIdempotencyKey(t *testing.T) {
	e := mustEvent(t, completedParams())
	stub := &bodyTransport{status: 200}
	c := NewWebhookChannel("https://example.invalid/hook", stub)
	c.Payload = "slack"
	if _, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(stub.keys) != 1 || stub.keys[0] != e.ID {
		t.Fatalf("idempotency keys = %v, want [%q]", stub.keys, e.ID)
	}
}

func TestWebhookSlackPayloadCarriesNoURL(t *testing.T) {
	e := mustEvent(t, completedParams())
	stub := &bodyTransport{status: 200}
	url := "https://hooks.example.invalid/services/SECRET-TOKEN"
	c := NewWebhookChannel(url, stub)
	c.Payload = PayloadSlack
	ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if err != nil || ack != AckConfirmed {
		t.Fatalf("ack=%q err=%v", ack, err)
	}
	if len(stub.bodies) != 1 {
		t.Fatalf("recorded %d bodies, want 1", len(stub.bodies))
	}
	if strings.Contains(string(stub.bodies[0]), "SECRET-TOKEN") {
		t.Fatalf("payload leaked the webhook URL: %s", stub.bodies[0])
	}
}

func TestNormalizePayloadStyle(t *testing.T) {
	cases := map[string]string{
		"":        PayloadNative,
		"native":  PayloadNative,
		" slack ": PayloadSlack,
		"SLACK":   PayloadSlack,
		"bogus":   "bogus",
	}
	for in, want := range cases {
		if got := NormalizePayloadStyle(in); got != want {
			t.Fatalf("NormalizePayloadStyle(%q) = %q, want %q", in, got, want)
		}
	}
	if !ValidPayloadStyle("") || !ValidPayloadStyle(PayloadNative) || !ValidPayloadStyle(PayloadSlack) {
		t.Fatal("a supported payload style was rejected")
	}
	if ValidPayloadStyle("blocks") {
		t.Fatal("an unknown payload style was accepted")
	}
}
