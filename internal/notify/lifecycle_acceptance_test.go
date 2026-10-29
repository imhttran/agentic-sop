package notify

import (
	"errors"
	"testing"
	"time"
)

// Acceptance mapping for NTFY-008 (issue scenario coverage, notify layer):
//
//	lifecycle: TestAcceptanceLifecycleAckToResultMapping drives the Drainer through
//	the confirmed/rejected/ambiguous acknowledgements and pins each Result/Ack pair.
//	read-only: TestAcceptanceReadOnlyChannelUnavailable proves an unconfigured
//	channel yields ErrChannelUnavailable and no send.
//	attempt: TestAcceptanceAttemptBoundRetriesThenExhausts proves attempts 1 and 2
//	retry and the bound attempt exhausts.
//	ambiguity: TestAcceptanceAmbiguousResultRecordsRetry proves AckAmbiguous and a
//	send error both record RETRY with a bounded backoff and the persisted attempt.
//	classification: TestAcceptanceClassificationOfAcks proves the ack vocabulary maps
//	to DELIVERED/EXHAUSTED/RETRY-or-EXHAUSTED.

// acceptanceBase is a fixed instant so every test is deterministic and uses only
// the injected clock.
func acceptanceBase() time.Time {
	return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
}

// countingChannel counts Send calls and replays a scripted ack per call. It never
// touches the network.
type countingChannel struct {
	acks  []Ack
	errs  []error
	calls int
}

func (c *countingChannel) Send(d Delivery) (Ack, error) {
	i := c.calls
	c.calls++
	a := AckConfirmed
	if i < len(c.acks) {
		a = c.acks[i]
	}
	var err error
	if i < len(c.errs) {
		err = c.errs[i]
	}
	return a, err
}

func (c *countingChannel) Capabilities() Capabilities { return Capabilities{} }

// TestAcceptanceLifecycleAckToResultMapping drives one delivery through confirmed,
// rejected, and ambiguous acks and asserts the Drainer's Result.Ack and Result.Result
// mapping for each. It fails if any classification regresses.
func TestAcceptanceLifecycleAckToResultMapping(t *testing.T) {
	base := acceptanceBase()
	cases := []struct {
		name      string
		ack       Ack
		wantAck   Ack
		wantRes   string
		wantState string
	}{
		{"confirmed", AckConfirmed, AckConfirmed, ResultDelivered, StatusDelivered},
		{"rejected", AckRejected, AckRejected, ResultExhausted, StatusExhausted},
		{"ambiguous", AckAmbiguous, AckAmbiguous, ResultRetry, StatusPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob := newFakeOutbox()
			ob.add("evt-life", "log", 5, base)
			ch := &countingChannel{acks: []Ack{tc.ack}}
			d := &Drainer{Outbox: ob, Channel: ch, Now: func() time.Time { return base }}
			report, err := d.Drain(10)
			if err != nil {
				t.Fatalf("Drain: %v", err)
			}
			if len(report.Results) != 1 {
				t.Fatalf("results = %d, want 1", len(report.Results))
			}
			got := report.Results[0]
			if got.Ack != tc.wantAck {
				t.Errorf("Ack = %q, want %q", got.Ack, tc.wantAck)
			}
			if got.Result != tc.wantRes {
				t.Errorf("Result = %q, want %q", got.Result, tc.wantRes)
			}
			if rec := ob.get("evt-life", "log"); rec.Status != tc.wantState {
				t.Errorf("persisted status = %q, want %q", rec.Status, tc.wantState)
			}
		})
	}
}

// TestAcceptanceReadOnlyChannelUnavailable proves a drain with no channel reports
// ErrChannelUnavailable and never calls Send: a read-only/unconfigured state must
// not attempt a delivery.
func TestAcceptanceReadOnlyChannelUnavailable(t *testing.T) {
	base := acceptanceBase()
	ob := newFakeOutbox()
	ob.add("evt-ro", "log", 3, base)
	d := &Drainer{Outbox: ob, Channel: nil, Now: func() time.Time { return base }}
	report, err := d.Drain(10)
	if !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("error = %v, want ErrChannelUnavailable", err)
	}
	var ue *UnavailableChannelError
	if !errors.As(err, &ue) {
		t.Fatalf("error = %v, want *UnavailableChannelError", err)
	}
	if report.Claimed != 0 || len(report.Results) != 0 {
		t.Fatalf("report = %+v, want no claimed deliveries and no results", report)
	}
	// The delivery was never touched: it is still PENDING with attempt 0.
	rec := ob.get("evt-ro", "log")
	if rec.Status != StatusPending || rec.Attempt != 0 {
		t.Fatalf("persisted state = %+v, want untouched PENDING attempt 0", rec)
	}
}

