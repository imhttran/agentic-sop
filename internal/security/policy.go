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
	// AdvisoryDatabasePath is the local cargo-audit-style advisory database path.
	// It is part of the policy identity so a changed database cannot reuse prior
	// evidence.
	AdvisoryDatabasePath string `json:"advisory_database_path,omitempty"`
	// OSVDatabasePath is the separate OSV offline cache root. It is never used as
	// a cargo-audit database and vice versa.
	OSVDatabasePath string `json:"osv_database_path,omitempty"`
	// StaleAdvisoryDatabase is the policy for a dependency scan whose advisory
	// database is stale. The zero value is deny: a stale database never yields
	// PASS. Set it to SecurityStaleAllow ("allow") to accept stale evidence with
	// an explicit PARTIAL coverage diagnostic. It is part of the policy identity.
	StaleAdvisoryDatabase string `json:"stale_advisory_database,omitempty"`
	// Auto enables automatic dependency adapter selection. It is configuration
	// policy only and does not affect the verdict.
	Auto bool `json:"auto,omitempty"`
	// RequiredGateActive marks a configured required gate that remains active even
	// when the add-on is globally disabled. Global disable must not bypass it.
	RequiredGateActive bool `json:"required_gate_active,omitempty"`
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

// EvaluatePolicy applies the policy without repository language detection. It is
// retained for category-level callers; the runner uses
// EvaluatePolicyWithDetection.
func EvaluatePolicy(execs []Execution, policy Policy) Evaluation {
	return EvaluatePolicyWithDetection(execs, policy, nil)
}

