package julia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// This file defines the Runner seam: the boundary between the Julia adapter and
// the concrete ONNX execution that actually evaluates a decision. The structs
// here are Julia adapter/runtime structures, not part of the provider-neutral
// public contract: they carry Julia's native vocabulary (qtype, ordered options)
// and exist only to describe one inference call.
//
// The adapter depends only on the Runner interface, so it can be exercised in
// tests without any ONNX runtime, and a future in-process ONNX binding can
// replace the external helper process without touching the adapter, the
// translation layer, the CLI, or the public decision contract.

// Inputs is one Julia runtime inference request. It carries the free-form state
// and one entry per question to evaluate.
type Inputs struct {
	// State is the free-form context the decision is made against.
	State string `json:"state"`
	// Questions are the questions to evaluate against State.
	Questions []QuestionInput `json:"questions"`
}

// QuestionInput is a single question handed to the Julia runtime. It carries
// Julia's native question type and ordered options, so the runtime does not need
// to re-derive them from provider-neutral question types.
type QuestionInput struct {
	// ID identifies the question and keys the corresponding output.
	ID string `json:"id"`
	// Type is the provider-neutral question type, retained for diagnostics.
	Type decision.QuestionType `json:"type"`
	// QType is Julia's native question type: 0 choice, 1 score, 2 noul/boolean.
	QType int `json:"qtype"`
	// Text is the resolved natural-language instruction for the question.
	Text string `json:"text"`
	// Options are the ordered Julia options. For BOOLEAN these are always
	// ["false", "true"] (index 0 false, index 1 true).
	Options []string `json:"options,omitempty"`
}

// Outputs is the raw, per-question result returned by a Runner.
//
// It is close to the model's native output and is normalized by the translation
// layer before it reaches callers. In particular it does not prescribe a boolean
// representation: a boolean question reports its result via Probability (the
// model's yes-probability).
type Outputs struct {
	// Model is the model identifier the runner reports, when known.
	Model string `json:"model,omitempty"`
	// Answers maps each question ID to its raw output.
	Answers map[string]QuestionOutput `json:"answers"`
}

// QuestionOutput is the raw output for a single question. Which fields are
// populated depends on the question type:
//
//   - choice:  Choice and (optionally) Probabilities, and optionally Confidence.
//   - boolean: Probability (the model's yes-probability), optionally Confidence.
//   - score:   Score, optionally Confidence.
type QuestionOutput struct {
	// Choice is the selected label for a choice question.
	Choice string `json:"choice,omitempty"`
	// Probabilities maps each allowed label to its probability in [0, 1].
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	// Probability is the model's yes-probability for a boolean question.
	Probability *float64 `json:"probability,omitempty"`
	// Score is the numeric value for a score question.
	Score *float64 `json:"score,omitempty"`
	// Confidence is the model's confidence in the answer, in [0, 1].
	Confidence *float64 `json:"confidence,omitempty"`
}

// Runner executes the Julia/ONNX model. Implementations take a Julia runtime
// request (Inputs) and return the raw Outputs (an external helper process today,
// an in-process ONNX binding later).
//
// A Runner must be safe to call with a cancelled or expired context and must
// surface transport-like failures in a way the adapter can classify (see
// errRunnerUnavailable and errRunnerMalformed).
type Runner interface {
	// Name reports the runner/model identifier (for example "julia").
	Name() string
	// Available reports whether the runner can currently serve inference. It
	// must not perform an inference request.
	Available(ctx context.Context) bool
	// Run evaluates Inputs and returns the raw Outputs.
	Run(ctx context.Context, in Inputs) (Outputs, error)
}

// RunnerFunc adapts an ordinary function into a Runner. It makes tests trivial:
// supply Run behaviour plus an availability flag and runner name.
type RunnerFunc struct {
	// NameValue is reported by Name.
	NameValue string
	// AvailableValue is reported by Available.
	AvailableValue bool
	// RunFunc performs the inference. A nil RunFunc returns an empty Outputs.
	RunFunc func(ctx context.Context, in Inputs) (Outputs, error)
}

// Name implements Runner.
func (f RunnerFunc) Name() string {
	if f.NameValue == "" {
		return ProviderName
	}
	return f.NameValue
}

// Available implements Runner. It never invokes RunFunc.
func (f RunnerFunc) Available(context.Context) bool { return f.AvailableValue }

// Run implements Runner.
func (f RunnerFunc) Run(ctx context.Context, in Inputs) (Outputs, error) {
	if f.RunFunc == nil {
		return Outputs{}, nil
	}
	return f.RunFunc(ctx, in)
}

// Sentinel error classes a Runner may return so the adapter can map a failure
// onto the correct decision.ErrorKind. A Runner wraps these so the adapter can
// classify transport-like failures without depending on the concrete runtime.
var (
	// errRunnerUnavailable marks a runner that cannot be reached or executed
	// (for example an unconfigured or failing inference command). The adapter
	// maps it onto decision.ErrUnavailable.
	errRunnerUnavailable = errors.New("julia: runner unavailable")
	// errRunnerMalformed marks a runner that ran but produced output the runner
	// could not decode. The adapter maps it onto decision.ErrMalformedResponse.
	errRunnerMalformed = errors.New("julia: runner produced malformed output")
)

// isRunnerUnavailable reports whether err is classified as an unavailable
// runner.
func isRunnerUnavailable(err error) bool { return errors.Is(err, errRunnerUnavailable) }

// isRunnerMalformed reports whether err is classified as malformed runner
// output.
func isRunnerMalformed(err error) bool { return errors.Is(err, errRunnerMalformed) }

