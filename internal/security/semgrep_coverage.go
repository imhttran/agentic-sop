package security

import (
	"strconv"
	"strings"
)

// semgrepSkipClass identifies one class of skipped or restricted target in
// Semgrep's human-readable summary. The class is evidence about why coverage is
// incomplete; it is never used to decide that a skip is acceptable.
type semgrepSkipClass string

const (
	// semgrepSkipDeclaredExclude is a target omitted by a configured --exclude
	// pattern. That is declared out-of-scope content, not a coverage gap.
	semgrepSkipDeclaredExclude semgrepSkipClass = "declared-exclude"
	// semgrepSkipSemgrepIgnore is a target omitted by .semgrepignore patterns
	// (Semgrep's built-in defaults and repository content). It is inside the
	// declared scope, so it is a coverage gap.
	semgrepSkipSemgrepIgnore semgrepSkipClass = "semgrepignore"
	// semgrepSkipLargeFile is a target omitted because it exceeds Semgrep's size
	// limit. It is inside the declared scope, so it is a coverage gap.
	semgrepSkipLargeFile semgrepSkipClass = "large-file"
	// semgrepSkipUnrecognized is any skip reason this build does not recognize. It
	// is still a gap: an unknown scanner limitation never yields COMPLETE.
	semgrepSkipUnrecognized semgrepSkipClass = "unrecognized"
)

// semgrepSkipCountUnknown marks a skip whose summary exposes no target count.
const semgrepSkipCountUnknown = -1

type semgrepSkip struct {
	Class  semgrepSkipClass
	Reason string
	Count  int
}

// semgrepCoverageEvidence is what the scanner reported about the targets it
// examined. SummaryKnown distinguishes "the scanner reported no skips" from "the
// scanner did not report a recognizable coverage summary at all".
type semgrepCoverageEvidence struct {
	SummaryKnown bool
	Scanned      int
	ScannedKnown bool
	Skipped      []semgrepSkip
}

// gaps returns skip classes that fall inside the declared scope.
func (e semgrepCoverageEvidence) gaps() []semgrepSkip {
	var out []semgrepSkip
	for _, skip := range e.Skipped {
		if skip.Class != semgrepSkipDeclaredExclude {
			out = append(out, skip)
		}
	}
	return out
}

// declared returns skips caused by configured exclusions.
func (e semgrepCoverageEvidence) declared() []semgrepSkip {
	var out []semgrepSkip
	for _, skip := range e.Skipped {
		if skip.Class == semgrepSkipDeclaredExclude {
			out = append(out, skip)
		}
	}
	return out
}

// parseSemgrepCoverageSummary reads the coverage limits Semgrep reports only in
// its human summary. A summary is "known" only when the scanner showed both its
// summary section and at least one of the evidence lines this parser relies on.
// Anything else is unrecognized, and unrecognized evidence must not become
// COMPLETE.
func parseSemgrepCoverageSummary(stderr string) semgrepCoverageEvidence {
	var (
		evidence   semgrepCoverageEvidence
		inSummary  bool
		ignoreInfo bool
		skipSeen   bool
		inSkip     bool
		terminated bool
	)
	for _, line := range strings.Split(stderr, "\n") {
		text := strings.TrimSpace(line)
		if !inSummary {
			// A skip block or summary section opens coverage parsing. The surrounding
			// box drawing is cosmetic and deliberately ignored.
			if strings.Contains(text, "Scan Summary") {
				inSummary = true
				continue
			}
			if strings.Contains(text, "Scan skipped:") {
				inSummary = true
				skipSeen = true
				inSkip = true
			}
			continue
		}
		reason, isBullet := semgrepSummaryBullet(text)
		if !isBullet {
			reason = text
		}
		// Summary lines are bullets in Semgrep's output; the marker is cosmetic, so
		// both bullet and non-bullet forms are parsed identically.
		if strings.HasPrefix(reason, "Ran ") {
			// The closing summary line is the terminator. Its absence means the summary
			// may have been truncated, which must not become COMPLETE.
			terminated = true
			inSkip = false
			continue
		}
		if strings.HasPrefix(reason, "Scan skipped:") {
			inSkip = true
			skipSeen = true
			continue
		}
		if strings.HasPrefix(reason, "Targets scanned:") {
			if count, ok := semgrepSummaryCount(reason); ok {
				evidence.Scanned = count
				evidence.ScannedKnown = true
			}
			inSkip = false
			continue
		}
		if strings.Contains(reason, "No ignore information available") {
			ignoreInfo = true
			inSkip = false
			continue
		}
		if !inSkip {
			continue
		}
		if isSemgrepSummaryLabel(reason) {
			inSkip = false
			continue
		}
		if reason == "" {
			continue
		}
		evidence.Skipped = append(evidence.Skipped, classifySemgrepSkip(reason))
	}
	evidence.SummaryKnown = inSummary && terminated && (evidence.ScannedKnown || ignoreInfo || skipSeen)
	return evidence
}

