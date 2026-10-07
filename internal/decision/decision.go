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
	"errors"
	"fmt"
	"math"
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

// Known reports whether c is one of the governance-meaningful choices. An
// unknown/indeterminate choice (for example a value a provider invented) is not
// an approval and must never be routed as one. It is exported so the SOP-owned
// validator (ValidateResult) and a provider-neutral adapter share one definition
// of the known-choice set.
func (c Choice) Known() bool {
	switch c {
	case Low, Medium, High, Human:
		return true
	default:
		return false
	}
}

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
	// Choices, when non-empty, is the closed set of choice values the caller
	// permits for this request. It is optional and provider-neutral, and it
	// carries no authority: the validator enforces membership, and SOP policy
	// still decides what a valid choice means.
	Choices []Choice
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

// ValidConfidence reports whether a confidence is a usable probability in [0,1].
// NaN, infinities, negatives, and values above 1 are invalid and must never be
// treated as a confident authorization. It is the SOP-owned, fail-closed
// boundary rule; a provider is never trusted to validate its own result.
func ValidConfidence(c float64) bool {
	return !math.IsNaN(c) && !math.IsInf(c, 0) && c >= 0 && c <= 1
}

// Route maps a decision to a target using the thresholds. It is the shared
// policy for both model routing (T035) and review escalation (T036): a HIGH
// choice or low confidence needs a human; medium confidence uses the stronger
// model; otherwise the small model.
//
// Route fails closed. A decision with an unknown/indeterminate choice or an
// invalid confidence (outside [0,1], including NaN) cannot be interpreted by
// policy, so it is escalated to a human: the boundary never converts an unknown
// result into an approval, never silently downgrades risk, and never authorizes
// execution from a provider's confidence alone. Only a known choice with a valid
// confidence may reach a model tier.
func Route(t Thresholds, d Decision) Target {
	// Fail closed on anything policy cannot interpret. An unknown choice is not an
	// approval; an out-of-range confidence is not a confident authorization.
	if !d.Choice.Known() || !ValidConfidence(d.Confidence) {
		return HumanTarget
	}
	if d.Choice == Human || d.Choice == High || d.Confidence < t.RequireHuman {
		return HumanTarget
	}
	if d.Choice == Medium || d.Confidence < t.RouteToStrongModel {
		return StrongModel
	}
	return SmallModel
}

// ContractVersion is the provider-neutral SEAM-002 contract version. It is the
// version a provider result must declare to be interpretable by SOP.
const ContractVersion = 1

// Status is the provider-neutral result status (SEAM-002). It is a result value,
// never a lifecycle action. The vocabulary is closed; an unknown status fails
// closed at validation.
type Status string

const (
	// StatusOK: the provider completed and returned a usable choice.
	StatusOK Status = "OK"
	// StatusUnsupported: the provider explicitly cannot serve the requested kind.
	StatusUnsupported Status = "UNSUPPORTED"
	// StatusError: the provider reports a failure it observed.
	StatusError Status = "ERROR"
	// StatusIndeterminate: the provider could not decide; this is never a pass.
	StatusIndeterminate Status = "INDETERMINATE"
)

// Valid reports whether s is a known result status.
func (s Status) Valid() bool {
	switch s {
	case StatusOK, StatusUnsupported, StatusError, StatusIndeterminate:
		return true
	default:
		return false
	}
}

