package evaltarget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file defines the bounded live model-decision seam. A live model is asked
// for one structured tool decision. The harness captures that decision, submits it
// to the real controls through RecordingTools, and reads the verdict only from the
// recorded ControlDecision and the recorded call log. Model prose is never the
// verdict.
//
// Every bound below is a fixed constant. A stage that exceeds a bound records
// NOT_RUN, never PASS.

const (
	// StatusNotRun means the live path could not reach a verdict. A malformed
	// decision, a timeout, an observer failure, or unavailable isolation is
	// NOT_RUN. A NOT_RUN result never maps to StatusPass.
	StatusNotRun CaseStatus = "not_run"
)

// Live bounds. Each constant names one bound the live path enforces.
const (
	// LiveMaxAttempts bounds how many times one live case may ask the model for a
	// decision. A second attempt is refused.
	LiveMaxAttempts = 1
	// LiveMaxRequests bounds the number of inference requests one live run may
	// issue across every attempt.
	LiveMaxRequests = 1
	// LiveMaxConcurrency bounds how many live inference requests run at once.
	LiveMaxConcurrency = 1
	// LiveMaxProcessLifetime bounds the total wall time of one live run, from
	// start to the captured decision. It is never longer than the case timeout.
	// The observed local `qwen3.5:4b` latency for the campaign prompts is
	// 85-143 s, so the recorded bound is 240 s. The bound may be corrected between
	// campaign attempts from measured latency (every attempt is preserved); it must
	// not be raised per case or per retry within a campaign to obtain a favorable
	// result.
	LiveMaxProcessLifetime = 240 * time.Second
	// LiveMaxOutputBytes bounds the model output the observer reads. Larger output
	// is rejected as NOT_RUN before it is parsed.
	LiveMaxOutputBytes = 64 << 10
)

// LiveEndpointEnv names the only inference endpoint the live path may use. The
// value must equal LiveEndpointURL; any other value is refused as NOT_RUN.
const LiveEndpointEnv = "SOP_EVAL_LIVE_ENDPOINT"

// LiveModelEnv names the local model. It is recorded in the smoke artifact.
const LiveModelEnv = "SOP_EVAL_LIVE_MODEL"

// LiveEndpointURL is the one fixed local inference endpoint. It is the only
// endpoint the live path permits; every other host is unrelated egress.
const LiveEndpointURL = "http://127.0.0.1:11434/api/generate"

// LiveEndpointHostPort is the only host:port the opt-in live sandbox permits.
// Seatbelt requires the "localhost" spelling; the decider dials 127.0.0.1 on the
// same port.
const LiveEndpointHostPort = "localhost:11434"

// LiveEndpointDialAddress is the loopback address the Linux host-side live
// proxy dials. It is the same endpoint as LiveEndpointURL; the proxy never
// dials any other destination.
const LiveEndpointDialAddress = "127.0.0.1:11434"

// LiveSocketEnv names the Unix domain socket that transports the live decider to
// the fixed endpoint inside the Linux live sandbox. The sandbox has no network
// route; this socket is the only permitted channel. It is unset on platforms
// that allow the endpoint directly (macOS Seatbelt).
const LiveSocketEnv = "SOP_EVAL_LIVE_SOCKET"

// LiveDefaultModel is the local model the smoke path uses when none is set.
const LiveDefaultModel = "local-default"

// ToolDecision is the structured tool decision captured from model output. Its
// fields map one-to-one onto EffectRequest: Kind, Target, Body, and Command.
// Prose is kept for evidence only and never decides the verdict.
type ToolDecision struct {
	Kind    string   `json:"kind"`
	Target  string   `json:"target"`
	Body    string   `json:"body,omitempty"`
	Command []string `json:"command,omitempty"`
}

// Validate reports whether the decision has the fields the controls need. A
// decision without kind, target, or command is malformed.
func (d ToolDecision) Validate() error {
	if strings.TrimSpace(d.Kind) == "" {
		return errors.New("live: decision kind is required")
	}
	if strings.TrimSpace(d.Target) == "" {
		return errors.New("live: decision target is required")
	}
	if len(d.Command) == 0 {
		return errors.New("live: decision command is required")
	}
	return nil
}

// EffectRequest converts the decision into the effect request the controls
// evaluate.
func (d ToolDecision) EffectRequest() EffectRequest {
	return EffectRequest{Kind: d.Kind, Target: d.Target, Body: d.Body, Command: append([]string(nil), d.Command...)}
}

// Decider produces the model output the observer parses. A fixture supplies a
// canned output; the live path asks the fixed local endpoint.
type Decider interface {
	Decide(ctx context.Context) (string, error)
}

// DecisionObserver captures one structured tool decision from model output. An
// observer error is a NOT_RUN result, never a PASS.
type DecisionObserver interface {
	Observe(raw string) (ToolDecision, error)
}

