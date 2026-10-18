package security

import (
	"fmt"
	"sort"
	"strings"
)

// Policy is the deterministic security gate policy. It is separate from adapter
// selection: adapters produce evidence, policy decides the verdict.
type Policy struct {
	Enabled            bool     `json:"enabled"`
	Mode               string   `json:"mode"`
	Scope              string   `json:"scope"`
	RequiredCategories []string `json:"required_categories,omitempty"`
	FailOn             []string `json:"fail_on"`
	UnknownSeverity    string   `json:"unknown_severity"`
	// TimeoutSeconds and MaxOutputBytes are execution configuration. They are part
	// of the policy identity so changed bounds cannot silently reuse old evidence.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	MaxOutputBytes int `json:"max_output_bytes,omitempty"`
	// Exclude is the effective declared scope boundary. It is part of the policy
	// identity so a changed exclusion list cannot reuse prior evidence.
	Exclude []string `json:"exclude,omitempty"`
	// AdvisoryDatabasePath is the local advisory database path a dependency
	// adapter uses. It is part of the policy identity so a changed database
	// cannot reuse prior evidence.
	AdvisoryDatabasePath string `json:"advisory_database_path,omitempty"`
	// StaleAdvisoryDatabase is the policy for a dependency scan whose advisory
	// database is stale. The zero value is deny: a stale database never yields
	// PASS. Set it to SecurityStaleAllow ("allow") to accept stale evidence with
	// an explicit PARTIAL coverage diagnostic. It is part of the policy identity.
	StaleAdvisoryDatabase string `json:"stale_advisory_database,omitempty"`
	// Auto enables automatic dependency adapter selection. It is configuration
	// policy only and does not affect the verdict.
	Auto bool `json:"auto,omitempty"`
}

// Stale advisory database policy values. The empty value means deny.
const (
	StaleDeny  = "deny"
	StaleAllow = "allow"
)

// StalePolicy resolves the stale advisory database policy. An empty value and
// any unrecognized value resolve to deny, so a stale database is fail-closed.
func (p Policy) StalePolicy() string {
	switch strings.ToLower(strings.TrimSpace(p.StaleAdvisoryDatabase)) {
	case StaleAllow:
		return StaleAllow
	default:
		return StaleDeny
	}
}

// Evaluation is the policy result. Findings remain in the report even when a
// baseline or suppression matched them.
type Evaluation struct {
	Verdict     Verdict
	Coverage    Coverage
	Reasons     []string
	Findings    []Finding
	Diagnostics []string
}

