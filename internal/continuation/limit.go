package continuation

import "github.com/imhttran/agentic-sop/internal/autonomy"

// Counter is the persisted continuation count. *runpkg.Run satisfies it through
// continuations.txt, which survives across processes, so a restart cannot reset
// the budget. A counter that also implements atomicCounter is consumed under its
// own cross-process lock.
type Counter interface {
	ReadContinuations() int
	RecordContinuations(int) error
}

// atomicCounter is an optional Counter capability. *runpkg.Run implements it so
// the read, check, and write happen in one project-locked critical section; a
// caller cannot spend the same continuation twice from concurrent processes.
type atomicCounter interface {
	ConsumeContinuation(max int) (n int, allowed bool, err error)
}

// Consume spends one continuation from c when fewer than max have been used,
// returning the count after the call and whether the continuation is allowed. A
// non-positive max falls back to autonomy.DefaultMaxContinuations. When c supports
// atomic consumption, the read and write are one locked operation; otherwise the
// count is persisted before the continuation is allowed, so a failed write
// refuses it.
func Consume(c Counter, max int) (n int, allowed bool, err error) {
	if max <= 0 {
		max = autonomy.DefaultMaxContinuations
	}
	if ac, ok := c.(atomicCounter); ok {
		return ac.ConsumeContinuation(max)
	}
	n = c.ReadContinuations()
	if n >= max {
		return n, false, nil
	}
	if err := c.RecordContinuations(n + 1); err != nil {
		return n, false, err
	}
	return n + 1, true, nil
}
