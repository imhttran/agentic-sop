package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/quality"
)

// JEV CLI reporting (JEV011).
//
// renderJEVReport turns a persisted JEV run artifact into a concise, deterministic
// block of CLI output. It is a read-only projection of the artifact: it renders
// reporting only, never transitions task state, never manufactures validation,
// review, or quality outcomes, and never re-runs JEV. It emits nothing when JEV
// did not run (doc == nil), so a disabled/absent JEV leaves the CLI output
// exactly as it was before JEV011.
//
// Reporting is derived entirely from the persisted artifact and the configured
// JEV fail_on severities, so repeated runs on the same artifact render the same
// text. PRD terminology is preserved verbatim: JEV, PASS, FINDINGS, INCOMPLETE,
// ERROR, and the severity values INFO/LOW/MEDIUM/HIGH/CRITICAL.

// jevMaxBlockingRendered bounds how many blocking findings are printed inline.
// Blocking findings beyond the bound are never dropped silently: the renderer
// appends a remainder line pointing at the full report.
const jevMaxBlockingRendered = 10

// jevMaxNonBlockingRendered bounds the inline listing of non-blocking findings.
// Non-blocking findings are always accessible through the JEV report path when
// one exists, so the inline listing stays incidental and never dominates.
const jevMaxNonBlockingRendered = 10

// renderJEVReport renders the concise JEV CLI block for a persisted artifact.
// It returns "" when JEV did not run. failOn names the blocking severities
// (config.Quality.JEVFailOn()); artifactPath is the repository-relative path of
// the persisted report, or "" when none was written.
func renderJEVReport(doc *jevRunDoc, failOn []string, artifactPath string) string {
	if doc == nil {
		return ""
	}

	blocking, nonBlocking := splitJEVFindings(doc.Findings, failOn)

	var b strings.Builder
	fmt.Fprintf(&b, "JEV: %s\n", jevStatusLabel(doc))

	if doc.FailClosed {
		reason := strings.TrimSpace(doc.Reason)
		if reason == "" {
			reason = "JEV analysis was not a clean pass"
		}
		fmt.Fprintf(&b, "  %s\n", reason)
	}

	// The blocking-finding count is authoritative: when the artifact persisted
	// the invocation's blocking-finding count, prefer it so the CLI agrees with
	// what the gate weighed rather than recomputing from the current config
	// (which may have changed since the artifact was written). Only fall back to
	// the render-time split when no persisted count is available.
	blockingCount := len(blocking)
	if doc.Metrics != nil {
		blockingCount = doc.Metrics.BlockingFindings
	}
	fmt.Fprintf(&b, "  %s\n", blockingFindingsLine(blockingCount))

	// Blocking findings carry severity and source location so they are
	// actionable in place. A bounded list keeps the output from dominating.
	writeJEVFindings(&b, blocking, jevMaxBlockingRendered)
	if extra := len(blocking) - jevMaxBlockingRendered; extra > 0 {
		fmt.Fprintf(&b, "  ... %d more blocking finding(s); see the JEV report\n", extra)
	}

	// Non-blocking findings are accessible but must not read as a failure.
	if len(nonBlocking) > 0 {
		fmt.Fprintf(&b, "  %d non-blocking finding(s)\n", len(nonBlocking))
		writeJEVFindings(&b, nonBlocking, jevMaxNonBlockingRendered)
		if extra := len(nonBlocking) - jevMaxNonBlockingRendered; extra > 0 {
			fmt.Fprintf(&b, "  ... %d more non-blocking finding(s); see the JEV report\n", extra)
		}
	}

	// The report path is diagnostic output only. It is shown when a report was
	// actually written (so full findings and evidence stay reachable) and never
	// fabricated when no artifact exists.
	if p := strings.TrimSpace(artifactPath); p != "" {
		fmt.Fprintf(&b, "  JEV report: %s\n", p)
	}

	return b.String()
}