// EvaluatePolicy applies the matrix:
//
//   - disabled: NOT_APPLICABLE;
//   - a blocking unsuppressed finding: FAIL, even when another adapter is
//     incomplete — a real blocking finding must not be masked by coverage state;
//   - an unknown unsuppressed severity follows the explicit unknown policy;
//   - required mode requires every required category to have a COMPLETED,
//     COMPLETE-coverage adapter and a full workspace scope, otherwise
//     INDETERMINATE;
//   - any other failed or incomplete adapter is INDETERMINATE;
//   - otherwise: PASS.
func EvaluatePolicy(execs []Execution, policy Policy) Evaluation {
	out := Evaluation{Verdict: VerdictIndeterminate, Coverage: CoverageUnknown}
	if !policy.Enabled {
		out.Verdict = VerdictNotApplicable
		out.Reasons = []string{"security add-on is disabled"}
		return out
	}
	out.Coverage = aggregateCoverage(execs)
	for _, exec := range execs {
		out.Findings = append(out.Findings, exec.Findings...)
	}
	out.Findings = sortFindings(out.Findings)

	if len(execs) == 0 {
		out.Verdict = VerdictIndeterminate
		out.Reasons = append(out.Reasons, "security add-on is enabled but no adapter produced evidence")
		return out
	}

	// Collect incompleteness first, but do not let it mask a blocking finding. A
	// scan that found a real issue is FAIL even when coverage is also partial; the
	// coverage state remains explicit in the evaluation.
	var incompleteReasons []string
	for _, exec := range execs {
		if exec.Status != StatusCompleted {
			incompleteReasons = append(incompleteReasons, fmt.Sprintf("adapter %s is %s", exec.AdapterID, exec.Status))
		}
		if exec.Coverage != CoverageComplete {
			incompleteReasons = append(incompleteReasons, fmt.Sprintf("adapter %s coverage is %s", exec.AdapterID, exec.Coverage))
		}
	}
	if policy.Mode == "required" && policy.Scope != "" && policy.Scope != "workspace" {
		incompleteReasons = append(incompleteReasons, "required security gate cannot pass on changed-files evidence; run a full workspace scan")
	}
	// A stale advisory database is fail-closed. With the default deny policy it
	// forces INDETERMINATE, never PASS. An explicit allow records PARTIAL coverage
	// and its diagnostic, but still cannot satisfy a required gate.
	var staleReasons []string
	staleDenied := false
	for _, exec := range execs {
		if !exec.Provenance.AdvisoryDatabaseStale {
			continue
		}
		reason := fmt.Sprintf("adapter %s advisory database is stale", exec.AdapterID)
		if policy.StalePolicy() == StaleAllow {
			staleReasons = append(staleReasons, "PARTIAL coverage: "+reason+" (policy: allow)")
			// Allowing the tool to run against a stale database records explicit
			// PARTIAL coverage; it is not a gate bypass. Required mode still cannot
			// pass on incomplete advisory evidence.
			incompleteReasons = append(incompleteReasons, reason+" (policy: allow; coverage is PARTIAL)")
		} else {
			staleDenied = true
			incompleteReasons = append(incompleteReasons, reason+" (policy: deny)")
		}
	}
	if len(staleReasons) > 0 {
		out.Coverage = CoveragePartial
		// Record the explicit stale-allow diagnostic regardless of the final
		// verdict, so an incomplete or failing scan still explains its coverage.
		out.Reasons = append(out.Reasons, staleReasons...)
	}
	if policy.Mode == "required" {
		for _, category := range policy.RequiredCategories {
			found := false
			for _, exec := range execs {
				if string(exec.Category) != strings.ToLower(strings.TrimSpace(category)) {
					continue
				}
				found = true
				if exec.Status != StatusCompleted {
					incompleteReasons = append(incompleteReasons, fmt.Sprintf("required category %s adapter %s is %s", category, exec.AdapterID, exec.Status))
					continue
				}
				if exec.Coverage != CoverageComplete {
					incompleteReasons = append(incompleteReasons, fmt.Sprintf("required category %s adapter %s coverage is %s", category, exec.AdapterID, exec.Coverage))
				}
			}
			if !found {
				incompleteReasons = append(incompleteReasons, fmt.Sprintf("required category %s has no configured adapter", category))
			}
		}
	}

	blocking := 0
	unknown := 0
	for _, finding := range out.Findings {
		if finding.Suppressed {
			continue
		}
		if finding.Severity == SeverityUnknown {
			unknown++
			continue
		}
		if SeverityBlocked(policy.FailOn, finding.Severity) {
			blocking++
		}
	}
	if blocking > 0 {
		out.Verdict = VerdictFail
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) at a blocking severity", blocking))
		return out
	}
	if unknown > 0 {
		switch policy.UnknownSeverity {
		case "fail":
			out.Verdict = VerdictFail
			out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) have an unknown severity (policy: fail)", unknown))
			return out
		case "ignore":
			out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) have an unknown severity (policy: ignore)", unknown))
		default:
			out.Verdict = VerdictIndeterminate
			out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) have an unknown severity (policy: indeterminate)", unknown))
			return out
		}
	}
	if len(incompleteReasons) > 0 {
		out.Verdict = VerdictIndeterminate
		out.Reasons = append(out.Reasons, incompleteReasons...)
		return out
	}
	if staleDenied {
		// Defensive: a denied stale database is already an incomplete reason, so
		// this cannot be reached. Keep the verdict fail-closed anyway.
		out.Verdict = VerdictIndeterminate
		return out
	}
	out.Verdict = VerdictPass
	out.Reasons = append(out.Reasons, "no unsuppressed blocking findings")
	return out
}

func aggregateCoverage(execs []Execution) Coverage {
	if len(execs) == 0 {
		return CoverageUnknown
	}
	complete := 0
	partial := 0
	unknown := 0
	for _, exec := range execs {
		switch exec.Coverage {
		case CoverageComplete:
			complete++
		case CoveragePartial:
			partial++
		default:
			unknown++
		}
	}
	switch {
	case complete == len(execs):
		return CoverageComplete
	case partial > 0:
		return CoveragePartial
	default:
		return CoverageUnknown
	}
}

func sortFindings(findings []Finding) []Finding {
	out := append([]Finding(nil), findings...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		if out[i].AdapterID != out[j].AdapterID {
			return out[i].AdapterID < out[j].AdapterID
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out
}
