package handoff

import (
	"context"
	"fmt"
	"strings"
)

// Mode selects how a compressor is resolved.
type Mode string

const (
	// ModeNone always uses the no-op compressor.
	ModeNone Mode = "none"
	// ModeAuto uses the optional compressor when it is available and healthy,
	// otherwise falls back to the no-op compressor.
	ModeAuto Mode = "auto"
	// ModeCaveman explicitly prefers the optional compressor; when it is
	// unavailable, compression is skipped and the capsule is preserved.
	ModeCaveman Mode = "caveman"
)

// HealthChecker is an optional, cheap readiness check a compressor may
// implement. It must be bounded by ctx and non-destructive.
type HealthChecker interface {
	Check(ctx context.Context) error
}

// Candidate is an optional, selectable compressor (for example the Caveman
// adapter). It reports its name so diagnostics can identify it.
type Candidate interface {
	Compressor
	HealthChecker
	Name() string
}

// Diagnostic explains how a compressor was resolved. It is metadata, never
// workflow truth.
type Diagnostic struct {
	Mode       Mode
	Compressor string
	Reason     string
}

// SelectCompressor resolves a compressor for a mode. It never errors: inability
// to use the optional compressor is a fallback, not a fatal condition. A nil
// candidate means the optional compressor is not configured.
//
//   - none:    always no-op, without probing the candidate.
//   - auto:    use the candidate when it is healthy, else no-op.
//   - caveman: use the candidate when it is healthy; otherwise return a nil
//     compressor so compression is skipped and the capsule is preserved, with a
//     diagnostic explaining why.
func SelectCompressor(ctx context.Context, mode Mode, candidate Candidate) (Compressor, Diagnostic) {
	switch mode {
	case ModeNone:
		return NoOpCompressor{}, Diagnostic{Mode: mode, Compressor: "NoOp", Reason: "compression disabled"}

	case ModeAuto:
		if candidate == nil {
			return NoOpCompressor{}, Diagnostic{Mode: mode, Compressor: "NoOp", Reason: "optional compressor not configured"}
		}
		if err := candidate.Check(ctx); err != nil {
			return NoOpCompressor{}, Diagnostic{Mode: mode, Compressor: "NoOp", Reason: fmt.Sprintf("%s unhealthy: %v", candidate.Name(), err)}
		}
		return candidate, Diagnostic{Mode: mode, Compressor: candidate.Name()}

	case ModeCaveman:
		if candidate == nil {
			return nil, Diagnostic{Mode: mode, Reason: "optional compressor unavailable"}
		}
		if err := candidate.Check(ctx); err != nil {
			return nil, Diagnostic{Mode: mode, Reason: fmt.Sprintf("%s unhealthy: %v", candidate.Name(), err)}
		}
		return candidate, Diagnostic{Mode: mode, Compressor: candidate.Name()}

	default:
		return NoOpCompressor{}, Diagnostic{Mode: mode, Compressor: "NoOp", Reason: fmt.Sprintf("unknown mode %q", strings.TrimSpace(string(mode)))}
	}
}
