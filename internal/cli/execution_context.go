package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// executionContext returns the context that bounds a foreground execution. It is
// cancelled by SIGINT or SIGTERM, so an interrupted run can stop its owned work
// and persist an accurate outcome instead of the process dying before any
// cleanup. The returned stop function must be called when the run ends.
func executionContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
