package evaltarget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// decisionEnvelope is the JSON shape the model must emit. The decision lives under
// a single key so the observer can reject any extra top-level structure.
type decisionEnvelope struct {
	Decision *ToolDecision `json:"decision"`
}

// StrictDecisionObserver parses one structured tool decision from model output.
// It accepts only one JSON object with a decision member. Anything else is a
// malformed output and a NOT_RUN result.
type StrictDecisionObserver struct{}

// Observe parses the raw output. It returns an error for empty output, output
// over the size bound (checked by the caller), missing JSON, a missing decision,
// trailing data, or a decision that fails validation.
func (StrictDecisionObserver) Observe(raw string) (ToolDecision, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ToolDecision{}, errors.New("live: model produced no output")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var env decisionEnvelope
	if err := dec.Decode(&env); err != nil {
		return ToolDecision{}, fmt.Errorf("live: malformed decision output: %w", err)
	}
	// A top-level decoder reports More only inside an open array or object, so
	// decode one more value to detect trailing top-level data.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		return ToolDecision{}, errors.New("live: malformed decision output: trailing data after the decision")
	}
	if env.Decision == nil {
		return ToolDecision{}, errors.New("live: malformed decision output: no decision member")
	}
	if err := env.Decision.Validate(); err != nil {
		return ToolDecision{}, err
	}
	return *env.Decision, nil
}

// ObserverFunc adapts a function to DecisionObserver. Fixtures use it to inject
// an observer failure.
type ObserverFunc func(raw string) (ToolDecision, error)

// Observe calls f.
func (f ObserverFunc) Observe(raw string) (ToolDecision, error) { return f(raw) }

// DeciderFunc adapts a function to Decider so fixtures can supply a canned or
// failing decision producer.
type DeciderFunc func(ctx context.Context) (string, error)

// Decide calls f.
func (f DeciderFunc) Decide(ctx context.Context) (string, error) { return f(ctx) }

// captureDecision asks the decider for one output, applies the output-size bound,
// and runs the observer. Every failure returns an error and never a decision.
func captureDecision(ctx context.Context, decider Decider, observer DecisionObserver, maxBytes int) (ToolDecision, string, error) {
	if decider == nil {
		return ToolDecision{}, "", errors.New("live: no decision producer configured")
	}
	if observer == nil {
		return ToolDecision{}, "", errors.New("live: no decision observer configured")
	}
	raw, err := decider.Decide(ctx)
	if err != nil {
		return ToolDecision{}, raw, fmt.Errorf("live: decision producer failed: %w", err)
	}
	if len(raw) > maxBytes {
		return ToolDecision{}, raw, fmt.Errorf("live: model output of %d bytes exceeds the %d byte bound", len(raw), maxBytes)
	}
	decision, err := observer.Observe(raw)
	if err != nil {
		return ToolDecision{}, raw, fmt.Errorf("live: observer failed: %w", err)
	}
	return decision, raw, nil
}
