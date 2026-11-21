package clef

// Evaluation-only single-attempt helper for the decision-model A/B benchmark.
//
// It reuses the PRODUCTION structured-decision path: the selected transport
// (SystemOne by default), the production request builder (BuildSystemOneRequest),
// and the production response parser (NormalizeSystemOneResponse). It performs
// exactly one HTTP request and never retries and never falls back, so the caller
// observes the first attempt exactly as the production provider returns it.
//
// Nothing in the production decision path calls DecideOnce.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// BenchOutcome is the result of exactly one production-shaped decision attempt.
type BenchOutcome struct {
	Result        decision.DecisionResult
	SchemaValid   bool
	Model         string
	Elapsed       time.Duration
	ErrorCategory string
	ErrorDetail   string
}

// DecideOnce sends exactly one production-shaped structured-decision request to
// the endpoint and normalizes the first response through SOP's own parser.
//
// It returns a non-nil error whenever the attempt did not produce a schema-valid
// decision. The outcome still carries a class-only error category and the exact
// diagnostic text so the caller can record the failure faithfully.
func DecideOnce(ctx context.Context, baseURL, model string, timeout time.Duration, req decision.DecisionRequest) (BenchOutcome, error) {
	start := time.Now()
	out := BenchOutcome{Model: model}

	cfg := Config{
		BaseURL:   baseURL,
		Model:     model,
		Timeout:   timeout,
		Enable:    true,
		Transport: TransportSystemOne,
	}
	res, err := New(cfg, nil).Decide(ctx, req)
	out.Elapsed = time.Since(start)
	if err != nil {
		out.ErrorCategory = benchErrorCategory(err)
		out.ErrorDetail = benchErrorDetail(err)
		return out, err
	}
	out.Result = res
	out.SchemaValid = true
	return out, nil
}

// benchErrorCategory maps a provider/transport failure to a short, class-only
// category. It never returns response prose, so a credential or a model payload
// cannot leak through a category.
func benchErrorCategory(err error) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return "timeout"
	}
	var he *httpStatusError
	if errors.As(err, &he) {
		return "http_error"
	}
	var pe *decision.ProviderError
	if errors.As(err, &pe) {
		switch pe.Kind {
		case decision.KindUnavailable:
			return "unavailable"
		case decision.KindMalformedResponse:
			return "malformed"
		case decision.KindInvalidRequest:
			return "invalid_request"
		default:
			return "provider_error"
		}
	}
	if isMalformedResponse(err) || errors.Is(err, decision.ErrMalformedResponse) {
		return "malformed"
	}
	if isUnavailable(err) || errors.Is(err, decision.ErrUnavailable) {
		return "unavailable"
	}
	if errors.Is(err, decision.ErrInvalidRequest) {
		return "invalid_request"
	}
	return "provider_error"
}

// benchErrorDetail returns the exact diagnostic text for an attempt. For an HTTP
// status failure that includes the endpoint's error body (for example a
// model-load failure reported with 409).
func benchErrorDetail(err error) string {
	var he *httpStatusError
	if errors.As(err, &he) {
		return fmt.Sprintf("HTTP %d: %s", he.StatusCode, he.Body)
	}
	return err.Error()
}
