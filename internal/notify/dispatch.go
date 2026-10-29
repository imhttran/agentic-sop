// This file is the bounded, restart-safe delivery worker. It claims due
// deliveries from the durable outbox, sends each through the configured channel,
// maps the acknowledgement to a delivery result, and applies a bounded
// exponential backoff. All retry state lives in the outbox: no in-memory state
// survives a process, so a fresh process can drain a due delivery after its retry
// time passes.
package notify

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Default backoff bounds used when a caller supplies no policy.
const (
	DefaultBaseDelay   = 500 * time.Millisecond
	DefaultMaxDelay    = 30 * time.Second
	DefaultMaxAttempts = 3
	DefaultDrainLimit  = 50
)

// Delivery record statuses, mirroring the durable outbox vocabulary.
const (
	StatusPending   = "PENDING"
	StatusClaimed   = "CLAIMED"
	StatusDelivered = "DELIVERED"
	StatusExhausted = "EXHAUSTED"
)

// Delivery result values accepted by the outbox outcome recorder.
const (
	ResultDelivered = "DELIVERED"
	ResultRetry     = "RETRY"
	ResultExhausted = "EXHAUSTED"
)

// ErrChannelUnavailable reports that no usable channel is configured. A drain
// or a test send returns it explicitly instead of silently succeeding.
var ErrChannelUnavailable = errors.New("notify: no delivery channel is available")

// UnavailableChannelError names the channel that could not be built and why.
type UnavailableChannelError struct {
	Channel string
	Reason  string
}

func (e *UnavailableChannelError) Error() string {
	if e.Channel == "" {
		return fmt.Sprintf("notify: channel unavailable: %s", e.Reason)
	}
	return fmt.Sprintf("notify: channel %q unavailable: %s", e.Channel, e.Reason)
}

func (e *UnavailableChannelError) Unwrap() error { return ErrChannelUnavailable }

// DeliveryRecord is one due delivery the worker may attempt.
type DeliveryRecord struct {
	EventID       string
	Channel       string
	Status        string
	Attempt       int
	MaxAttempts   int
	LastError     string
	NextAttemptAt time.Time
	ClaimToken    string
	// Event is the optional event body to deliver. A producer that leaves it nil
	// supplies only the event id, and the worker rebuilds a minimal event from
	// EventID.
	Event *Event
}

// Outbox is the durable delivery state the worker reads and writes.
type Outbox interface {
	ClaimDue(limit int, now time.Time) ([]DeliveryRecord, error)
	RecordOutcome(token, result, lastError string, nextAttemptAt, now time.Time) error
}

// Backoff is the bounded exponential retry policy.
type Backoff struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

// DefaultBackoff returns the default bounded policy.
func DefaultBackoff() Backoff {
	return Backoff{BaseDelay: DefaultBaseDelay, MaxDelay: DefaultMaxDelay}
}

// resolved returns the policy with non-positive bounds replaced by the defaults
// and a maximum that is never smaller than the base.
func (b Backoff) resolved() Backoff {
	base, max := b.BaseDelay, b.MaxDelay
	if base <= 0 {
		base = DefaultBaseDelay
	}
	if max <= 0 {
		max = DefaultMaxDelay
	}
	if max < base {
		max = base
	}
	return Backoff{BaseDelay: base, MaxDelay: max}
}