// EvaluatePolicyWithDetection applies the policy matrix:
//
//   - disabled: NOT_APPLICABLE, unless a required gate is active (INDETERMINATE);
//   - a blocking unsuppressed finding: FAIL, even when another adapter is
//     incomplete — a real blocking finding must not be masked by coverage state;
//   - an unknown unsuppressed severity follows the explicit unknown policy;
//   - required mode requires every required category, and every detected language
//     for a language-scoped category, to have a COMPLETED, COMPLETE-coverage
//     adapter; a category-level success cannot hide a missing language;
//   - explicitly disabled coverage that is not replaced is a reported gap and is
//     never treated as clean;
//   - any other failed or incomplete adapter is INDETERMINATE;
//   - otherwise: PASS.
func EvaluatePolicyWithDetection(execs []Execution, policy Policy, detection *Detection) Evaluation {
	out := Evaluation{Verdict: VerdictIndeterminate, Coverage: CoverageUnknown}
	if !policy.Enabled {
		if policy.RequiredGateActive {
			out.Verdict = VerdictIndeterminate
			out.Reasons = []string{"security add-on is globally disabled but an active required gate is configured; global disable is not a required-gate exemption"}
			return out
		}
		out.Verdict = VerdictNotApplicable
		out.Reasons = []string{"security add-on is disabled"}
		return out
	}
	out.Coverage = aggregateCoverage(execs)
	for _, exec := range execs {
		out.Findings = append(out.Findings, exec.Findings...)
	}
	out.Findings = sortFindings(out.Findings)

	enabledExecs := nonDisabled(execs)
	if len(enabledExecs) == 0 {
		if len(execs) > 0 {
			out.Verdict = VerdictIndeterminate
			out.Reasons = append(out.Reasons, "security add-on is enabled but every adapter is disabled")
		} else {
			out.Verdict = VerdictIndeterminate
			out.Reasons = append(out.Reasons, "security add-on is enabled but no adapter produced evidence")
		}
		return out
	}

	// Collect incompleteness first, but do not let it mask a blocking finding. A
	// scan that found a real issue is FAIL even when coverage is also partial; the
	// coverage state remains explicit in the evaluation.
	var incompleteReasons []string
	for _, exec := range enabledExecs {
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
	for _, exec := range enabledExecs {
		if !exec.Provenance.AdvisoryDatabaseStale {
			continue
		}
		reason := fmt.Sprintf("adapter %s advisory database is stale", exec.AdapterID)
		if policy.StalePolicy() == StaleAllow {
			staleReasons = append(staleReasons, "PARTIAL coverage: "+reason+" (policy: allow)")
			incompleteReasons = append(incompleteReasons, reason+" (policy: allow; coverage is PARTIAL)")
		} else {
			staleDenied = true
			incompleteReasons = append(incompleteReasons, reason+" (policy: deny)")
		}
	}
	if len(staleReasons) > 0 {
		out.Coverage = CoveragePartial
		out.Reasons = append(out.Reasons, staleReasons...)
	}

	// Explicitly disabled coverage is a gap, not a clean scan. It is reported even
	// in report-only mode, but only when the disabled coverage is not replaced by
	// an enabled adapter.
	disabledGaps := disabledCoverageGaps(execs, detection)
	if len(disabledGaps) > 0 {
		out.Coverage = CoveragePartial
		out.Reasons = append(out.Reasons, disabledGaps...)
	}

	if policy.Mode == "required" {
		incompleteReasons = append(incompleteReasons, requiredCoverageReasons(enabledExecs, policy, detection)...)
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
	if len(disabledGaps) > 0 {
		out.Verdict = VerdictIndeterminate
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

// requiredCoverageReasons enforces required categories and, when detection is
// available, every detected language independently. A category-level success in
// Go cannot satisfy a missing Rust obligation.
func requiredCoverageReasons(execs []Execution, policy Policy, detection *Detection) []string {
	var reasons []string
	for _, raw := range policy.RequiredCategories {
		category := Category(strings.ToLower(strings.TrimSpace(raw)))
		var inCategory []Execution
		for _, exec := range execs {
			if exec.Category == category {
				inCategory = append(inCategory, exec)
			}
		}
		if len(inCategory) == 0 {
			reasons = append(reasons, fmt.Sprintf("required category %s has no enabled adapter evidence", category))
			continue
		}
		if category == CategorySecrets || detection == nil || len(detection.Languages) == 0 {
			covered := false
			for _, exec := range inCategory {
				if exec.Status == StatusCompleted && exec.Coverage == CoverageComplete {
					covered = true
					break
				}
			}
			if !covered {
				reasons = append(reasons, fmt.Sprintf("required category %s has no COMPLETED adapter with COMPLETE coverage", category))
			}
			continue
		}
		for _, language := range detection.Languages {
			covered := false
			undeclared := false
			for _, exec := range inCategory {
				if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
					continue
				}
				if exec.RepositoryWide {
					covered = true
					break
				}
				if len(exec.Languages) == 0 {
					undeclared = true
					continue
				}
				if containsLanguage(exec.Languages, language) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			if undeclared {
				reasons = append(reasons, fmt.Sprintf("required category %s adapter language coverage is undeclared; cannot prove %s coverage", category, language))
				continue
			}
			reasons = append(reasons, fmt.Sprintf("required category %s has no COMPLETE %s coverage", category, language))
		}
	}
	return reasons
}

// disabledCoverageGaps reports explicit disables that were not replaced by an
// enabled adapter. A disabled adapter with an enabled replacement is not a gap.
func disabledCoverageGaps(execs []Execution, detection *Detection) []string {
	var enabled []Execution
	var disabled []Execution
	for _, exec := range execs {
		if exec.Disabled {
			disabled = append(disabled, exec)
			continue
		}
		enabled = append(enabled, exec)
	}
	if len(disabled) == 0 {
		return nil
	}
	var reasons []string
	for _, exec := range disabled {
		if exec.Category == CategorySecrets {
			if !hasEnabledCoverage(enabled, exec.Category, "", true) {
				reasons = append(reasons, fmt.Sprintf("disabled coverage: %s was disabled and no enabled %s adapter replaces it", exec.AdapterID, exec.Category))
			}
			continue
		}
		languages := exec.Languages
		if len(languages) == 0 && detection != nil {
			languages = detection.Languages
		}
		if len(languages) == 0 {
			if !hasEnabledCoverage(enabled, exec.Category, "", false) {
				reasons = append(reasons, fmt.Sprintf("disabled coverage: %s was disabled and no enabled %s adapter replaces it", exec.AdapterID, exec.Category))
			}
			continue
		}
		for _, language := range languages {
			if !hasEnabledCoverage(enabled, exec.Category, language, false) {
				reasons = append(reasons, fmt.Sprintf("disabled coverage: %s was disabled and no enabled %s adapter covers %s", exec.AdapterID, exec.Category, language))
			}
		}
	}
	return reasons
}

func hasEnabledCoverage(execs []Execution, category Category, language Language, repositoryWide bool) bool {
	for _, exec := range execs {
		if exec.Category != category || exec.Disabled {
			continue
		}
		if repositoryWide {
			return exec.RepositoryWide
		}
		if exec.RepositoryWide || (language != "" && containsLanguage(exec.Languages, language)) {
			return true
		}
	}
	return false
}

func nonDisabled(execs []Execution) []Execution {
	out := make([]Execution, 0, len(execs))
	for _, exec := range execs {
		if !exec.Disabled {
			out = append(out, exec)
		}
	}
	return out
}

// CoverageByCategory is the post-scan coverage report. It reports every detected
// language independently and distinguishes covered, partial, disabled, missing,
// unsupported, failed, and unconfigured outcomes.
func CoverageByCategory(execs []Execution, detection *Detection, policy Policy) []CategoryCoverage {
	required := map[string]bool{}
	for _, raw := range policy.RequiredCategories {
		required[strings.ToLower(strings.TrimSpace(raw))] = true
	}
	var out []CategoryCoverage
	for _, category := range []Category{CategorySAST, CategorySecrets, CategoryDependencies} {
		coverage := CategoryCoverage{Category: category, Required: required[string(category)]}
		var inCategory []Execution
		for _, exec := range execs {
			if exec.Category == category {
				inCategory = append(inCategory, exec)
			}
		}
		if len(inCategory) == 0 {
			if !required[string(category)] {
				continue
			}
			coverage.State = CoverageStateMissing
			coverage.Reasons = append(coverage.Reasons, "required category has no enabled adapter evidence")
			if category != CategorySecrets && detection != nil && len(detection.Languages) > 0 {
				for _, language := range detection.Languages {
					coverage.Languages = append(coverage.Languages, LanguageCoverage{
						Language: language, State: CoverageStateMissing,
						Reasons: []string{"required language coverage has no enabled adapter"},
					})
				}
				coverage.State = aggregateLanguageState(coverage.Languages)
			}
			out = append(out, coverage)
			continue
		}
		if category == CategorySecrets {
			coverage.RepositoryWide = true
			coverage.State, coverage.Reasons = repositoryCoverage(inCategory)
			out = append(out, coverage)
			continue
		}
		languages := detectedLanguages(detection, inCategory)
		if len(languages) == 0 {
			coverage.State, coverage.Reasons = repositoryCoverage(inCategory)
			out = append(out, coverage)
			continue
		}
		for _, language := range languages {
			lc := LanguageCoverage{Language: language}
			var covering []Execution
			var coveringDisabled []Execution
			for _, exec := range inCategory {
				if exec.RepositoryWide || containsLanguage(exec.Languages, language) {
					if exec.Disabled {
						coveringDisabled = append(coveringDisabled, exec)
					} else {
						covering = append(covering, exec)
					}
				}
			}
			if len(covering) > 0 {
				lc.State, lc.Reasons = languageCoverageState(covering)
				for _, exec := range covering {
					lc.Adapters = append(lc.Adapters, exec.AdapterID)
				}
			} else if len(coveringDisabled) > 0 {
				lc.State = CoverageStateDisabled
				for _, exec := range coveringDisabled {
					lc.Adapters = append(lc.Adapters, exec.AdapterID)
				}
				lc.Reasons = append(lc.Reasons, "explicitly disabled coverage")
			} else if coverage.Required {
				lc.State = CoverageStateMissing
				lc.Reasons = append(lc.Reasons, "required language coverage has no enabled adapter")
			} else {
				lc.State = CoverageStateUnconfigured
			}
			sort.Strings(lc.Adapters)
			coverage.Languages = append(coverage.Languages, lc)
		}
		coverage.State = aggregateLanguageState(coverage.Languages)
		out = append(out, coverage)
	}
	return out
}

func detectedLanguages(detection *Detection, execs []Execution) []Language {
	set := map[Language]bool{}
	if detection != nil {
		for _, language := range detection.Languages {
			set[language] = true
		}
	}
	if len(set) == 0 {
		for _, exec := range execs {
			for _, language := range exec.Languages {
				set[language] = true
			}
		}
	}
	out := make([]Language, 0, len(set))
	for language := range set {
		out = append(out, language)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func repositoryCoverage(execs []Execution) (CoverageState, []string) {
	var enabled []Execution
	var disabled []Execution
	for _, exec := range execs {
		if exec.Disabled {
			disabled = append(disabled, exec)
			continue
		}
		enabled = append(enabled, exec)
	}
	if len(enabled) > 0 {
		return languageCoverageState(enabled)
	}
	if len(disabled) > 0 {
		var reasons []string
		for _, exec := range disabled {
			reasons = append(reasons, fmt.Sprintf("%s is disabled", exec.AdapterID))
		}
		return CoverageStateDisabled, reasons
	}
	return CoverageStateUnconfigured, nil
}

func languageCoverageState(execs []Execution) (CoverageState, []string) {
	var reasons []string
	completedComplete := false
	completedPartial := false
	failed := false
	skipped := false
	for _, exec := range execs {
		switch {
		case exec.Status == StatusCompleted && exec.Coverage == CoverageComplete:
			completedComplete = true
		case exec.Status == StatusCompleted && exec.Coverage == CoveragePartial:
			completedPartial = true
			reasons = append(reasons, fmt.Sprintf("%s coverage is PARTIAL", exec.AdapterID))
		case exec.Status == StatusUnavailable:
			failed = true
			reasons = append(reasons, fmt.Sprintf("%s is UNAVAILABLE", exec.AdapterID))
		case exec.Status == StatusFailed || exec.Status == StatusTimedOut || exec.Status == StatusCancelled:
			failed = true
			reasons = append(reasons, fmt.Sprintf("%s is %s", exec.AdapterID, exec.Status))
		default:
			skipped = true
			reasons = append(reasons, fmt.Sprintf("%s is %s", exec.AdapterID, exec.Status))
		}
	}
	switch {
	case completedComplete:
		return CoverageStateCovered, nil
	case completedPartial:
		return CoverageStatePartial, reasons
	case failed:
		return CoverageStateFailed, reasons
	case skipped:
		return CoverageStateUnsupported, reasons
	default:
		return CoverageStateMissing, reasons
	}
}

func aggregateCoverage(execs []Execution) Coverage {
	var enabled []Execution
	for _, exec := range execs {
		if !exec.Disabled {
			enabled = append(enabled, exec)
		}
	}
	if len(enabled) == 0 {
		return CoverageUnknown
	}
	complete := 0
	partial := 0
	unknown := 0
	for _, exec := range enabled {
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
	case complete == len(enabled):
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