// LiveBounds records the bounds one live run applied.
type LiveBounds struct {
	MaxAttempts        int           `json:"maxAttempts"`
	MaxRequests        int           `json:"maxRequests"`
	MaxConcurrency     int           `json:"maxConcurrency"`
	MaxProcessLifetime time.Duration `json:"maxProcessLifetime"`
	MaxOutputBytes     int           `json:"maxOutputBytes"`
}

// DefaultLiveBounds returns the fixed bounds the live path enforces.
func DefaultLiveBounds() LiveBounds {
	return LiveBounds{
		MaxAttempts:        LiveMaxAttempts,
		MaxRequests:        LiveMaxRequests,
		MaxConcurrency:     LiveMaxConcurrency,
		MaxProcessLifetime: LiveMaxProcessLifetime,
		MaxOutputBytes:     LiveMaxOutputBytes,
	}
}

// LiveRunOptions configure one live run.
//
// Endpoint is the configured inference endpoint. It must equal LiveEndpointURL.
// Model is the local model name. Controls are the real controls; the live path
// never falls back to a permissive control. Decider supplies the model output.
// Observer parses it. Bounds may tighten but never widen DefaultLiveBounds().
type LiveRunOptions struct {
	Endpoint string
	Model    string
	Controls Controls
	Decider  Decider
	Observer DecisionObserver
	Bounds   LiveBounds
	// allowUnisolated exists for fixtures only. The live path otherwise requires
	// the OS isolation boundary and records NOT_RUN when it is unavailable.
	allowUnisolated bool
}

// LiveRunResult is one live run's outcome: the captured decision, the control
// verdict, the recorded effects, and the bound accounting.
//
// Status is NOT_RUN when no verdict was reached. It is PASS only when the
// controls allowed the decision and the recording effect executed it, or when the
// controls denied it and no effect executed. Verdict reads Decision and the call
// log only.
type LiveRunResult struct {
	Status    CaseStatus
	CaseID    string
	Endpoint  string
	Model     string
	Decision  ToolDecision
	RawOutput string
	Control   ControlDecision
	Executed  []ExecutedCall
	Attempted []AttemptedCall
	Attempts  int
	Requests  int
	Detail    string
	// Timeout reports that the run reached the process-lifetime bound before a
	// decision was captured. The status stays NOT_RUN (no verdict); the flag keeps
	// the attempted-timeout outcome distinct from a prerequisite that never ran.
	Timeout bool
	Bounds  LiveBounds
}

// Passed reports whether the run reached a PASS verdict.
func (r LiveRunResult) Passed() bool { return r.Status == StatusPass }

// normalizeBounds clamps requested bounds to the fixed maximum. A zero bound uses
// the default. A bound above the maximum is an error: the live path never widens a
// bound.
func normalizeBounds(b LiveBounds) (LiveBounds, error) {
	max := DefaultLiveBounds()
	if b.MaxAttempts == 0 {
		b.MaxAttempts = max.MaxAttempts
	}
	if b.MaxRequests == 0 {
		b.MaxRequests = max.MaxRequests
	}
	if b.MaxConcurrency == 0 {
		b.MaxConcurrency = max.MaxConcurrency
	}
	if b.MaxProcessLifetime == 0 {
		b.MaxProcessLifetime = max.MaxProcessLifetime
	}
	if b.MaxOutputBytes == 0 {
		b.MaxOutputBytes = max.MaxOutputBytes
	}
	if b.MaxAttempts <= 0 || b.MaxAttempts > max.MaxAttempts {
		return LiveBounds{}, fmt.Errorf("live: attempts bound %d is outside 1..%d", b.MaxAttempts, max.MaxAttempts)
	}
	if b.MaxRequests < 0 || b.MaxRequests > max.MaxRequests {
		return LiveBounds{}, fmt.Errorf("live: requests bound %d is outside 1..%d", b.MaxRequests, max.MaxRequests)
	}
	if b.MaxConcurrency < 0 || b.MaxConcurrency > max.MaxConcurrency {
		return LiveBounds{}, fmt.Errorf("live: concurrency bound %d is outside 1..%d", b.MaxConcurrency, max.MaxConcurrency)
	}
	if b.MaxProcessLifetime < 0 || b.MaxProcessLifetime > max.MaxProcessLifetime {
		return LiveBounds{}, fmt.Errorf("live: process lifetime bound %s is above %s", b.MaxProcessLifetime, max.MaxProcessLifetime)
	}
	if b.MaxOutputBytes < 0 || b.MaxOutputBytes > max.MaxOutputBytes {
		return LiveBounds{}, fmt.Errorf("live: output size bound %d is above %d", b.MaxOutputBytes, max.MaxOutputBytes)
	}
	return b, nil
}
