// Package decision is SOP's bounded decision boundary: a provider produces a
// choice with a confidence, and policy (outside the provider) decides what that
// choice means. Providers must never execute actions.
//
// The default provider is deterministic — simple, explainable rules — so the Jev
// experiment is opt-in and can be compared against rules before it is trusted
// (plan T037: enable it by default only if it provides measurable value).
package decision

import (
	"context"
	"fmt"
	"strings"
)

// Choice is a decision's outcome.
type Choice string

const (
	Low    Choice = "LOW"
	Medium Choice = "MEDIUM"
	High   Choice = "HIGH"
	Human  Choice = "HUMAN"
)

// Decision is a bounded decision: a choice, a confidence in [0,1], and optional
// metadata for auditability.
type Decision struct {
	Choice     Choice
	Confidence float64
	Metadata   map[string]string
}

// Request is the bounded context a provider decides on. Signals are numeric
// inputs (for example change size or file count); Subject is free text.
type Request struct {
	UseCase string
	Subject string
	Signals map[string]float64
}

// Provider produces a decision. Implementations must not execute actions.
type Provider interface {
	Name() string
	Decide(ctx context.Context, req Request) (Decision, error)
}

// NewProvider returns the provider named by configuration. "deterministic" is
// implemented; "jev" is recognized but not implemented, so an unsupported name
// fails clearly rather than silently falling back.
func NewProvider(name string) (Provider, error) {
	switch strings.TrimSpace(name) {
	case "", "deterministic":
		return DeterministicProvider{}, nil
	default:
		return nil, fmt.Errorf("decision: provider %q is not implemented (only \"deterministic\")", name)
	}
}

// DeterministicProvider classifies by explainable rules. It is the default.
type DeterministicProvider struct{}

// Name returns the provider name.
func (DeterministicProvider) Name() string { return "deterministic" }

// riskyKeywords mark work that tends to change behavior in dangerous ways.
var riskyKeywords = []string{"migration", "concurrency", "race", "security", "auth", "encrypt"}

// Decide classifies complexity from signals and the subject text.
func (DeterministicProvider) Decide(_ context.Context, req Request) (Decision, error) {
	hits := 0
	subject := strings.ToLower(req.Subject)
	for _, keyword := range riskyKeywords {
		if strings.Contains(subject, keyword) {
			hits++
		}
	}
	size := req.Signals["files"] + req.Signals["criteria"]

	switch {
	case hits > 0 || size >= 5:
		return Decision{
			Choice:     High,
			Confidence: 0.9,
			Metadata:   map[string]string{"reason": "risky subject or large change"},
		}, nil
	case size >= 2:
		return Decision{Choice: Medium, Confidence: 0.8, Metadata: map[string]string{"reason": "moderate size"}}, nil
	default:
		return Decision{Choice: Low, Confidence: 0.9, Metadata: map[string]string{"reason": "small change"}}, nil
	}
}

// Thresholds configure how a decision is applied. They live in configuration.
type Thresholds struct {
	// RouteToStrongModel: confidence below this uses the stronger model.
	RouteToStrongModel float64 `yaml:"route_to_strong_model" json:"route_to_strong_model"`
	// RequireHuman: confidence below this (or a HIGH choice) needs a human.
	RequireHuman float64 `yaml:"require_human" json:"require_human"`
}

// Target is where a decision routes: a model tier or a human.
type Target string

const (
	SmallModel  Target = "SMALL_MODEL"
	StrongModel Target = "STRONG_MODEL"
	HumanTarget Target = "HUMAN"
)

// Route maps a decision to a target using the thresholds. It is the shared
// policy for both model routing (T035) and review escalation (T036): a HIGH
// choice or low confidence needs a human; medium confidence uses the stronger
// model; otherwise the small model.
func Route(t Thresholds, d Decision) Target {
	if d.Choice == Human || d.Choice == High || d.Confidence < t.RequireHuman {
		return HumanTarget
	}
	if d.Choice == Medium || d.Confidence < t.RouteToStrongModel {
		return StrongModel
	}
	return SmallModel
}
