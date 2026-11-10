package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// LLM08 notification-boundary checks.
//
// TestLLM08NotificationRedactsCredentialShapes proves that credential-shaped
// descriptive text (Reason, NextAction, Outcome) is replaced by the shared
// internal/redact detector inside NewEvent, so the persisted outbox row and every
// channel payload carry only the marker. The non-secret positive control proves the
// detector is scoped: ordinary prose still reaches the outbox and the channel
// unchanged. No new DLP layer is involved.

// llm08Secret is a synthetic credential-shaped value. It is not a real secret.
const llm08Secret = "sk-llm08-notify-canary-do-not-use"

// llm08PositiveControl is ordinary descriptive prose that must survive unchanged.
const llm08PositiveControl = "fix the failing test and rerun sop run"

// llm08FailedParams builds a failed event whose descriptive fields carry a secret
// when secret is true.
func llm08FailedParams(secret bool) Params {
	p := Params{
		Kind:       KindFailed,
		Project:    "demo",
		JobID:      "job-1",
		TaskID:     "task-1",
		Attempt:    1,
		Transition: "BLOCKED",
		Reason:     "validation failed",
		NextAction: llm08PositiveControl,
	}
	if secret {
		p.Reason = "validation failed because api_key=" + llm08Secret
		p.NextAction = "remove bearer " + llm08Secret + " and rerun"
	}
	return p
}

// TestLLM08NotificationRedactsCredentialShapes proves the event constructor
// redacts Reason, NextAction and Outcome before any outbox write or channel send.
func TestLLM08NotificationRedactsCredentialShapes(t *testing.T) {
	t.Run("completed outcome", func(t *testing.T) {
		p := completedParams()
		p.Outcome = "task completed with token " + llm08Secret
		p.NextAction = "api_key=" + llm08Secret
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		for field, v := range map[string]string{"outcome": e.Outcome, "next_action": e.NextAction} {
			if strings.Contains(v, llm08Secret) {
				t.Fatalf("%s leaked the secret: %q", field, v)
			}
			if !strings.Contains(v, redactionMarker) {
				t.Fatalf("%s was not marked as redacted: %q", field, v)
			}
		}
	})

	t.Run("failed reason", func(t *testing.T) {
		e, err := NewEvent(llm08FailedParams(true))
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if strings.Contains(e.Reason, llm08Secret) {
			t.Fatalf("reason leaked the secret: %q", e.Reason)
		}
		if strings.Contains(e.NextAction, llm08Secret) {
			t.Fatalf("next action leaked the secret: %q", e.NextAction)
		}
		if !strings.Contains(e.Reason, redactionMarker) || !strings.Contains(e.NextAction, redactionMarker) {
			t.Fatalf("descriptive fields were not marked as redacted: %+v", e)
		}
	})

	t.Run("waiting approval reason", func(t *testing.T) {
		appr := pendingApproval()
		appr.Reason = "destructive migration needs password=" + llm08Secret
		p := Params{
			Kind:              KindWaitingForHuman,
			Project:           "demo",
			TaskID:            "task-1",
			Attempt:           1,
			Transition:        "WAITING_FOR_HUMAN",
			Approval:          appr,
			ResolutionCommand: "sop approve appr-1 --approve",
		}
		e, err := NewEvent(p)
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if strings.Contains(e.Reason, llm08Secret) {
			t.Fatalf("approval reason leaked the secret: %q", e.Reason)
		}
	})
}

