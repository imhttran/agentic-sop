// Package review provides SOP's structured review boundary. It evaluates an
// implementation before remote CI: the agent returns findings, and SOP (not the
// model) decides whether those findings block progress.
package review

import (
	"context"
	"strings"
)

// Severity is the seriousness of a review finding.
type Severity string

const (
	Info     Severity = "INFO"
	Low      Severity = "LOW"
	Medium   Severity = "MEDIUM"
	High     Severity = "HIGH"
	Critical Severity = "CRITICAL"
)

var severityRank = map[Severity]int{
	Info:     0,
	Low:      1,
	Medium:   2,
	High:     3,
	Critical: 4,
}

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool {
	_, ok := severityRank[s]
	return ok
}

// Finding is a single review finding.
type Finding struct {
	Severity   Severity
	Title      string
	Detail     string
	File       string
	Line       int
	Suggestion string
}

// Report is the outcome of a review.
type Report struct {
	Summary  string
	Findings []Finding
}

// Blocking reports whether any finding is at or above threshold. It is the only
// authority on whether a review blocks progress.
func (r Report) Blocking(threshold Severity) bool {
	min := severityRank[threshold]
	for _, f := range r.Findings {
		if severityRank[f.Severity] >= min {
			return true
		}
	}
	return false
}

// Request is the bounded context handed to a reviewer.
type Request struct {
	Task        string `json:"task"`
	Criteria    string `json:"criteria"`
	Diff        string `json:"diff"`
	TestSummary string `json:"test_summary"`
}

// Provider performs a structured review.
type Provider interface {
	Review(ctx context.Context, request Request) (Report, error)
}

// text renders the request into bounded review context.
func (r Request) text() string {
	parts := []string{}
	for _, p := range []string{r.Task, r.Criteria, r.TestSummary, r.Diff} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}