// writeJEVReport renders the JEV block to w, if any. A nil document writes
// nothing, so a disabled/absent JEV adds no noise.
func writeJEVReport(w io.Writer, doc *jevRunDoc, failOn []string, artifactPath string) {
	if block := renderJEVReport(doc, failOn, artifactPath); block != "" {
		io.WriteString(w, block)
	}
}

// splitJEVFindings partitions persisted findings into blocking and non-blocking
// groups under the configured JEV fail_on severities, preserving the artifact's
// order within each group so rendering is deterministic. The split reuses the
// single JEV severity policy (quality.JEVSeverityPriority), so the CLI and the
// gate can never disagree about what blocks.
func splitJEVFindings(findings []jevFindingDoc, failOn []string) (blocking, nonBlocking []jevFindingDoc) {
	for _, f := range findings {
		if quality.JEVSeverityPriority(f.Severity, failOn) == quality.JEVBlocking {
			blocking = append(blocking, f)
		} else {
			nonBlocking = append(nonBlocking, f)
		}
	}
	return blocking, nonBlocking
}

// blockingFindingsLine renders the concise blocking-finding count line, e.g.
// "0 blocking findings" or "2 blocking findings". A negative count is clamped to
// zero so a malformed persisted value never renders a nonsensical line.
func blockingFindingsLine(n int) string {
	if n < 0 {
		n = 0
	}
	if n == 1 {
		return "1 blocking finding"
	}
	return fmt.Sprintf("%d blocking findings", n)
}

// jevStatusLabel renders the artifact's status verbatim for the summary line,
// preserving PRD terminology. A fail-closed artifact is never shown as PASS, and
// an empty or unrecognized status renders as ERROR rather than implying success:
// only the known PRD statuses PASS, FINDINGS, INCOMPLETE, and ERROR are ever
// shown as themselves.
func jevStatusLabel(doc *jevRunDoc) string {
	status := strings.ToUpper(strings.TrimSpace(doc.Status))
	if doc.FailClosed && status == "PASS" {
		return "ERROR"
	}
	switch status {
	case "PASS", "FINDINGS", "INCOMPLETE", "ERROR":
		return status
	default:
		// An unrecognized or empty status is never a pass signal: report it
		// fail-closed as ERROR rather than echoing an unexpected string.
		return "ERROR"
	}
}

// writeJEVFindings renders up to limit findings, each on one line carrying its
// severity and source location (file:line) when known. A finding without a file
// or line renders without misleading location data and is still marked by its
// severity. The output is bounded by limit so a long list never dominates.
func writeJEVFindings(b *strings.Builder, findings []jevFindingDoc, limit int) {
	if limit > len(findings) {
		limit = len(findings)
	}
	for _, f := range findings[:limit] {
		fmt.Fprintf(b, "  - [%s] %s\n", jevSeverityLabel(f.Severity), jevFindingLocation(f))
	}
}

// jevSeverityLabel renders a finding severity verbatim (upper-cased to match PRD
// terminology). An unknown or empty severity renders as-is rather than being
// silently relabeled.
func jevSeverityLabel(sev string) string {
	s := strings.ToUpper(strings.TrimSpace(sev))
	if s == "" {
		return "UNKNOWN"
	}
	return s
}

// jevFindingLocation renders a finding's message with its source location. The
// location (file:line) is prefixed only when known, so a finding with no file or
// line does not carry misleading location data.
func jevFindingLocation(f jevFindingDoc) string {
	msg := strings.TrimSpace(f.Finding)
	if msg == "" {
		msg = "(no description)"
	}
	file := strings.TrimSpace(f.File)
	switch {
	case file != "" && f.Line > 0:
		return fmt.Sprintf("%s:%d %s", file, f.Line, msg)
	case file != "":
		return fmt.Sprintf("%s %s", file, msg)
	case f.Line > 0:
		return fmt.Sprintf("line %d %s", f.Line, msg)
	default:
		return msg
	}
}
