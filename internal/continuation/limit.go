package continuation

import "github.com/imhttran/agentic-sop/internal/autonomy"

// Counter is the persisted continuation count. *runpkg.Run satisfies it through
// continuations.txt, which survives across processes, so a restart cannot reset
// the budget.
type Counter interface {
	ReadContinuations() int
	RecordContinuations(int) error
}

// Consume spends one continuation from c when fewer than max have been used,
// returning the count after the call and whether the continuation is allowed. A
// non-positive max falls back to autonomy.DefaultMaxContinuations. The count is
// persisted before the continuation is allowed, so a failed write refuses it.
func Consume(c Counter, max int) (n int, allowed bool, err error) {
	if max <= 0 {
		max = autonomy.DefaultMaxContinuations
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