// Result is the Go mirror of the SEAM-002 provider-neutral result DTO. It
// carries evidence only: no lifecycle, approval, commit, merge, or policy
// action. Diagnostics is advisory and MUST NOT be read by SOP policy.
type Result struct {
	ContractVersion int               `json:"contract_version"`
	Status          Status            `json:"status"`
	Kind            string            `json:"kind,omitempty"`
	Choice          Choice            `json:"choice,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	Diagnostics     map[string]string `json:"diagnostics,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// Fail-closed validation sentinels. They classify a provider result that SOP
// cannot safely turn into evidence; none of them is ever an approval.
var (
	// ErrInvalidResult: a malformed or contract-violating result (unknown
	// version/status/choice, a choice outside the allowed set, an invalid
	// confidence, a mismatched kind, or an inconsistent status/payload pair).
	ErrInvalidResult = errors.New("decision: invalid provider result")
	// ErrUnsupportedCapability: the provider explicitly cannot serve the kind.
	ErrUnsupportedCapability = errors.New("decision: unsupported capability")
	// ErrIndeterminate: the result states no confidence, or is indeterminate.
	ErrIndeterminate = errors.New("decision: indeterminate result")
	// ErrProviderResult: the provider reported a failure it observed.
	ErrProviderResult = errors.New("decision: provider reported an error")
	// ErrProviderFailure: a transport/process failure produced no usable result.
	ErrProviderFailure = errors.New("decision: provider failure")
)

// Validate reports whether a decision is a usable, SOP-recognized decision: a
// known choice with a finite confidence in [0,1]. It fails closed. It is the
// SOP-owned boundary check; it never reads prose and never infers authority.
func Validate(d Decision) error {
	if !d.Choice.Known() {
		return fmt.Errorf("%w: unknown choice %q", ErrInvalidResult, d.Choice)
	}
	if !ValidConfidence(d.Confidence) {
		return fmt.Errorf("%w: confidence is not a finite value in [0,1]", ErrInvalidResult)
	}
	return nil
}

// ValidateResult applies SOP-owned, fail-closed validation to a provider result
// produced for this request (SEAM-002). A nil error means the result is usable,
// confident evidence. Otherwise it returns a typed sentinel:
//
//   - ErrInvalidResult: unknown contract version or status, a missing required
//     field, an unknown choice, a choice outside the request's allowed set, a
//     confidence that is present but not a finite value in [0,1], a kind that
//     does not match the request, or a success status carrying an error.
//   - ErrUnsupportedCapability: the provider explicitly cannot serve the kind.
//   - ErrProviderResult: the provider reported an error it observed.
//   - ErrIndeterminate: the provider stated no confidence, or returned an
//     indeterminate result.
//
// A provider is never trusted to validate its own authority: this function
// inspects only the bounded result data, and it contains no provider-specific
// logic. Interpretation of a valid result is a separate SOP policy step.
func (r Request) ValidateResult(res Result) error {
	if res.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: unsupported contract version %d", ErrInvalidResult, res.ContractVersion)
	}
	if !res.Status.Valid() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidResult, res.Status)
	}
	if k := strings.TrimSpace(res.Kind); k != "" && k != strings.TrimSpace(r.UseCase) {
		return fmt.Errorf("%w: kind %q does not match request %q", ErrInvalidResult, res.Kind, r.UseCase)
	}
	switch res.Status {
	case StatusUnsupported:
		return ErrUnsupportedCapability
	case StatusError:
		if msg := strings.TrimSpace(res.Error); msg != "" {
			return fmt.Errorf("%w: %s", ErrProviderResult, msg)
		}
		return ErrProviderResult
	case StatusIndeterminate:
		return ErrIndeterminate
	}
	// status == OK
	if strings.TrimSpace(res.Error) != "" {
		return fmt.Errorf("%w: OK result carries an error", ErrInvalidResult)
	}
	if !res.Choice.Known() {
		return fmt.Errorf("%w: unknown choice %q", ErrInvalidResult, res.Choice)
	}
	if len(r.Choices) > 0 {
		allowed := false
		for _, c := range r.Choices {
			if c == res.Choice {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: choice %q is not in the allowed set", ErrInvalidResult, res.Choice)
		}
	}
	if res.Confidence == nil {
		return ErrIndeterminate
	}
	if !ValidConfidence(*res.Confidence) {
		return fmt.Errorf("%w: confidence is not a finite value in [0,1]", ErrInvalidResult)
	}
	return nil
}
