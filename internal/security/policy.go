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
//   - required: every required category must have a COMPLETED, COMPLETE-coverage
//     adapter and the scope must be a full workspace scan, otherwise
//     INDETERMINATE;
//   - a blocking unsuppressed finding: FAIL;
//   - an unknown unsuppressed severity follows the explicit unknown policy;
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

	// A failed or incomplete scan is never a passing security gate, including in
	// report-only mode: report-only controls blocking, not the honesty of the
	// verdict.
	incomplete := false
	for _, exec := range execs {
		if exec.Status != StatusCompleted {
			incomplete = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("adapter %s is %s", exec.AdapterID, exec.Status))
		}
		if exec.Coverage != CoverageComplete {
			incomplete = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("adapter %s coverage is %s", exec.AdapterID, exec.Coverage))
		}
	}
	if incomplete {
		out.Verdict = VerdictIndeterminate
		return out
	}

	if policy.Mode == "required" && policy.Scope != "" && policy.Scope != "workspace" {
		out.Verdict = VerdictIndeterminate
		out.Reasons = append(out.Reasons, "required security gate cannot pass on changed-files evidence; run a full workspace scan")
		return out
	}

	failedCoverage := false
	if policy.Mode == "required" {
		for _, category := range policy.RequiredCategories {
			found := false
			for _, exec := range execs {
				if string(exec.Category) != strings.ToLower(strings.TrimSpace(category)) {
					continue
				}
				found = true
				if exec.Status != StatusCompleted {
					failedCoverage = true
					out.Reasons = append(out.Reasons, fmt.Sprintf("required category %s adapter %s is %s", category, exec.AdapterID, exec.Status))
					continue
				}
				if exec.Coverage != CoverageComplete {
					failedCoverage = true
					out.Reasons = append(out.Reasons, fmt.Sprintf("required category %s adapter %s coverage is %s", category, exec.AdapterID, exec.Coverage))
				}
			}
			if !found {
				failedCoverage = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("required category %s has no configured adapter", category))
			}
		}
	}
	if failedCoverage {
		out.Verdict = VerdictIndeterminate
		return out
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
		case "ignore":
			out.Verdict = VerdictPass
			out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) have an unknown severity (policy: ignore)", unknown))
		default:
			out.Verdict = VerdictIndeterminate
			out.Reasons = append(out.Reasons, fmt.Sprintf("%d unsuppressed finding(s) have an unknown severity (policy: indeterminate)", unknown))
		}
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