// CommandRunner is the production Runner backed by an external inference
// command. It marshals Inputs to the command's stdin as JSON and decodes Outputs
// from the command's stdout as JSON.
//
// The documented, default execution path is the repository-owned Python/ONNX
// helper (tools/julia/infer.py) invoked as `python3 <helper>`; an operator may
// override it with JULIA_INFERENCE_CMD. Either way the command implements the
// same JSON-in/JSON-out contract. An in-process ONNX binding can replace
// CommandRunner later without changing the Runner interface.
type CommandRunner struct {
	// Command is the argv of the inference command. An empty Command means the
	// runner is unconfigured and therefore unavailable.
	Command []string
	// ModelPath optionally points at the ONNX model. When set it is exported to
	// the child as JULIA_MODEL_PATH and its presence is checked by Available.
	ModelPath string
	// HelperPath is the repository-owned helper script backing the default
	// command, when one is used. When set, Available requires it to exist and
	// requires a real ModelPath; an operator-provided JULIA_INFERENCE_CMD leaves
	// it empty.
	HelperPath string
	// Timeout bounds a single inference call. A non-positive value falls back
	// to DefaultTimeout.
	Timeout time.Duration
}

// Name implements Runner.
func (r *CommandRunner) Name() string { return ProviderName }

// Available implements Runner. It is a cheap, side-effect-free prerequisite
// check; it never executes the command.
//
// It reports true only when a command is configured and its program resolves on
// PATH. For the built-in helper it additionally requires the helper script and a
// real model path to exist, because the helper cannot serve inference without
// them.
func (r *CommandRunner) Available(context.Context) bool {
	if r == nil || len(r.Command) == 0 {
		return false
	}
	if _, err := exec.LookPath(r.Command[0]); err != nil {
		return false
	}
	if r.HelperPath != "" {
		if !fileExists(r.HelperPath) {
			return false
		}
		if strings.TrimSpace(r.ModelPath) == "" || !fileExists(r.ModelPath) {
			return false
		}
	}
	return true
}

// Run implements Runner.
//
// It bounds the call with the configured Timeout, marshals in to stdin, decodes
// stdout into Outputs, captures stderr for diagnostics, and forwards
// JULIA_MODEL_PATH to the child environment.
//
// Failures are classified so the adapter can map them onto a decision.ErrorKind:
//
//   - command unset, start/exec failure, timeout -> errRunnerUnavailable
//   - a helper that exits non-zero and reports kind "malformed" -> errRunnerMalformed
//   - a helper that exits non-zero and reports kind "failure"  -> a generic error
//     (the adapter maps it to decision.ErrProviderFailure)
//   - an unclassified non-zero exit -> errRunnerUnavailable
//   - undecodable stdout -> errRunnerMalformed
func (r *CommandRunner) Run(ctx context.Context, in Inputs) (Outputs, error) {
	if r == nil || len(r.Command) == 0 {
		return Outputs{}, fmt.Errorf("%w: no inference command configured", errRunnerUnavailable)
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(in)
	if err != nil {
		return Outputs{}, fmt.Errorf("julia: encode runner input: %w", err)
	}

	cmd := exec.CommandContext(ctx, r.Command[0], r.Command[1:]...)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = r.childEnv()

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Outputs{}, fmt.Errorf("%w: inference command timed out after %s: %v", errRunnerUnavailable, timeout, ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		switch decodeHelperErrorKind(stdout.Bytes()) {
		case helperErrorMalformed:
			return Outputs{}, fmt.Errorf("%w: inference helper reported malformed output: %v: %s", errRunnerMalformed, err, msg)
		case helperErrorFailure:
			return Outputs{}, fmt.Errorf("julia: inference failed: %v: %s", err, msg)
		default:
			if msg != "" {
				return Outputs{}, fmt.Errorf("%w: inference command failed: %v: %s", errRunnerUnavailable, err, msg)
			}
			return Outputs{}, fmt.Errorf("%w: inference command failed: %v", errRunnerUnavailable, err)
		}
	}

	var out Outputs
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Outputs{}, fmt.Errorf("%w: decode runner output: %v", errRunnerMalformed, err)
	}
	return out, nil
}

// helperErrorKind classifies an error the inference helper reported on stdout.
type helperErrorKind int

const (
	// helperErrorNone means no machine-readable error envelope was present.
	helperErrorNone helperErrorKind = iota
	// helperErrorUnavailable maps onto errRunnerUnavailable.
	helperErrorUnavailable
	// helperErrorMalformed maps onto errRunnerMalformed.
	helperErrorMalformed
	// helperErrorFailure maps onto a generic decision.ErrProviderFailure.
	helperErrorFailure
)

// runnerErrorEnvelope is the optional machine-readable error the inference
// helper may emit on stdout when it fails. It lets the adapter classify a helper
// failure precisely instead of defaulting every non-zero exit to "unavailable".
// stdout remains the single machine-readable channel; diagnostics stay on
// stderr.
type runnerErrorEnvelope struct {
	Error *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeHelperErrorKind inspects a failed helper's stdout for an error envelope.
func decodeHelperErrorKind(stdout []byte) helperErrorKind {
	var env runnerErrorEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil || env.Error == nil {
		return helperErrorNone
	}
	switch env.Error.Kind {
	case "malformed":
		return helperErrorMalformed
	case "unavailable":
		return helperErrorUnavailable
	case "failure":
		return helperErrorFailure
	default:
		return helperErrorNone
	}
}

// childEnv returns the environment for the inference command: the current
// process environment plus JULIA_MODEL_PATH when a model path is configured.
func (r *CommandRunner) childEnv() []string {
	env := os.Environ()
	if strings.TrimSpace(r.ModelPath) != "" {
		env = append(env, "JULIA_MODEL_PATH="+r.ModelPath)
	}
	return env
}
