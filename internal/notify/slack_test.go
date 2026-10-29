package notify

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// slackStub records each request body and returns a canned response or error.
type slackStub struct {
	status int
	err    error
	bodies [][]byte
	keys   []string
}

func (s *slackStub) Do(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	s.bodies = append(s.bodies, b)
	s.keys = append(s.keys, req.Header.Get(IdempotencyHeader))
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{StatusCode: s.status, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
}

const slackTestURL = "https://hooks.slack.test/services/T000/B000/SECRETSECRET"

func TestSlackPostsATextMessage(t *testing.T) {
	e := mustEvent(t, completedParams())
	stub := &slackStub{status: 200}
	c := NewSlackChannel(slackTestURL, stub)

	ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if err != nil || ack != AckConfirmed {
		t.Fatalf("ack=%q err=%v", ack, err)
	}
	var got map[string]string
	if err := json.Unmarshal(stub.bodies[0], &got); err != nil {
		t.Fatalf("body is not a JSON object of strings: %v", err)
	}
	if got["text"] == "" || len(got) != 1 {
		t.Fatalf("body = %v, want exactly a text field", got)
	}
	if !strings.Contains(got["text"], "completed") || !strings.Contains(got["text"], e.ID[:12]) {
		t.Errorf("text lacks the kind or the event id: %q", got["text"])
	}
	if stub.keys[0] != e.ID {
		t.Errorf("idempotency key = %q, want the event id", stub.keys[0])
	}
}

func TestSlackClassifiesResponses(t *testing.T) {
	e := mustEvent(t, completedParams())
	cases := []struct {
		name   string
		status int
		err    error
		want   Ack
	}{
		{"ok", 200, nil, AckConfirmed},
		{"client error", 400, nil, AckRejected},
		{"revoked webhook", 404, nil, AckRejected},
		{"server error", 503, nil, AckAmbiguous},
		{"transport error", 0, errors.New("connection reset"), AckAmbiguous},
		{"redirect", 302, nil, AckAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewSlackChannel(slackTestURL, &slackStub{status: tc.status, err: tc.err})
			ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
			if ack != tc.want {
				t.Fatalf("ack = %q, want %q (err=%v)", ack, tc.want, err)
			}
		})
	}
}

// TestSlackErrorsNeverContainTheWebhookURL proves a transport failure does not
// leak the URL: the standard library's URL error repeats it, and the Slack URL's
// path is the secret.
func TestSlackErrorsNeverContainTheWebhookURL(t *testing.T) {
	e := mustEvent(t, completedParams())
	cause := errors.New("dial tcp: i/o timeout")
	stub := &slackStub{err: &url.Error{Op: "Post", URL: slackTestURL, Err: cause}}
	ack, err := NewSlackChannel(slackTestURL, stub).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if ack != AckAmbiguous || err == nil {
		t.Fatalf("ack=%q err=%v", ack, err)
	}
	if strings.Contains(err.Error(), "SECRETSECRET") || strings.Contains(err.Error(), "hooks.slack.test") {
		t.Fatalf("error leaks the webhook URL: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("error lost its cause: %v", err)
	}

	for _, status := range []int{400, 500} {
		_, err := NewSlackChannel(slackTestURL, &slackStub{status: status}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
		if err == nil || strings.Contains(err.Error(), "SECRETSECRET") {
			t.Fatalf("status %d error = %v", status, err)
		}
	}
}

func TestSlackTextEscapesControlCharacters(t *testing.T) {
	p := completedParams()
	p.Outcome = "done <!channel> & <https://evil.example|click>"
	e := mustEvent(t, p)
	text := SlackText(e)
	if strings.Contains(text, "<!channel>") || strings.Contains(text, "<https://") {
		t.Fatalf("text keeps Slack control sequences: %q", text)
	}
	if !strings.Contains(text, "&lt;!channel&gt;") || !strings.Contains(text, "&amp;") {
		t.Errorf("text did not escape: %q", text)
	}
}

func TestSlackRequiresAURL(t *testing.T) {
	e := mustEvent(t, completedParams())
	ack, err := NewSlackChannel("  ", &slackStub{status: 200}).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if ack != AckRejected || err == nil {
		t.Fatalf("ack=%q err=%v", ack, err)
	}
}

func TestSlackDoesNotClaimDuplicateFreeDelivery(t *testing.T) {
	if SlackCapabilities().KeyHonored {
		t.Fatal("slack must not claim to honour the idempotency key")
	}
}