// Delay returns the bounded delay before the next attempt after the given
// 1-based attempt count. Delay(1) is BaseDelay; each later attempt doubles it
// until the delay reaches MaxDelay, which it never exceeds.
func (b Backoff) Delay(attempt int) time.Duration {
	p := b.resolved()
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 32 {
		shift = 32
	}
	delay := p.BaseDelay
	for i := 0; i < shift; i++ {
		delay *= 2
		if delay >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

// Result is one delivery outcome in a drain report.
type Result struct {
	EventID string
	Channel string
	Ack     Ack
	Result  string
	Attempt int
	Error   string
}

// DrainReport summarizes one bounded drain pass.
type DrainReport struct {
	Claimed   int
	Delivered int
	Retried   int
	Exhausted int
	Results   []Result
}

// Drainer runs one bounded drain pass against the durable outbox.
type Drainer struct {
	Outbox      Outbox
	Channel     Channel
	Backoff     Backoff
	MaxAttempts int
	Now         func() time.Time
}

// backoffPolicy returns the backoff policy with defaults filled.
func (d *Drainer) backoffPolicy() Backoff {
	return d.Backoff.resolved()
}

func (d *Drainer) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

// maxAttempts resolves the attempt bound for one delivery: the row's own bound
// wins, then the drainer's, then the package default. The bound counts TOTAL
// attempts, including the first delivery: with a bound of 3, attempts 1 and 2
// retry and attempt 3 is the one that exhausts the delivery. The worker is the
// single owner of that decision; the store only records the attempt performed.
func (d *Drainer) maxAttempts(rec DeliveryRecord) int {
	if rec.MaxAttempts > 0 {
		return rec.MaxAttempts
	}
	if d.MaxAttempts > 0 {
		return d.MaxAttempts
	}
	return DefaultMaxAttempts
}

// Drain runs exactly ONE bounded pass: it claims up to limit due deliveries,
// sends each, and records the outcome. A nil outbox or an unavailable channel is
// reported explicitly instead of silently succeeding.
func (d *Drainer) Drain(limit int) (DrainReport, error) {
	var report DrainReport
	if d.Outbox == nil {
		return report, errors.New("notify: drain requires a delivery outbox")
	}
	if d.Channel == nil {
		return report, &UnavailableChannelError{Reason: "no channel is configured"}
	}
	if limit <= 0 {
		limit = DefaultDrainLimit
	}
	records, err := d.Outbox.ClaimDue(limit, d.now())
	if err != nil {
		return report, err
	}
	report.Claimed = len(records)
	for _, rec := range records {
		outcome, recErr := d.attempt(rec)
		report.Results = append(report.Results, outcome)
		if recErr != nil {
			// A failed durable write must not be counted as a delivered, retried, or
			// exhausted outcome: return the error so the caller sees it.
			return report, fmt.Errorf("notify: record outcome for event %q: %w", rec.EventID, recErr)
		}
		switch outcome.Result {
		case ResultDelivered:
			report.Delivered++
		case ResultExhausted:
			report.Exhausted++
		default:
			report.Retried++
		}
	}
	return report, nil
}

// attempt sends one claimed delivery and records its outcome. The returned
// Result describes what the worker decided; the error reports a failed durable
// write, which the caller must surface rather than count as a success.
func (d *Drainer) attempt(rec DeliveryRecord) (Result, error) {
	// attemptNumber is the 1-based count of the attempt this pass performs: the
	// attempts already persisted plus this one.
	attemptNumber := rec.Attempt + 1
	out := Result{EventID: rec.EventID, Channel: rec.Channel, Attempt: attemptNumber}

	e, ack, ackErr := d.send(rec, attemptNumber)
	if e == nil {
		out.Result = ResultExhausted
		out.Error = "delivery event could not be rebuilt"
		if ackErr != nil {
			out.Error = "delivery event could not be rebuilt: " + ackErr.Error()
		}
		return out, d.record(rec, ResultExhausted, out.Error, attemptNumber)
	}

	max := d.maxAttempts(rec)

	if ackErr != nil {
		// A send error is an unknown outcome: never a confirmed delivery and never
		// a definitive refusal.
		out.Ack = AckAmbiguous
		out.Error = ackErr.Error()
		return d.retryOrExhaust(rec, out, attemptNumber, max)
	}

	switch ack {
	case AckConfirmed:
		out.Ack = AckConfirmed
		out.Result = ResultDelivered
		return out, d.record(rec, ResultDelivered, "", attemptNumber)
	case AckRejected:
		out.Ack = AckRejected
		out.Result = ResultExhausted
		out.Error = "receiving channel rejected the delivery"
		return out, d.record(rec, ResultExhausted, out.Error, attemptNumber)
	default:
		out.Ack = AckAmbiguous
		out.Error = "delivery outcome is ambiguous"
		return d.retryOrExhaust(rec, out, attemptNumber, max)
	}
}

// retryOrExhaust records a retry when the attempt budget remains and marks the
// delivery EXHAUSTED when this attempt spent it. The bound is enforced on the
// attempt count, so the worker itself flips a spent delivery to EXHAUSTED on the
// attempt that reaches the limit.
func (d *Drainer) retryOrExhaust(rec DeliveryRecord, out Result, attemptNumber, max int) (Result, error) {
	if attemptNumber >= max {
		out.Result = ResultExhausted
		return out, d.record(rec, ResultExhausted, out.Error, attemptNumber)
	}
	out.Result = ResultRetry
	return out, d.record(rec, ResultRetry, out.Error, attemptNumber)
}

// send rebuilds the event to deliver and sends it through the channel. It
// returns a nil event when the delivery cannot be rebuilt, so the caller marks it
// EXHAUSTED rather than sending an invalid payload.
func (d *Drainer) send(rec DeliveryRecord, attemptNumber int) (*Event, Ack, error) {
	if d.Channel == nil {
		return nil, "", &UnavailableChannelError{Reason: "no channel is configured"}
	}
	e := Event{ID: rec.EventID}
	if rec.Event != nil {
		e = *rec.Event
	}
	// The envelope carries the event identity, the stable idempotency key for this
	// event, and the attempt number, so an adapter never has to infer the key from
	// the payload it happens to be handed.
	env := Delivery{Event: e, IdempotencyKey: e.ID, Attempt: attemptNumber}
	if err := env.Validate(); err != nil {
		return nil, "", err
	}
	ack, err := d.Channel.Send(env)
	return &e, ack, err
}

// record persists one outcome. The next attempt time uses the per-attempt
// exponential delay for the attempt this pass performed, so the delay strictly
// increases with the persisted attempt count up to the configured maximum.
func (d *Drainer) record(rec DeliveryRecord, result, lastError string, attemptNumber int) error {
	if d.Outbox == nil {
		return errors.New("notify: drain requires a delivery outbox")
	}
	now := d.now()
	next := now
	if result == ResultRetry {
		next = now.Add(d.backoffPolicy().Delay(attemptNumber))
	}
	return d.Outbox.RecordOutcome(rec.ClaimToken, result, lastError, next, now)
}

// SendTest delivers one synthetic test event through ch and returns its ack.
func SendTest(ch Channel, e Event) (Ack, error) {
	if ch == nil {
		return "", &UnavailableChannelError{Reason: "no channel is configured"}
	}
	env := Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}
	if err := env.Validate(); err != nil {
		return "", err
	}
	return ch.Send(env)
}

// NewTestEvent builds the synthetic test event sop notify test sends.
func NewTestEvent(project string) (Event, error) {
	return NewEvent(Params{
		Kind:       KindCompleted,
		Project:    project,
		Attempt:    1,
		Transition: "NOTIFY_TEST",
		Outcome:    "notification test event",
		NextAction: "no action required; this event confirms the notification channel is reachable",
		// Each explicit test invocation is its own event. Without a fresh nonce every
		// test produced the same event id, so the second and later runs deduplicated
		// onto the delivered first row and claimed nothing.
		Nonce: newTestNonce(),
	})
}

// newTestNonce returns a fresh identity discriminator for one test event. A retry
// of that delivery reuses the row's recorded event id, so the nonce only ever
// distinguishes one test invocation from the next.
func newTestNonce() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
