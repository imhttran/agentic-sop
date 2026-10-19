package retrievalboundary

import (
	"context"
	"errors"
)

// timeoutErr maps a context cancellation/deadline to the typed ErrTimeout. A caller that
// cancels for a non-deadline reason still gets a timeout-class error at the boundary so a
// bounded read never blocks indefinitely.
func timeoutErr(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrTimeout
	}
	if err == nil {
		return nil
	}
	return ErrTimeout
}