// TestLLM08PositiveControlDeliveredUnchanged proves the detector does not
// over-redact: ordinary prose is persisted and delivered exactly as written.
func TestLLM08PositiveControlDeliveredUnchanged(t *testing.T) {
	e, err := NewEvent(llm08FailedParams(false))
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.NextAction != llm08PositiveControl {
		t.Fatalf("positive control next action = %q, want %q", e.NextAction, llm08PositiveControl)
	}
	if e.Reason != "validation failed" {
		t.Fatalf("positive control reason = %q", e.Reason)
	}
	if strings.Contains(e.NextAction, redactionMarker) {
		t.Fatalf("positive control text was redacted: %q", e.NextAction)
	}

	out := newFakeOutbox()
	out.add(e.ID, "webhook", 3, time.Time{})
	out.get(e.ID, "webhook").Event = &e
	var sent Event
	ch := &capturingChannel{onSend: func(v Event) { sent = v }}
	d := &Drainer{Outbox: out, Channel: ch, Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	report, err := d.Drain(10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if report.Delivered != 1 {
		t.Fatalf("positive control was not delivered: %+v", report)
	}
	if sent.NextAction != llm08PositiveControl {
		t.Fatalf("delivered next action = %q, want %q", sent.NextAction, llm08PositiveControl)
	}
}

// TestLLM08OutboxRowOmitsSecret proves a credential-shaped reason never reaches
// the persisted outbox row and that the non-secret control still does.
func TestLLM08OutboxRowOmitsSecret(t *testing.T) {
	secretParams := llm08FailedParams(true)
	secretParams.Nonce = "llm08-outbox-secret"
	secretEvent, err := NewEvent(secretParams)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	controlParams := llm08FailedParams(false)
	controlParams.Nonce = "llm08-outbox-control"
	controlEvent, err := NewEvent(controlParams)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if secretEvent.ID == controlEvent.ID {
		t.Fatal("secret and control fixtures share an event identity; outbox rows would collide")
	}

	out := newFakeOutbox()
	out.add(secretEvent.ID, "file", 3, time.Time{})
	out.get(secretEvent.ID, "file").Event = &secretEvent
	out.add(controlEvent.ID, "file", 3, time.Time{})
	out.get(controlEvent.ID, "file").Event = &controlEvent

	ch := &stubChannel{acks: []Ack{AckConfirmed, AckConfirmed}}
	d := &Drainer{Outbox: out, Channel: ch, Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	if _, err := d.Drain(10); err != nil {
		t.Fatalf("drain: %v", err)
	}

	serialized, err := json.Marshal(out.get(secretEvent.ID, "file"))
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	if strings.Contains(string(serialized), llm08Secret) {
		t.Fatalf("persisted outbox row leaked the secret: %s", serialized)
	}
	controlRow, err := json.Marshal(out.get(controlEvent.ID, "file"))
	if err != nil {
		t.Fatalf("marshal control row: %v", err)
	}
	if !strings.Contains(string(controlRow), llm08PositiveControl) {
		t.Fatalf("control outbox row lost the positive control text: %s", controlRow)
	}
	if out.get(secretEvent.ID, "file").Status != StatusDelivered || out.get(controlEvent.ID, "file").Status != StatusDelivered {
		t.Fatal("delivery semantics changed: expected both deliveries confirmed")
	}
}

// TestLLM08SlackPayloadOmitsSecret proves the Slack payload built from the event
// carries only the marker, while the control text survives.
func TestLLM08SlackPayloadOmitsSecret(t *testing.T) {
	secretEvent, err := NewEvent(llm08FailedParams(true))
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	body, err := EncodeSlackPayload(secretEvent)
	if err != nil {
		t.Fatalf("EncodeSlackPayload: %v", err)
	}
	if strings.Contains(string(body), llm08Secret) {
		t.Fatalf("slack payload leaked the secret: %s", body)
	}
	if !strings.Contains(string(body), redactionMarker) {
		t.Fatalf("slack payload is not marked redacted: %s", body)
	}

	controlEvent, err := NewEvent(llm08FailedParams(false))
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	controlBody, err := EncodeSlackPayload(controlEvent)
	if err != nil {
		t.Fatalf("EncodeSlackPayload: %v", err)
	}
	if !strings.Contains(string(controlBody), llm08PositiveControl) {
		t.Fatalf("slack positive control text missing: %s", controlBody)
	}
}

// TestLLM08NativeWebhookPayloadOmitsSecret proves the native webhook body posted
// to a real HTTP receiver never carries the secret, while the positive control
// text arrives intact.
func TestLLM08NativeWebhookPayloadOmitsSecret(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	for _, secret := range []bool{true, false} {
		e, err := NewEvent(llm08FailedParams(secret))
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		ch := NewWebhookChannel(server.URL, server.Client())
		ack, err := ch.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
		if err != nil {
			t.Fatalf("webhook Send: %v", err)
		}
		if ack != AckConfirmed {
			t.Fatalf("ack = %q, want confirmed", ack)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("receiver saw %d bodies, want 2", len(bodies))
	}
	if strings.Contains(bodies[0], llm08Secret) {
		t.Fatalf("native webhook payload leaked the secret: %s", bodies[0])
	}
	if !strings.Contains(bodies[0], redactionMarker) {
		t.Fatalf("native webhook payload not marked redacted: %s", bodies[0])
	}
	if !strings.Contains(bodies[1], llm08PositiveControl) {
		t.Fatalf("native webhook positive control text missing: %s", bodies[1])
	}
}

// TestLLM08RedactionPreservesEventIdentity proves redacting descriptive text does
// not change the event id: the identity hash covers only identity fields.
func TestLLM08RedactionPreservesEventIdentity(t *testing.T) {
	base := llm08FailedParams(false)
	base.OccurredAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	control, err := NewEvent(base)
	if err != nil {
		t.Fatalf("NewEvent control: %v", err)
	}
	secret := llm08FailedParams(true)
	secret.OccurredAt = base.OccurredAt
	redacted, err := NewEvent(secret)
	if err != nil {
		t.Fatalf("NewEvent secret: %v", err)
	}
	if control.ID != redacted.ID {
		t.Fatalf("redaction changed the event id: %q != %q", control.ID, redacted.ID)
	}
}

// TestLLM08UsesSharedRedactDetector proves the notify package reuses
// internal/redact: a value the shared detector recognizes is redacted, and an
// ordinary sentence in the same field is not.
func TestLLM08UsesSharedRedactDetector(t *testing.T) {
	e, err := NewEvent(llm08FailedParams(true))
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if !strings.Contains(e.Reason, redactionMarker) {
		t.Fatalf("shared detector did not redact the assignment shape: %q", e.Reason)
	}
	if !strings.Contains(e.NextAction, "and rerun") {
		t.Fatalf("shared detector dropped non-secret prose from the same field: %q", e.NextAction)
	}
}
