// Bounded outbound delivery for session/update frames (ACP-004).
//
// The delivery policy, in one place:
//
//   - The buffered frame count never exceeds EventBufferBound, whatever the
//     client does: a slow or stopped client cannot grow server memory without
//     limit.
//   - Intermediate progress coalesces. When the buffer is full, PushProgress
//     replaces the most recently buffered progress frame in place with the
//     newest snapshot, so the size stays at the bound, the newest progress
//     snapshot stays buffered, and a superseded older one leaves. Terminal
//     outcomes are never touched by this rule.
//   - A terminal outcome is preserved. PushOutcome never replaces an outcome;
//     when the buffer is full it drops the oldest progress frames to reserve a
//     slot, and only when no progress frame remains to drop does the push fail
//     with ErrEventBufferFull (so bounded memory is never relaxed).
//   - Every coalescing and dropping decision records an actionable diagnostic.
//   - PumpEvents drains the buffer in FIFO order through the single
//     protocolWriter — whole frames only, so concurrent responses can
//     interleave whole frames but never bytes within a frame. It runs on the
//     request/response path after each dispatched frame and is safe to call
//     at any point.
package acp

import "bytes"

// PushProgress appends one intermediate progress frame. When the buffer is
// full it coalesces: the most recently buffered progress frame is replaced in
// place with the newest snapshot, the buffered count stays at the bound, and
// no outcome frame is touched. When the buffer holds only outcomes there is
// nothing safe to coalesce, so the push fails with ErrEventBufferFull.
func (b *eventBuffer) PushProgress(frame []byte) (coalesced bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.frames) < b.bound {
		b.frames = append(b.frames, bytes.Clone(frame))
		b.progress = append(b.progress, true)
		return false, nil
	}
	for i := len(b.frames) - 1; i >= 0; i-- {
		if b.progress[i] {
			b.frames[i] = bytes.Clone(frame)
			return true, nil
		}
	}
	return false, ErrEventBufferFull
}

// PushOutcome appends one terminal outcome frame. It drops the oldest
// buffered progress frames, one at a time, to reserve the slot, and never
// drops an outcome. When the buffer holds only outcomes and nothing may be
// dropped, the push fails with ErrEventBufferFull.
func (b *eventBuffer) PushOutcome(frame []byte) (dropped int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.frames) >= b.bound {
		idx := -1
		for i := range b.progress {
			if b.progress[i] {
				idx = i
				break
			}
		}
		if idx < 0 {
			return dropped, ErrEventBufferFull
		}
		b.frames = append(b.frames[:idx], b.frames[idx+1:]...)
		b.progress = append(b.progress[:idx], b.progress[idx+1:]...)
		dropped++
	}
	b.frames = append(b.frames, bytes.Clone(frame))
	b.progress = append(b.progress, false)
	return dropped, nil
}

// EnqueueProgress queues one intermediate progress frame for delivery. The
// frame is coalescable under the delivery policy above; a coalescing decision
// is recorded as an actionable diagnostic.
func (s *Server) EnqueueProgress(frame []byte) error {
	coalesced, err := s.events.PushProgress(frame)
	if err != nil {
		s.diag.Printf("event buffer full (bound %d): progress frame dropped and the buffer holds only terminal outcomes, so no frame may be coalesced; check whether the client is reading the protocol stream", EventBufferBound)
		return err
	}
	if coalesced {
		s.diag.Printf("progress frame coalesced: the oldest buffered progress snapshot was replaced to stay at the %d-frame bound; the newest progress snapshot stays buffered and terminal outcomes are untouched", EventBufferBound)
	}
	return nil
}

// EnqueueOutcome queues one terminal outcome frame. Coalescing never removes
// it: when the buffer is full, older progress frames drop (diagnosed) to
// reserve the slot, and the push fails only when nothing may drop.
func (s *Server) EnqueueOutcome(frame []byte) error {
	dropped, err := s.events.PushOutcome(frame)
	if dropped > 0 {
		s.diag.Printf("%d intermediate progress frame(s) dropped to reserve a slot for a terminal outcome (bound %d)", dropped, EventBufferBound)
	}
	if err != nil {
		s.diag.Printf("event buffer full (bound %d): terminal outcome frame rejected because the buffer holds only terminal outcomes; check whether the client is reading the protocol stream", EventBufferBound)
		return err
	}
	return nil
}

// PumpEvents drains the buffered frames to the client in FIFO order through
// the single protocolWriter. Whole frames only: WriteFrame writes one frame
// body and its newline under one lock. A write error stops the drain and is
// diagnosed; the remaining frames stay buffered within the bound.
func (s *Server) PumpEvents() {
	for {
		frame, ok := s.events.Pop()
		if !ok {
			return
		}
		if err := s.out.WriteFrame(frame); err != nil {
			s.diag.Printf("write event frame: %v; this frame is lost and the remaining buffered frames stay within the %d-frame bound", err, EventBufferBound)
			return
		}
	}
}
