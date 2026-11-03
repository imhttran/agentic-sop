package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/executionlease"
)

// executionLease takes the project execution lease for one `sop run`, honouring
// execution.lease (docs/plans/PROPOSAL-SHARED-EXECUTION-EXCLUSION.md):
//
//	off       nothing is acquired (the default: execution is unchanged)
//	advisory  a contended start is recorded and proceeds
//	required  a contended start is refused and names the holder
//
// The returned lease is nil when nothing was acquired, and Release on a nil lease is
// safe, so callers can defer it unconditionally.
func executionLease(cfg config.Config, dir string, owner executionlease.Owner, stderr io.Writer) (*executionlease.Held, error) {
	mode, err := executionlease.ParseMode(cfg.Execution.Lease)
	if err != nil {
		return nil, err
	}
	if mode == executionlease.ModeOff {
		return nil, nil
	}

	held, err := executionlease.Acquire(dir, owner)
	if err == nil {
		// Refresh the record while the execution runs, so a healthy run longer than
		// HeartbeatLimit is not mistaken for stale where pid liveness cannot be
		// checked. A lost lease is reported, not acted on: cancelling the run is a
		// separate decision.
		held.WatchHeartbeat(executionlease.HeartbeatEvery, func(err error) {
			fmt.Fprintf(stderr, "run: execution lease heartbeat failed (%s): %v\n", held.Owner().Describe(), err)
		})
		return held, nil
	}
	var busy *executionlease.BusyError
	if !errors.As(err, &busy) {
		return nil, err
	}
	if mode == executionlease.ModeAdvisory {
		// Advisory records contention and proceeds; it holds nothing, so the competing
		// execution is never blocked by evidence-gathering.
		fmt.Fprintf(stderr, "run: advisory execution lease: %v\n", err)
		return nil, nil
	}
	return nil, err
}
