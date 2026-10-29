package notify

import (
	"errors"
	"testing"
	"time"
)

// fakeOutbox is a minimal durable-outbox stand-in for the worker tests. It keeps
// per-delivery state and models claim/outcome the way the store does, so a fresh
// Drainer over the same fake simulates a fresh process.
type fakeOutbox struct {
	deliveries map[string]*DeliveryRecord
	tokens     int
	recordErr  error
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{deliveries: map[string]*DeliveryRecord{}}
}

func (f *fakeOutbox) add(eventID, channel string, maxAttempts int, next time.Time) {
	f.deliveries[eventID+"|"+channel] = &DeliveryRecord{
		EventID:       eventID,
		Channel:       channel,
		Status:        StatusPending,
		MaxAttempts:   maxAttempts,
		NextAttemptAt: next,
	}
}

func (f *fakeOutbox) get(eventID, channel string) *DeliveryRecord {
	return f.deliveries[eventID+"|"+channel]
}

func (f *fakeOutbox) ClaimDue(limit int, now time.Time) ([]DeliveryRecord, error) {
	var out []DeliveryRecord
	for _, d := range f.deliveries {
		if len(out) >= limit {
			break
		}
		if d.Status == StatusDelivered || d.Status == StatusExhausted {
			continue
		}
		if d.NextAttemptAt.After(now) {
			continue
		}
		f.tokens++
		d.ClaimToken = "tok" + time.Duration(f.tokens).String()
		d.Status = StatusClaimed
		out = append(out, *d)
	}
	return out, nil
}

func (f *fakeOutbox) RecordOutcome(token, result, lastError string, nextAttemptAt, now time.Time) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	for _, d := range f.deliveries {
		if d.ClaimToken != token || d.ClaimToken == "" {
			continue
		}
		d.Attempt++
		d.LastError = lastError
		d.NextAttemptAt = nextAttemptAt
		d.ClaimToken = ""
		switch result {
		case ResultDelivered:
			d.Status = StatusDelivered
		case ResultExhausted:
			d.Status = StatusExhausted
		default:
			if d.Attempt >= d.MaxAttempts {
				d.Status = StatusExhausted
			} else {
				d.Status = StatusPending
			}
		}
		return nil
	}
	return errors.New("stale claim")
}

// stubChannel returns a configured ack and error per call.
type stubChannel struct {
	acks  []Ack
	errs  []error
	calls int
}

func (s *stubChannel) Send(d Delivery) (Ack, error) {
	i := s.calls
	s.calls++
	var a Ack = AckConfirmed
	if i < len(s.acks) {
		a = s.acks[i]
	}
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return a, err
}

func (s *stubChannel) Capabilities() Capabilities { return Capabilities{} }

func TestBackoffGrowsAndCaps(t *testing.T) {
	b := Backoff{BaseDelay: 100 * time.Millisecond, MaxDelay: 1000 * time.Millisecond}
	// Delay(1) is the base; each later attempt doubles it, capped at the max.
	want := []time.Duration{100, 200, 400, 800, 1000, 1000}
	for i, w := range want {
		if got := b.Delay(i + 1); got != w*time.Millisecond {
			t.Fatalf("attempt %d: got %v want %v", i+1, got, w*time.Millisecond)
		}
	}
	// A non-positive or zero attempt index floors to the base delay.
	if got := b.Delay(0); got != time.Millisecond*100 {
		t.Fatalf("attempt 0: got %v want %v", got, time.Millisecond*100)
	}
	prev := time.Duration(0)
	for i := 1; i <= 10; i++ {
		d := b.Delay(i)
		if d > b.MaxDelay {
			t.Fatalf("attempt %d: delay %v exceeds cap", i, d)
		}
		if i > 1 && d < prev {
			t.Fatalf("attempt %d: delay decreased", i)
		}
		prev = d
	}
}

func TestBackoffDefaultsWhenUnset(t *testing.T) {
	b := Backoff{}
	if got := b.Delay(1); got != DefaultBaseDelay {
		t.Fatalf("got %v want base default %v", got, DefaultBaseDelay)
	}
	for i := 1; i <= 40; i++ {
		got := b.Delay(i)
		if got > DefaultMaxDelay {
			t.Fatalf("attempt %d: delay %v exceeds default cap", i, got)
		}
	}
}

func TestBackoffNonPositiveBoundsFallBack(t *testing.T) {
	// A non-positive base and max fall back to the defaults, so the delay never
	// becomes zero or unbounded.
	b := Backoff{BaseDelay: -1, MaxDelay: -1}
	if got := b.Delay(1); got != DefaultBaseDelay {
		t.Fatalf("got %v want %v", got, DefaultBaseDelay)
	}
	if got := b.Delay(100); got != DefaultMaxDelay {
		t.Fatalf("got %v want %v", got, DefaultMaxDelay)
	}
	// A max smaller than the base is raised to the base.
	c := Backoff{BaseDelay: time.Second, MaxDelay: time.Millisecond}
	if got := c.Delay(1); got != time.Second {
		t.Fatalf("got %v want %v", got, time.Second)
	}
}

