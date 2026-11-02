package acp

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// protocolWriter is the only writer allowed to put protocol frames on stdout.
// One process builds one writer; the single instance is the single
// serialization point, so concurrent responses can interleave whole frames but
// never bytes within a frame. It never writes diagnostics: the protocol stream
// carries protocol frames only.
type protocolWriter struct {
	mu  sync.Mutex
	out io.Writer
}

// newProtocolWriter wraps out as the protocol stream.
func newProtocolWriter(out io.Writer) *protocolWriter {
	return &protocolWriter{out: out}
}

// WriteFrame writes exactly one newline-delimited frame. The frame body and
// the terminating newline are written in one locked Write call, so a partial
// write from another goroutine can never appear between two frames.
func (w *protocolWriter) WriteFrame(frame []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	buf := make([]byte, 0, len(frame)+1)
	buf = append(buf, frame...)
	buf = append(buf, '\n')
	_, err := w.out.Write(buf)
	return err
}

// ErrEventBufferFull is returned by EventBuffer Push beyond its bound.
var ErrEventBufferFull = errors.New("acp: event buffer is full")

// eventBuffer is the bounded FIFO for outbound event frames. The bound keeps a
// stalled client from growing server memory without limit: a push beyond the
// bound is rejected in place through Push, the caller records an actionable
// diagnostic, and the buffered count never exceeds the bound. The outbound
// progress stage (delivery.go) adds the classified push paths that coalesce
// intermediate frames while preserving terminal outcomes.
type eventBuffer struct {
	bound  int
	mu     sync.Mutex
	frames [][]byte
	// progress flags every buffered frame as intermediate progress (true) or
	// a terminal outcome (false). Push records false: frames enqueued through
	// the unclassified EnqueueEvent path are treated as outcomes and are
	// never coalesced. The classified producers are EnqueueProgress and
	// EnqueueOutcome (delivery.go).
	progress []bool
}

// newEventBuffer builds a buffer whose Push fails at bound frames.
func newEventBuffer(bound int) *eventBuffer {
	return &eventBuffer{bound: bound}
}

// Push appends one frame as a terminal outcome. It returns
// ErrEventBufferFull when the buffer already holds its bound; the rejected
// frame is dropped and the buffered set is unchanged. Classified producers
// (EnqueueProgress, EnqueueOutcome) use the coalescing paths instead.
func (b *eventBuffer) Push(frame []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.frames) >= b.bound {
		return ErrEventBufferFull
	}
	b.frames = append(b.frames, bytes.Clone(frame))
	b.progress = append(b.progress, false)
	return nil
}

// Pop removes and returns the oldest buffered frame.
func (b *eventBuffer) Pop() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.frames) == 0 {
		return nil, false
	}
	frame := b.frames[0]
	b.frames[0] = nil
	b.frames = b.frames[1:]
	b.progress = b.progress[1:]
	return frame, true
}

// Len reports the buffered frame count.
func (b *eventBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.frames)
}

// EnqueueEvent queues one outbound event frame for later delivery to the
// client through the unclassified path: such a frame is recorded as a
// terminal outcome, so coalescing never removes it. The classified producers
// (EnqueueProgress and EnqueueOutcome in delivery.go) own the SOP progress
// and outcome frames. On ErrEventBufferFull the server records an actionable
// diagnostic and gives the frame back to the caller as an error.
func (s *Server) EnqueueEvent(frame []byte) error {
	if err := s.events.Push(frame); err != nil {
		s.diag.Printf("event buffer full (bound %d): event dropped; the client is not reading the protocol stream", EventBufferBound)
		return err
	}
	return nil
}