// semgrepSummaryBullet strips a summary bullet marker. It reports whether the
// line was a bullet at all.
func semgrepSummaryBullet(text string) (string, bool) {
	for _, marker := range []string{"•", "◦", "-"} {
		if strings.HasPrefix(text, marker) {
			return strings.TrimSpace(strings.TrimPrefix(text, marker)), true
		}
	}
	return "", false
}

// classifySemgrepSkip maps one skip reason to a class. The reason text is
// preserved verbatim so no target detail or count is invented.
func classifySemgrepSkip(reason string) semgrepSkip {
	reason = strings.TrimSpace(reason)
	skip := semgrepSkip{Class: semgrepSkipUnrecognized, Reason: SanitizeText(reason, 256), Count: semgrepSkipCountUnknown}
	if count, ok := semgrepSummaryCount(reason); ok {
		skip.Count = count
	}
	switch {
	case strings.HasPrefix(reason, "Matching --exclude patterns"):
		skip.Class = semgrepSkipDeclaredExclude
	case strings.Contains(reason, ".semgrepignore"):
		skip.Class = semgrepSkipSemgrepIgnore
	case strings.HasPrefix(reason, "Files larger than"):
		skip.Class = semgrepSkipLargeFile
	}
	return skip
}

// semgrepSummaryCount parses a trailing ": N" count. It reports false when the
// summary exposes no usable count.
func semgrepSummaryCount(text string) (int, bool) {
	index := strings.LastIndex(text, ":")
	if index < 0 {
		return 0, false
	}
	value := strings.TrimSpace(strings.ReplaceAll(text[index+1:], ",", ""))
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 {
		return 0, false
	}
	return count, true
}

// semgrepSkipDetail renders skip reasons and counts for persisted diagnostics.
func semgrepSkipDetail(skips []semgrepSkip) string {
	parts := make([]string, 0, len(skips))
	for _, skip := range skips {
		if skip.Count == semgrepSkipCountUnknown {
			parts = append(parts, skip.Reason+" (count unavailable)")
			continue
		}
		parts = append(parts, skip.Reason)
	}
	return strings.Join(parts, "; ")
}

// semgrepCoverageDecision converts coverage evidence into a coverage claim.
//
//   - A skip inside the declared scope is PARTIAL; it is never COMPLETE.
//   - Missing or unrecognized summary evidence is UNKNOWN, which makes the policy
//     verdict INDETERMINATE rather than clean.
//   - COMPLETE requires a recognized summary and no in-scope skips.
//
// Coverage is deliberately independent of the findings verdict: COMPLETE means
// the declared scope was scanned, not that it is free of vulnerabilities.
func semgrepCoverageDecision(evidence semgrepCoverageEvidence, detected int) (Coverage, []string) {
	var diagnostics []string
	if gaps := evidence.gaps(); len(gaps) > 0 {
		diagnostics = append(diagnostics, "semgrep skipped target(s) inside the declared scope; coverage is PARTIAL: "+semgrepSkipDetail(gaps))
		return CoveragePartial, diagnostics
	}
	if !evidence.SummaryKnown {
		return CoverageUnknown, []string{"semgrep reported no recognizable coverage summary; coverage evidence is missing or unrecognized (INDETERMINATE)"}
	}
	if detected > 0 && (!evidence.ScannedKnown || evidence.Scanned == 0) {
		return CoverageUnknown, []string{"semgrep reported no scanned targets despite detected source; coverage is INDETERMINATE"}
	}
	if declared := evidence.declared(); len(declared) > 0 {
		diagnostics = append(diagnostics, "semgrep omitted target(s) by configured exclusion patterns (declared out-of-scope): "+semgrepSkipDetail(declared))
	}
	return CoverageComplete, diagnostics
}