func TestDrainUnavailableChannel(t *testing.T) {
	d := &Drainer{Outbox: newFakeOutbox(), Channel: nil}
	if _, err := d.Drain(10); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("got %v want ErrChannelUnavailable", err)
	}
}

func TestDrainNilOutbox(t *testing.T) {
	d := &Drainer{Channel: &stubChannel{}}
	if _, err := d.Drain(10); err == nil {
		t.Fatal("expected an error when the outbox is missing")
	}
}

func TestDrainFailurePersistsAndFreshProcessRetries(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "file", 3, base)
	ch := &stubChannel{acks: []Ack{AckAmbiguous}, errs: []error{errors.New("timeout")}}
	clock := base
	d := &Drainer{Outbox: ob, Channel: ch, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: 10 * time.Second}, Now: func() time.Time { return clock }}

	report, err := d.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Retried != 1 {
		t.Fatalf("got %+v want one retry", report)
	}
	rec := ob.get("evt", "file")
	if rec.Attempt != 1 || rec.LastError == "" || !rec.NextAttemptAt.After(base) {
		t.Fatalf("persisted state wrong: %+v", rec)
	}
	// The first retry waits exactly one base interval (Delay(1)).
	if got, want := rec.NextAttemptAt, base.Add(time.Second); !got.Equal(want) {
		t.Fatalf("next attempt = %v, want %v", got, want)
	}

	// A fresh process before the retry time must not claim the delivery.
	fresh := &Drainer{Outbox: ob, Channel: ch, Now: func() time.Time { return clock }}
	if r, _ := fresh.Drain(10); r.Claimed != 0 {
		t.Fatalf("claimed before retry time: %+v", r)
	}

	// After the retry time, a fresh process drains it and backs off further.
	clock = rec.NextAttemptAt
	freshCh := &stubChannel{acks: []Ack{AckAmbiguous}, errs: []error{errors.New("timeout")}}
	fresh = &Drainer{Outbox: ob, Channel: freshCh, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: 10 * time.Second}, Now: func() time.Time { return clock }}
	r, err := fresh.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retried != 1 {
		t.Fatalf("got %+v want one retry", r)
	}
	// The second retry uses Delay(2), twice the persisted attempt-1 delay.
	if got, want := ob.get("evt", "file").NextAttemptAt, clock.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("second next attempt = %v, want %v", got, want)
	}

	// A third fresh process, after the second retry time, delivers it.
	clock = ob.get("evt", "file").NextAttemptAt
	done := &Drainer{Outbox: ob, Channel: &stubChannel{acks: []Ack{AckConfirmed}}, Now: func() time.Time { return clock }}
	r, err = done.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delivered != 1 {
		t.Fatalf("got %+v want one delivery", r)
	}
	if got := ob.get("evt", "file"); got.Status != StatusDelivered {
		t.Fatalf("got status %s want DELIVERED", got.Status)
	}
}

func TestDrainExhaustsAndStopsRetrying(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "webhook", 2, base)
	clock := base
	ch := &stubChannel{acks: []Ack{AckAmbiguous, AckAmbiguous, AckAmbiguous}, errs: []error{errors.New("boom"), errors.New("boom"), errors.New("boom")}}
	d := &Drainer{Outbox: ob, Channel: ch, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: time.Second}, Now: func() time.Time { return clock }}

	for i := 0; i < 5; i++ {
		clock = clock.Add(time.Hour)
		if _, err := d.Drain(10); err != nil {
			t.Fatal(err)
		}
	}
	rec := ob.get("evt", "webhook")
	if rec.Status != StatusExhausted {
		t.Fatalf("got status %s want EXHAUSTED", rec.Status)
	}
	if rec.Attempt != 2 {
		t.Fatalf("got %d attempts want 2 (bounded)", rec.Attempt)
	}
	if rec.LastError == "" {
		t.Fatal("expected last error recorded")
	}
}

func TestDrainExhaustsOnTheAttemptThatReachesTheBound(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "webhook", 3, base)
	clock := base
	ch := &stubChannel{acks: []Ack{AckAmbiguous, AckAmbiguous, AckAmbiguous}}
	d := &Drainer{Outbox: ob, Channel: ch, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: time.Second}, Now: func() time.Time { return clock }}

	// Attempt 1 and attempt 2 retry; attempt 3 reaches the bound and the worker
	// itself reports EXHAUSTED on that pass.
	for i := 1; i <= 2; i++ {
		r, err := d.Drain(10)
		if err != nil {
			t.Fatal(err)
		}
		if r.Retried != 1 || r.Exhausted != 0 {
			t.Fatalf("attempt %d report = %+v, want one retry", i, r)
		}
		clock = ob.get("evt", "webhook").NextAttemptAt
	}
	r, err := d.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Exhausted != 1 {
		t.Fatalf("attempt 3 report = %+v, want one exhausted", r)
	}
	rec := ob.get("evt", "webhook")
	if rec.Status != StatusExhausted || rec.Attempt != 3 || rec.LastError == "" {
		t.Fatalf("got %+v want EXHAUSTED attempt 3 with last error", rec)
	}
}

