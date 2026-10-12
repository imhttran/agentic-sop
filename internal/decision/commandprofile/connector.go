// Package commandprofile is SOP's production connector between the canonical
// SEAM-002 decision seam and a provider-neutral, batched decision adapter
// executable, bridged through the ratified M3 profile bridge
// (internal/decision/bridge).
//
// It carries no authority: it returns bounded decision evidence and never
// approves, executes, escalates, or bypasses the human boundary. Deterministic
// SOP policy (decision.Route, internal/autonomy) retains final authority, and
// every failure fails closed.
//
// The connector executes the configured adapter as a subprocess over the batched
// profile (one question, one consumed answer), with a private temporary request
// file, bounded output, an explicit exit-code mapping, and typed, class-only
// errors.
package commandprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
)

const (
	// defaultTimeout bounds one adapter invocation.
	defaultTimeout = 120 * time.Second
	// maxOutputBytes bounds the adapter's stdout, so an untrusted adapter cannot
	// exhaust memory. Output beyond the bound is a provider failure.
	maxOutputBytes = 1 << 20 // 1 MiB
)

// Options configures a Connector.
type Options struct {
	// Registry holds the config-driven M3 profiles the connector recognizes.
	Registry *bridge.Registry
	// Argv is the adapter executable and its fixed arguments
	// (decision.adapter_command); the connector appends the per-call arguments.
	Argv []string
	// Provider and Model are the adapter's selection hints, passed through.
	Provider string
	Model    string
	// Timeout bounds one invocation (defaults to defaultTimeout).
	Timeout time.Duration
}

// Connector implements decision.Provider over an external batched adapter.
type Connector struct {
	registry *bridge.Registry
	argv     []string
	provider string
	model    string
	timeout  time.Duration
}

// New builds a Connector. It never selects a profile by provider name alone; the
// registry is config-driven and recognition is structural (see bridge).
func New(opts Options) (*Connector, error) {
	if opts.Registry == nil {
		return nil, fmt.Errorf("commandprofile: profile registry is required")
	}
	if len(opts.Argv) == 0 || strings.TrimSpace(opts.Argv[0]) == "" {
		return nil, fmt.Errorf("commandprofile: adapter command is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Connector{
		registry: opts.Registry,
		argv:     append([]string(nil), opts.Argv...),
		provider: strings.TrimSpace(opts.Provider),
		model:    strings.TrimSpace(opts.Model),
		timeout:  timeout,
	}, nil
}

// Name reports the provider-neutral identifier of this provider.
func (c *Connector) Name() string { return "adapter" }

// Decide translates a canonical request through the M3 bridge, invokes the
// adapter, and returns canonical evidence. Every failure fails closed with a
// typed sentinel; none is ever an approval.
func (c *Connector) Decide(ctx context.Context, req decision.Request) (decision.Decision, error) {
	built, err := c.registry.Build(req)
	if err != nil {
		return decision.Decision{}, err
	}

	// Private temporary request file (dir 0700, file 0600), removed on return.
	dir, err := os.MkdirTemp("", "sop-adapter-")
	if err != nil {
		return decision.Decision{}, fmt.Errorf("%w: temporary request directory: %v", decision.ErrProviderFailure, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	reqPath := filepath.Join(dir, "request.json")
	payload, err := built.RequestJSON()
	if err != nil {
		return decision.Decision{}, fmt.Errorf("%w: encode profile request", decision.ErrProviderFailure)
	}
	if err := os.WriteFile(reqPath, payload, 0o600); err != nil {
		return decision.Decision{}, fmt.Errorf("%w: write profile request", decision.ErrProviderFailure)
	}

	argv := append([]string(nil), c.argv...)
	argv = append(argv, "decide", "-file", reqPath, "-json")
	if c.provider != "" {
		argv = append(argv, "-provider", c.provider)
	}
	if c.model != "" {
		argv = append(argv, "-model", c.model)
	}
	argv = append(argv, "-timeout", c.timeout.String())

	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.Command(argv[0], argv[1:]...)
	setProcGroup(cmd)
	stdout := newBoundedWriter(maxOutputBytes)
	stderr := newBoundedWriter(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return decision.Decision{}, fmt.Errorf("%w: adapter failed to start", decision.ErrProviderFailure)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var runErr error
	select {
	case runErr = <-done:
	case <-cctx.Done():
		// Terminate the whole process tree, then classify cancellation vs deadline
		// distinctly; both fail closed.
		killProcessTree(cmd)
		<-done
		if errors.Is(cctx.Err(), context.Canceled) {
			return decision.Decision{}, fmt.Errorf("%w: adapter canceled", decision.ErrProviderFailure)
		}
		return decision.Decision{}, fmt.Errorf("%w: adapter timed out", decision.ErrProviderFailure)
	}

	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			switch ee.ExitCode() {
			case 2:
				return decision.Decision{}, fmt.Errorf("%w: adapter rejected the request", decision.ErrInvalidResult)
			case 3:
				return decision.Decision{}, fmt.Errorf("%w: adapter provider unavailable", decision.ErrProviderFailure)
			default:
				return decision.Decision{}, fmt.Errorf("%w: adapter exited with status %d", decision.ErrProviderFailure, ee.ExitCode())
			}
		}
		return decision.Decision{}, fmt.Errorf("%w: adapter failed to run", decision.ErrProviderFailure)
	}
	if stdout.overflow {
		return decision.Decision{}, fmt.Errorf("%w: adapter output exceeded the %d-byte bound", decision.ErrProviderFailure, maxOutputBytes)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return decision.Decision{}, fmt.Errorf("%w: adapter returned empty output", decision.ErrProviderFailure)
	}

	// Strict response validation: unknown fields are rejected.
	var res bridge.Result
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		return decision.Decision{}, fmt.Errorf("%w: adapter returned malformed output", decision.ErrInvalidResult)
	}

	canonical, err := built.Interpret(res)
	if err != nil {
		return decision.Decision{}, err
	}
	if canonical.Confidence == nil {
		return decision.Decision{}, fmt.Errorf("%w: adapter returned no usable confidence", decision.ErrIndeterminate)
	}
	return decision.Decision{
		Choice:     canonical.Choice,
		Confidence: *canonical.Confidence,
		Metadata:   canonical.Diagnostics,
	}, nil
}

// boundedWriter retains at most max bytes and records overflow.
type boundedWriter struct {
	buf      strings.Builder
	max      int
	overflow bool
}

func newBoundedWriter(max int) *boundedWriter { return &boundedWriter{max: max} }

func (w *boundedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) > room {
			w.buf.Write(p[:room])
			w.overflow = true
		} else {
			w.buf.Write(p)
		}
	} else if len(p) > 0 {
		w.overflow = true
	}
	return len(p), nil
}

func (w *boundedWriter) String() string { return w.buf.String() }
