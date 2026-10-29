// Package notify defines SOP's notification lifecycle event as a first-class
// domain value, derives a stable event identity from the transition it
// describes, and delivers one event through one configured channel behind a
// narrow adapter interface.
//
// # Idempotency contract
//
// Every adapter receives the event id (Event.ID) as the idempotency key for
// the delivery. A channel that honours the key must apply one event id at most
// once, so a retry of one event never creates a second record on the receiving
// side.
//
// Duplicate-free delivery may be claimed only for a channel whose Capabilities
// prove that it honours the key. A channel that cannot prove it must not claim
// duplicate-free delivery, and its package documentation must say so.
//
// # Acknowledgement vocabulary
//
// An adapter classifies each attempt into exactly one Ack value:
//
//   - Confirmed: the transport proved delivery. For a key-honouring channel this
//     also proves the event was applied exactly once for its event id.
//   - Ambiguous: the transport outcome is unknown (for example a timeout, a
//     connection error, or a server error). SOP must record an ambiguous
//     acknowledgement as ambiguous. It must not assume delivery and must not
//     treat the attempt as a definitive refusal.
//   - Rejected: the receiver definitively refused the delivery. A rejected
//     outcome is a known failure, distinguishable from a transient ambiguous
//     one: retrying the same payload cannot be expected to change a rejection.
package notify

import (
	"errors"
	"strings"
)

// Ack is the acknowledgement an adapter returns for one delivery attempt. The
// three values are exhaustive: an adapter never invents another value.
type Ack string

const (
	// AckConfirmed: the transport proved delivery. For a channel whose
	// Capabilities report KeyHonored, the record for the event id exists exactly
	// once.
	AckConfirmed Ack = "confirmed"
	// AckAmbiguous: the transport outcome is unknown. SOP must record the attempt
	// as ambiguous; it must not assume delivery. A retry is permitted, but only a
	// channel whose Capabilities report KeyHonored can guarantee that the retry
	// does not duplicate; for any other channel the caller must accept that the
	// same event may be applied more than once.
	AckAmbiguous Ack = "ambiguous"
	// AckRejected: the receiver definitively refused the delivery. A rejection is
	// distinguishable from an ambiguous outcome because it is a known, final
	// failure rather than an unknown one.
	AckRejected Ack = "rejected"
)

// acks is the accepted Ack value set, kept here so a bad value fails with a
// message that names the accepted values.
var acks = map[Ack]bool{
	AckConfirmed: true,
	AckAmbiguous: true,
	AckRejected:  true,
}

// Valid reports whether a is one of the documented acknowledgement values.
func (a Ack) Valid() bool { return acks[a] }

// AckStrings returns the accepted acknowledgement spellings in a stable order,
// for error messages and tests.
func AckStrings() []string {
	return []string{string(AckConfirmed), string(AckAmbiguous), string(AckRejected)}
}

// Capabilities states what a channel proves about delivery. It is a value, not
// a method, so a caller can record the claim a channel makes.
type Capabilities struct {
	// KeyHonored reports that the channel applies one event id at most once, so
	// a retry of one event cannot create a second record on the receiver. Only a
	// channel whose KeyHonored is true may claim duplicate-free delivery.
	KeyHonored bool
	// Guarantee is the human-readable delivery guarantee the channel makes. It
	// must agree with KeyHonored: a channel that does not honour the key must not
	// state an exactly-once or duplicate-free guarantee here.
	Guarantee string
}

// Delivery is the envelope one delivery attempt carries. It exists so an adapter
// receives the event identity and the delivery metadata together, rather than a
// bare event: the idempotency key must be explicit at the boundary, and the attempt
// number is part of what the receiver is being told.
//
// The idempotency key is stable for the whole life of one event: every attempt of
// the same event carries the same key, so a channel that honours it never applies
// the event twice. It defaults to the event id and is never empty for a valid
// delivery.
type Delivery struct {
	// Event is the lifecycle event payload.
	Event Event
	// IdempotencyKey is the key the receiver deduplicates on. It is set by the
	// worker for every attempt of one event.
	IdempotencyKey string
	// Attempt is the 1-based attempt number this envelope represents.
	Attempt int
}

// Key returns the idempotency key, falling back to the event id.
func (d Delivery) Key() string {
	if k := strings.TrimSpace(d.IdempotencyKey); k != "" {
		return k
	}
	return d.Event.ID
}

// Validate reports whether the envelope carries what a channel needs: an event
// identity and a non-empty idempotency key.
func (d Delivery) Validate() error {
	if strings.TrimSpace(d.Event.ID) == "" {
		return errors.New("notify: delivery has no event id")
	}
	if d.Key() == "" {
		return errors.New("notify: delivery has no idempotency key")
	}
	return nil
}

// Channel is the narrow delivery adapter: one delivery envelope in, one
// acknowledgement out. An adapter owns no retry scheduling and no mutable SOP
// state; the dispatch worker decides when to call it and records what it returns.
type Channel interface {
	// Send delivers one envelope and reports the acknowledgement of that attempt.
	// It sends Delivery.Key() as the idempotency key on every attempt.
	Send(d Delivery) (Ack, error)
	// Capabilities reports what the channel proves about delivery.
	Capabilities() Capabilities
}
