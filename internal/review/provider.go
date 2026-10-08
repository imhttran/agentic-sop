package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/normalize"
)

// reviewOutputSchema is the output contract for the REVIEW capability.
const reviewOutputSchema = `Return JSON only (no prose, no markdown):
{"summary": string, "findings": [{"severity": "INFO|LOW|MEDIUM|HIGH|CRITICAL", "title": string, "detail": string, "file": string, "line": number, "suggestion": string}]}
Report only real findings; use severity CRITICAL/HIGH only for correctness, safety, or security problems.`

// reviewRetryBound is the small, fixed number of REVIEW-capability attempts made
// when a response cannot be parsed into a valid report. It is deliberately small:
// a malformed review response is a bounded outcome, not an unbounded loop. The
// bound counts total attempts, so a value of 3 is at most 3 REVIEW calls.
const reviewRetryBound = 3

// ErrMalformedSeverity reports a review finding whose severity is missing or not
// one of the allowed values. It is a deterministic verdict about a single finding:
// the finding is explicitly rejected and is never silently dropped. Findings that
// precede it and are otherwise valid are retained in the report returned alongside
// it, so a genuine HIGH/CRITICAL is never discarded merely because a sibling
// finding is malformed.
type ErrMalformedSeverity struct {
	// Index is the zero-based position of the finding in the response.
	Index int
	// Value is the raw severity string as returned; an empty string means the field
	// was absent or empty.
	Value string
}

func (e *ErrMalformedSeverity) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("review finding %d has an empty severity; the finding is rejected", e.Index)
	}
	return fmt.Sprintf("review finding %d has an invalid severity %q; the finding is rejected", e.Index, e.Value)
}

// ErrMalformedOutput reports that a valid review report could not be obtained from
// the REVIEW capability after the bounded retries. It preserves the raw response so
// the malformed output is never lost, and it is a bounded, classifiable outcome —
// never a PASS and never a silent fix-loop exit.
//
// Report carries every finding that COULD be parsed into the allowed severity set
// across the final attempt, so a genuine HIGH/CRITICAL finding emitted alongside a
// malformed one is still surfaced and still blocks at its severity. A zero Report
// (no valid findings) is itself a non-pass outcome: the caller must treat the
// malformed-output error as NEEDS_HUMAN, never as a clean review.
type ErrMalformedOutput struct {
	// Raw is the last raw response received, preserved verbatim.
	Raw string
	// Attempts is the number of REVIEW attempts made.
	Attempts int
	// Last is the underlying parse error from the final attempt.
	Last error
	// Report holds every finding that parsed with a valid severity on the final
	// attempt. It may carry blocking findings even though parsing failed overall.
	Report Report
}

func (e *ErrMalformedOutput) Error() string {
	msg := fmt.Sprintf("malformed review output after %d attempt(s): %v", e.Attempts, e.Last)
	if strings.TrimSpace(e.Raw) != "" {
		msg += fmt.Sprintf("; raw response: %s", e.Raw)
	}
	if len(e.Report.Findings) > 0 {
		msg += fmt.Sprintf("; %d valid finding(s) retained", len(e.Report.Findings))
	}
	return msg
}

func (e *ErrMalformedOutput) Unwrap() error { return e.Last }

// AgentProvider reviews by asking an agent for structured findings.
type AgentProvider struct {
	agent agent.Agent
}

// NewAgentProvider returns a Provider backed by the agent REVIEW capability.
func NewAgentProvider(a agent.Agent) *AgentProvider {
	return &AgentProvider{agent: a}
}

// Review requests the REVIEW capability and parses the JSON findings. A response
// that cannot be parsed into a valid report is retried up to a small fixed bound;
// when no valid report is obtained the outcome is a bounded ErrMalformedOutput that
// preserves the raw response AND every finding that parsed with a valid severity on
// the final attempt. It is never reported as a pass, and a retained HIGH/CRITICAL
// finding still blocks at its severity.
func (p *AgentProvider) Review(ctx context.Context, request Request) (Report, error) {
	var lastRaw string
	var lastErr error
	var lastReport Report
	for attempt := 1; attempt <= reviewRetryBound; attempt++ {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		resp, err := p.agent.Generate(ctx, agent.Request{
			Capability:         agent.Review,
			Task:               "Review the implementation for the change under review.",
			Input:              request.text(),
			OutputRequirements: reviewOutputSchema,
		})
		if err != nil {
			return Report{}, fmt.Errorf("review agent: %w", err)
		}
		lastRaw = resp.Content

		report, err := parseReport(resp.Content)
		if err == nil {
			return report, nil
		}
		lastErr = err
		lastReport = report
	}
	return lastReport, &ErrMalformedOutput{Raw: lastRaw, Attempts: reviewRetryBound, Last: lastErr, Report: lastReport}
}

type reportWire struct {
	Summary  string `json:"summary"`
	Findings []struct {
		Severity   string `json:"severity"`
		Title      string `json:"title"`
		Detail     string `json:"detail"`
		File       string `json:"file"`
		Line       int    `json:"line"`
		Suggestion string `json:"suggestion"`
	} `json:"findings"`
}

// parseReport deterministically parses an agent review response. Every finding
// whose severity is in the allowed set (INFO, LOW, MEDIUM, HIGH, CRITICAL) is
// retained and returned in the Report, even when a sibling finding is malformed. An
// invalid or empty severity is rejected explicitly and deterministically as an
// *ErrMalformedSeverity — never silently dropped and never tolerated as a pass.
//
// The returned Report always contains every finding that parsed with a valid
// severity, so a caller that receives a non-nil error alongside a Report with
// blocking findings (a genuine HIGH/CRITICAL beside a malformed sibling) still sees
// them and can still block at their severity.
func parseReport(content string) (Report, error) {
	data, err := normalize.JSON(content)
	if err != nil {
		return Report{}, fmt.Errorf("parse review response: %w", err)
	}
	var wire reportWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return Report{}, fmt.Errorf("parse review response: %w", err)
	}

	report := Report{Summary: wire.Summary, Findings: make([]Finding, 0, len(wire.Findings))}
	var malformed *ErrMalformedSeverity
	for i, f := range wire.Findings {
		severity := Severity(f.Severity)
		if !severity.Valid() {
			// Reject the malformed finding deterministically, but do NOT discard the
			// valid findings already collected: retain them so a genuine HIGH/CRITICAL
			// in the same response is never silently dropped. The first malformed
			// finding is recorded as the authoritative rejection.
			if malformed == nil {
				malformed = &ErrMalformedSeverity{Index: i, Value: f.Severity}
			}
			continue
		}
		report.Findings = append(report.Findings, Finding{
			Severity:   severity,
			Title:      f.Title,
			Detail:     f.Detail,
			File:       f.File,
			Line:       f.Line,
			Suggestion: f.Suggestion,
		})
	}
	if malformed != nil {
		return report, malformed
	}
	return report, nil
}

// IsMalformedOutput reports whether err is the bounded malformed-review-output
// outcome. Callers use it to classify the outcome as NEEDS_HUMAN; it never maps to
// a pass.
func IsMalformedOutput(err error) bool {
	var target *ErrMalformedOutput
	return errors.As(err, &target)
}