func TestDrainRejectedIsTerminal(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "webhook", 5, base)
	ch := &stubChannel{acks: []Ack{AckRejected}}
	d := &Drainer{Outbox: ob, Channel: ch, Now: func() time.Time { return base }}
	r, err := d.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Exhausted != 1 {
		t.Fatalf("got %+v want exhausted", r)
	}
	if got := ob.get("evt", "webhook"); got.Status != StatusExhausted || got.LastError == "" {
		t.Fatalf("got %+v want EXHAUSTED with last error", got)
	}
}

func TestDrainSendErrorIsAmbiguous(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "webhook", 3, base)
	ch := &stubChannel{errs: []error{errors.New("connection refused")}}
	d := &Drainer{Outbox: ob, Channel: ch, Now: func() time.Time { return base }}
	r, err := d.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retried != 1 {
		t.Fatalf("got %+v want one retry", r)
	}
	if r.Results[0].Ack != AckAmbiguous {
		t.Fatalf("ack = %q want %q", r.Results[0].Ack, AckAmbiguous)
	}
}

func TestDrainEmptyEventID(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("", "file", 3, base)
	d := &Drainer{Outbox: ob, Channel: &stubChannel{}, Now: func() time.Time { return base }}
	r, err := d.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Exhausted != 1 {
		t.Fatalf("got %+v want exhausted for an empty event id", r)
	}
}

func TestDrainReportsRecordOutcomeError(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ob := newFakeOutbox()
	ob.add("evt", "file", 3, base)
	ob.recordErr = errors.New("write failed")
	d := &Drainer{Outbox: ob, Channel: &stubChannel{}, Now: func() time.Time { return base }}
	r, err := d.Drain(10)
	if err == nil {
		t.Fatal("expected a durable-write error to be reported")
	}
	if r.Delivered != 0 || r.Retried != 0 || r.Exhausted != 0 {
		t.Fatalf("a failed write must not be counted: %+v", r)
	}
}

func TestDrainUsesDeliveredEventBody(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	e, err := NewEvent(completedParams())
	if err != nil {
		t.Fatal(err)
	}
	ob := newFakeOutbox()
	ob.add(e.ID, "file", 3, base)
	ob.get(e.ID, "file").Event = &e

	var sent Event
	ch := &capturingChannel{onSend: func(v Event) { sent = v }}
	d := &Drainer{Outbox: ob, Channel: ch, Now: func() time.Time { return base }}
	if r, err := d.Drain(10); err != nil || r.Delivered != 1 {
		t.Fatalf("got %+v %v", r, err)
	}
	if sent.Kind != e.Kind || sent.Project != e.Project || sent.Outcome != e.Outcome {
		t.Fatalf("sent event lost its payload: %+v", sent)
	}
}

// capturingChannel records the last event it received and confirms delivery.
type capturingChannel struct {
	onSend func(Event)
}

func (c *capturingChannel) Send(d Delivery) (Ack, error) {
	if c.onSend != nil {
		c.onSend(d.Event)
	}
	return AckConfirmed, nil
}

func (c *capturingChannel) Capabilities() Capabilities { return Capabilities{} }

func TestSendTest(t *testing.T) {
	if _, err := SendTest(nil, Event{ID: "x"}); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("got %v want ErrChannelUnavailable", err)
	}
	e, err := NewTestEvent("proj")
	if err != nil {
		t.Fatal(err)
	}
	ack, err := SendTest(&stubChannel{acks: []Ack{AckConfirmed}}, e)
	if err != nil || ack != AckConfirmed {
		t.Fatalf("got %v %v", ack, err)
	}
}

// TestDrainSendsDeliveryEnvelope proves each attempt carries an explicit envelope:
// the event identity, a stable idempotency key across attempts, and the 1-based
// attempt number.
func TestDrainSendsDeliveryEnvelope(t *testing.T) {
	var seen []Delivery
	ch := &funcChannel{onSend: func(d Delivery) (Ack, error) {
		seen = append(seen, d)
		if len(seen) == 1 {
			return AckAmbiguous, errors.New("timeout")
		}
		return AckConfirmed, nil
	}}
	out := newFakeOutbox()
	out.add("evt-env", "log", 3, time.Time{})

	d := &Drainer{Outbox: out, Channel: ch, MaxAttempts: 3}
	if _, err := d.Drain(1); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("channel received no delivery")
	}
	first := seen[0]
	if first.Event.ID != "evt-env" {
		t.Errorf("event id = %q", first.Event.ID)
	}
	if first.Key() != "evt-env" {
		t.Errorf("idempotency key = %q, want the event id", first.Key())
	}
	if first.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", first.Attempt)
	}
}

// funcChannel adapts a send function to the Channel interface.
type funcChannel struct {
	onSend func(Delivery) (Ack, error)
}

func (c *funcChannel) Send(d Delivery) (Ack, error) {
	if c.onSend != nil {
		return c.onSend(d)
	}
	return AckConfirmed, nil
}

func (c *funcChannel) Capabilities() Capabilities {
	return Capabilities{KeyHonored: true, Guarantee: "test channel"}
}
