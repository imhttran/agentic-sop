// Package command adapts an external, provider-neutral decision process to the
// decision.Provider interface. The process reads the SEAM-002 request DTO on
// stdin and writes the SEAM-002 result DTO on stdout.
//
// The process is reached by direct executable invocation (exec.CommandContext
// with a separate executable and argument vector) — no shell is used, so no
// provider output is ever interpreted as shell code. SOP validates the result
// (decision.Request.ValidateResult) before any policy could consume it; this
// adapter never grants the process lifecycle, approval, commit, merge, or
// policy authority. It implements no provider-specific transport.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// maxErrorDetail bounds the diagnostic detail (stderr) copied into an error, so
// a chatty or leaking process cannot flood the caller with unbounded output.
const maxErrorDetail = 512

// maxOutputBytes caps how much of a provider's stdout the adapter retains, so an
// untrusted provider cannot exhaust memory by writing unbounded output. Output
// larger than this bound is treated as a provider failure (fail closed), never a
// silently truncated result.
const maxOutputBytes = 1 << 20 // 1 MiB

// waitDelay bounds how long the adapter waits for the provider's I/O to drain after
// the process has exited or been killed, so a provider that leaves a child process
// holding stdout cannot stall the caller past SOP's deadline. An unfinished drain is
// a provider failure (fail closed), never a success.
const waitDelay = 2 * time.Second

// boundedWriter retains at most max bytes and records whether more was written.
// It always reports a full write so the child process is never blocked or errored
// by the bound; the overflow is surfaced explicitly by the caller instead.
type boundedWriter struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func newBoundedWriter(max int) *boundedWriter { return &boundedWriter{max: max} }

// Write retains up to the bound and drops the excess, flagging overflow.
func (w *boundedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) > room {
			_, _ = w.buf.Write(p[:room])
			w.overflow = true
		} else {
			_, _ = w.buf.Write(p)
		}
	} else if len(p) > 0 {
		w.overflow = true
	}
	return len(p), nil
}

// String returns the retained bytes.
func (w *boundedWriter) String() string { return w.buf.String() }

// Adapter runs a provider-neutral external decision process. The executable and
// its arguments are kept structurally separate; the adapter never interpolates
// provider output into a command.
type Adapter struct {
	name string
	argv []string
}

// New returns an Adapter for the given stable name and executable argv. The
// name is an opaque, provider-neutral identifier (selection is configuration,
// never a policy branch). An empty name or executable is an error.
func New(name string, argv []string) (decision.Provider, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("decision command: name is required")
	}
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("decision command: executable is required")
	}
	return &Adapter{name: strings.TrimSpace(name), argv: append([]string(nil), argv...)}, nil
}

// Name returns the provider's stable, provider-neutral identifier.
func (a *Adapter) Name() string { return a.name }

// requestWire is the SEAM-002 request DTO (provider-neutral).
type requestWire struct {
	ContractVersion int                `json:"contract_version"`
	Kind            string             `json:"kind"`
	Question        string             `json:"question"`
	Signals         map[string]float64 `json:"signals,omitempty"`
	Choices         []string           `json:"choices,omitempty"`
	DeadlineMS      int64              `json:"deadline_ms,omitempty"`
}

// Decide sends the bounded request and validates the bounded result. Any
// transport, parse, validation, unsupported, indeterminate, timeout, or
// cancellation outcome is returned as an error — never as an approval and never
// as a successful decision.
func (a *Adapter) Decide(ctx context.Context, req decision.Request) (decision.Decision, error) {
	wire := requestWire{
		ContractVersion: decision.ContractVersion,
		Kind:            req.UseCase,
		Question:        req.Subject,
		Signals:         req.Signals,
	}
	if len(req.Choices) > 0 {
		wire.Choices = make([]string, 0, len(req.Choices))
		for _, c := range req.Choices {
			wire.Choices = append(wire.Choices, string(c))
		}
	}
	// The deadline is advisory to the provider; SOP enforces it via ctx below.
	if dl, ok := ctx.Deadline(); ok {
		if ms := time.Until(dl).Milliseconds(); ms > 0 {
			wire.DeadlineMS = ms
		}
	}

	payload, err := json.Marshal(wire)
	if err != nil {
		return decision.Decision{}, fmt.Errorf("%w: encode request: %v", decision.ErrProviderFailure, err)
	}

	cmd := exec.CommandContext(ctx, a.argv[0], a.argv[1:]...)
	cmd.WaitDelay = waitDelay
	cmd.Stdin = bytes.NewReader(payload)

	stdout := newBoundedWriter(maxOutputBytes)
	stderr := newBoundedWriter(maxErrorDetail)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		// A cancelled or timed-out context terminates the process; it is a
		// provider failure, never a success.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return decision.Decision{}, fmt.Errorf("%w: decision command %s: %v", decision.ErrProviderFailure, a.name, ctxErr)
		}
		if detail := bounded(stderr.String()); detail != "" {
			return decision.Decision{}, fmt.Errorf("%w: decision command %s failed: %v: %s", decision.ErrProviderFailure, a.name, err, detail)
		}
		return decision.Decision{}, fmt.Errorf("%w: decision command %s failed: %v", decision.ErrProviderFailure, a.name, err)
	}

	if stdout.overflow {
		// Output beyond the bound is a provider failure, never a silently truncated
		// result: an untrusted provider cannot make SOP parse a partial document.
		return decision.Decision{}, fmt.Errorf("%w: decision command %s produced more than %d bytes of output", decision.ErrProviderFailure, a.name, maxOutputBytes)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		// Empty output is a failure; it can never become a successful decision.
		return decision.Decision{}, fmt.Errorf("%w: decision command %s returned empty output", decision.ErrProviderFailure, a.name)
	}

	var res decision.Result
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return decision.Decision{}, fmt.Errorf("%w: decision command %s returned malformed output: %v", decision.ErrInvalidResult, a.name, err)
	}
	// SOP owns validation; a provider result is data until it passes.
	if err := req.ValidateResult(res); err != nil {
		return decision.Decision{}, err
	}
	return decision.Decision{
		Choice:     res.Choice,
		Confidence: *res.Confidence,
		Metadata:   res.Diagnostics,
	}, nil
}

// bounded trims and truncates diagnostic detail for an error message.
func bounded(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxErrorDetail {
		return s[:maxErrorDetail]
	}
	return s
}