// TestAcceptanceAttemptBoundRetriesThenExhausts proves a bound of 3 lets attempts 1
// and 2 retry and exhausts on attempt 3, with the persisted attempt count matching.
func TestAcceptanceAttemptBoundRetriesThenExhausts(t *testing.T) {
	base := acceptanceBase()
	ob := newFakeOutbox()
	ob.add("evt-bound", "webhook", 3, base)
	clock := base
	ch := &countingChannel{acks: []Ack{AckAmbiguous, AckAmbiguous, AckAmbiguous}}
	d := &Drainer{Outbox: ob, Channel: ch, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: time.Second}, Now: func() time.Time { return clock }}

	for i := 1; i <= 2; i++ {
		r, err := d.Drain(10)
		if err != nil {
			t.Fatalf("drain %d: %v", i, err)
		}
		if r.Retried != 1 || r.Exhausted != 0 {
			t.Fatalf("attempt %d report = %+v, want one retry and no exhaustion", i, r)
		}
		if got := ob.get("evt-bound", "webhook"); got.Status != StatusPending {
			t.Fatalf("attempt %d persisted status = %q, want PENDING", i, got.Status)
		}
		clock = ob.get("evt-bound", "webhook").NextAttemptAt
	}
	r, err := d.Drain(10)
	if err != nil {
		t.Fatalf("drain 3: %v", err)
	}
	if r.Exhausted != 1 {
		t.Fatalf("attempt 3 report = %+v, want one exhausted", r)
	}
	rec := ob.get("evt-bound", "webhook")
	if rec.Status != StatusExhausted {
		t.Fatalf("persisted status = %q, want EXHAUSTED", rec.Status)
	}
	if rec.Attempt != 3 {
		t.Fatalf("persisted attempt = %d, want 3", rec.Attempt)
	}
	if ch.calls != 3 {
		t.Fatalf("Send calls = %d, want 3 (one per attempt)", ch.calls)
	}
}

// TestAcceptanceAmbiguousResultRecordsRetry proves both an AckAmbiguous return and a
// send error are ambiguous: Result.Ack is AckAmbiguous, the result is RETRY, and the
// next attempt is bounded by the backoff policy.
func TestAcceptanceAmbiguousResultRecordsRetry(t *testing.T) {
	base := acceptanceBase()
	cases := []struct {
		name string
		ch   *countingChannel
	}{
		{"ambiguous ack", &countingChannel{acks: []Ack{AckAmbiguous}}},
		{"send error", &countingChannel{errs: []error{errors.New("connection reset")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob := newFakeOutbox()
			ob.add("evt-amb", "webhook", 5, base)
			d := &Drainer{Outbox: ob, Channel: tc.ch, Backoff: Backoff{BaseDelay: time.Second, MaxDelay: 4 * time.Second}, Now: func() time.Time { return base }}
			r, err := d.Drain(10)
			if err != nil {
				t.Fatalf("Drain: %v", err)
			}
			if len(r.Results) != 1 || r.Results[0].Ack != AckAmbiguous {
				t.Fatalf("results = %+v, want one AckAmbiguous", r.Results)
			}
			if r.Results[0].Result != ResultRetry {
				t.Fatalf("result = %q, want %q", r.Results[0].Result, ResultRetry)
			}
			rec := ob.get("evt-amb", "webhook")
			if rec.Status != StatusPending {
				t.Fatalf("status = %q, want PENDING (retry)", rec.Status)
			}
			if rec.Attempt != 1 {
				t.Fatalf("persisted attempt = %d, want 1", rec.Attempt)
			}
			// Delay(1) is the base; the next attempt is base plus one base interval.
			if want := base.Add(time.Second); !rec.NextAttemptAt.Equal(want) {
				t.Fatalf("next attempt = %v, want %v (bounded backoff)", rec.NextAttemptAt, want)
			}
		})
	}
}

// TestAcceptanceClassificationOfAcks proves the ack vocabulary maps to exactly one
// outcome: confirmed to DELIVERED, rejected to EXHAUSTED, and ambiguous to RETRY
// while the budget remains or EXHAUSTED once it is spent.
func TestAcceptanceClassificationOfAcks(t *testing.T) {
	base := acceptanceBase()

	t.Run("confirmed is DELIVERED", func(t *testing.T) {
		ob := newFakeOutbox()
		ob.add("evt-c", "log", 1, base)
		d := &Drainer{Outbox: ob, Channel: &countingChannel{acks: []Ack{AckConfirmed}}, Now: func() time.Time { return base }}
		r, _ := d.Drain(10)
		if r.Delivered != 1 || r.Exhausted != 0 || r.Retried != 0 {
			t.Fatalf("report = %+v, want one delivered", r)
		}
	})

	t.Run("rejected is EXHAUSTED", func(t *testing.T) {
		ob := newFakeOutbox()
		ob.add("evt-r", "log", 5, base)
		d := &Drainer{Outbox: ob, Channel: &countingChannel{acks: []Ack{AckRejected}}, Now: func() time.Time { return base }}
		r, _ := d.Drain(10)
		if r.Exhausted != 1 {
			t.Fatalf("report = %+v, want one exhausted", r)
		}
	})

	t.Run("ambiguous is RETRY then EXHAUSTED at the bound", func(t *testing.T) {
		ob := newFakeOutbox()
		ob.add("evt-a", "log", 1, base)
		d := &Drainer{Outbox: ob, Channel: &countingChannel{acks: []Ack{AckAmbiguous}}, MaxAttempts: 1, Now: func() time.Time { return base }}
		r, _ := d.Drain(10)
		// With a bound of 1 the first ambiguous attempt spends the budget, so it is
		// EXHAUSTED rather than RETRY.
		if r.Exhausted != 1 {
			t.Fatalf("report = %+v, want one exhausted at the bound", r)
		}
		if got := ob.get("evt-a", "log"); got.Status != StatusExhausted {
			t.Fatalf("status = %q, want EXHAUSTED", got.Status)
		}
	})
}
