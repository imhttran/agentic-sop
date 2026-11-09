package toolharness

import (
	"bytes"
	"fmt"
)

// boundedCapture is the read-boundary bound for command output. It keeps at most
// limit bytes while counting every byte written, so a command that streams
// unbounded output cannot make the harness allocate it. It implements io.Writer,
// which is what os/exec writes into; the full stream is still drained (so the
// child never blocks on a full pipe), but only the bounded prefix is retained.
type boundedCapture struct {
	limit int
	buf   bytes.Buffer
	total int
}

// newBoundedCapture returns a capture that retains at most limit bytes. A
// non-positive limit keeps the built-in default rather than disabling the bound,
// matching the harness contract that a missing or malformed value never makes a
// safety limit unbounded.
func newBoundedCapture(limit int) *boundedCapture {
	if limit <= 0 {
		limit = DefaultConfig().MaxOutputBytes
	}
	return &boundedCapture{limit: limit}
}

// Write retains only the bytes that fit in the bound and reports the full length
// as written, so os/exec keeps reading and the child never sees a short write.
func (b *boundedCapture) Write(p []byte) (int, error) {
	b.total += len(p)
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

// String renders the retained bytes with the same truncation marker the
// post-allocation truncate helper used, so a bounded read and a full read of the
// same stream produce the same result.
func (b *boundedCapture) String() string {
	if b.total > b.limit {
		return b.buf.String() + fmt.Sprintf("\n… [truncated %d bytes]", b.total-b.limit)
	}
	return b.buf.String()
}
