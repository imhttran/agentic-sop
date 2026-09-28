package quality

// JEV quality policy (JEV008).
//
// This file implements the initial JEV quality policy as a pure, deterministic
// classification over JEV findings. JEV is analysis, not authority: the policy
// maps each finding's severity to either a blocking outcome (which contributes to
// the quality gate's FAIL) or a report-only outcome (which is surfaced as
// evidence but never changes the verdict by itself).
//
// # Severity policy (PRD JEV008)
//
//   - HIGH and CRITICAL findings FAIL the quality gate by default.
//   - INFO, LOW, and MEDIUM findings are reported without blocking by default.
//
// The blocking severities are read from the quality gate's own policy
// (config.Quality.JEVFailOn, which defaults to quality.fail_on). This reuses the
// single existing severity policy instead of minting a second one.
//
// # Fail-closed semantics
//
// A malformed JEV result, or an INCOMPLETE/ERROR status, is never a pass: it is
// fail-closed evidence that contributes a non-clearing reason (see Evaluate).
// The policy below therefore never upgrades a fail-closed result to a report-only
// result. A JEV PASS contributes nothing and cannot manufacture a pass.
//
// # Lifecycle
//
// The policy introduces no lifecycle transition and no retry concept. A blocking
// JEV finding makes the existing gate FAIL; the existing fix loop and its budget
// (config.Quality.MaxFixCycles) are unchanged and remain authoritative, so a
// JEV-driven failure re-enters the same FIX lifecycle with no JEV-owned counter
// and no unbounded retry.

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/review"
)

// JEVPriority names how a JEV finding is treated by the quality gate.
type JEVPriority string

const (
	// JEVBlocking marks a finding whose severity fails the quality gate.
	JEVBlocking JEVPriority = "BLOCKING"
	// JEVReport marks a finding that is reported without blocking by default.
	JEVReport JEVPriority = "REPORT"
)

// JEVSeverityPriority classifies a single JEV finding severity under the given
// JEV fail_on list. It returns JEVBlocking when the severity is named in failOn
// and JEVReport otherwise (which is also the classification for INFO, LOW, and
// MEDIUM, and for any unset/empty failOn list, where nothing blocks).
func JEVSeverityPriority(sev string, failOn []string) JEVPriority {
	s := strings.ToUpper(strings.TrimSpace(sev))
	for _, b := range failOn {
		if strings.ToUpper(strings.TrimSpace(b)) == s {
			return JEVBlocking
		}
	}
	return JEVReport
}

// JEVReportFindings returns the human-readable reasons for the JEV findings that
// are report-only under the policy (JEVReport). They are additive evidence: the
// caller surfaces them in the gate's reasons for visibility, and they never flip
// the verdict on their own. A nil evidence or a fail-closed evidence yields no
// report reasons (a fail-closed evidence is handled as a blocking reason by
// Evaluate, not here).
func JEVReportFindings(failOn []string, ev *JEVEvidence) []string {
	if ev == nil || ev.FailClosed {
		return nil
	}
	var reasons []string
	for _, f := range ev.Findings {
		if JEVSeverityPriority(string(f.Severity), failOn) != JEVReport {
			continue
		}
		reasons = append(reasons, jevFindingReason(f.Severity, f.Category, f.Message))
	}
	return reasons
}

// jevFindingReason renders a report-only JEV finding as a single reason line.
func jevFindingReason(sev review.Severity, category, message string) string {
	desc := strings.TrimSpace(message)
	if c := strings.TrimSpace(category); c != "" {
		if desc != "" {
			desc = c + ": " + desc
		} else {
			desc = c
		}
	}
	if desc == "" {
		return "JEV report (" + string(sev) + ")"
	}
	return "JEV report (" + string(sev) + "): " + desc
}
