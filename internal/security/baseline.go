package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Baseline is a reviewed set of suppressions. An entry requires a stable
// fingerprint, a reason, an owner, and an expiry. Suppression never removes a
// finding from the report; it only marks it and excludes it from blocking.
type Baseline struct {
	Version int             `json:"version"`
	Entries []BaselineEntry `json:"entries"`
}

// BaselineEntry is one reviewed suppression. Path and RuleID are optional
// additional constraints; the fingerprint is the primary identity.
type BaselineEntry struct {
	Fingerprint string    `json:"fingerprint"`
	RuleID      string    `json:"rule_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Reason      string    `json:"reason"`
	Owner       string    `json:"owner"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// LoadBaseline reads a reviewed baseline. An absent file is an empty baseline,
// not an error. A malformed or oversized file is an error: a baseline that
// cannot be understood must not silently suppress anything.
func LoadBaseline(root, relativePath string) (Baseline, []string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return Baseline{}, nil, nil
	}
	path := relativePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(relativePath))
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Baseline{}, nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Baseline{}, nil, err
	}
	if rel, err := filepath.Rel(absRoot, absPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Baseline{}, nil, fmt.Errorf("security: baseline %q escapes the workspace", relativePath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Baseline{}, []string{"baseline file not found; no suppressions applied"}, nil
		}
		return Baseline{}, nil, fmt.Errorf("security: read baseline: %w", err)
	}
	if len(data) > 1<<20 {
		return Baseline{}, nil, fmt.Errorf("security: baseline exceeds the 1 MiB limit")
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, nil, fmt.Errorf("security: parse baseline: %w", err)
	}
	if baseline.Version != 1 {
		return Baseline{}, nil, fmt.Errorf("security: unsupported baseline version %d", baseline.Version)
	}
	return baseline, nil, nil
}

// Apply marks findings matched by a reviewed and unexpired suppression. Invalid
// entries are ignored and reported. An expired entry is visible and does not
// suppress. New findings never match an unrelated entry because the fingerprint
// is exact.
func (b Baseline) Apply(findings []Finding, now time.Time) ([]Finding, []string) {
	out := append([]Finding(nil), findings...)
	var diagnostics []string
	active := make(map[string]BaselineEntry, len(b.Entries))
	expired := make(map[string]BaselineEntry, len(b.Entries))
	for i, entry := range b.Entries {
		entry.Fingerprint = strings.TrimSpace(entry.Fingerprint)
		entry.Reason = strings.TrimSpace(entry.Reason)
		entry.Owner = strings.TrimSpace(entry.Owner)
		if entry.Fingerprint == "" || entry.Reason == "" || entry.Owner == "" || entry.ExpiresAt.IsZero() {
			diagnostics = append(diagnostics, fmt.Sprintf("baseline entry %d ignored: fingerprint, reason, owner, and expires_at are required", i))
			continue
		}
		if !entry.ExpiresAt.After(now) {
			expired[entry.Fingerprint] = entry
			continue
		}
		active[entry.Fingerprint] = entry
	}
	for i := range out {
		f := &out[i]
		if entry, ok := active[f.Fingerprint]; ok {
			if !baselineEntryMatches(entry, *f) {
				continue
			}
			f.Suppressed = true
			f.Suppression = &SuppressionMatch{
				Reason:    SanitizeText(entry.Reason, MaxMessageBytes),
				Owner:     SanitizeText(entry.Owner, 256),
				ExpiresAt: entry.ExpiresAt.UTC(),
				Expired:   false,
			}
			continue
		}
		if entry, ok := expired[f.Fingerprint]; ok {
			if !baselineEntryMatches(entry, *f) {
				continue
			}
			f.Suppressed = false
			f.Suppression = &SuppressionMatch{
				Reason:    SanitizeText(entry.Reason, MaxMessageBytes),
				Owner:     SanitizeText(entry.Owner, 256),
				ExpiresAt: entry.ExpiresAt.UTC(),
				Expired:   true,
			}
			diagnostics = append(diagnostics, fmt.Sprintf("expired suppression for %s (%s) is visible and no longer effective", f.RuleID, f.Path))
		}
	}
	return out, diagnostics
}

func baselineEntryMatches(entry BaselineEntry, finding Finding) bool {
	if entry.RuleID != "" && entry.RuleID != finding.RuleID {
		return false
	}
	if entry.Path != "" {
		path, _ := NormalizePath(entry.Path)
		if path != "" && path != finding.Path {
			return false
		}
	}
	return true
}

// maxUnmatchedListed bounds the entries one drift diagnostic names, so a large
// stale baseline stays readable.
const maxUnmatchedListed = 5

// UnmatchedDiagnostics reports reviewed, unexpired entries that matched no
// finding in the scan. Suppression identity is the exact fingerprint, which
// includes the line and column, so ordinary edits move a fingerprint: the entry
// then suppresses nothing while still looking like a live review. A corrected
// finding produces the same state. Either way the entry should be re-reviewed
// and removed.
//
// The result is advisory. It never suppresses, blocks, or changes a verdict, so
// it cannot make a scan pass or fail. It reports only entries whose fingerprint
// matched nothing: an entry that matched a finding is silent, and invalid or
// expired entries keep the diagnostics Apply already produces. Call it once with
// the complete finding set, not per adapter, so an entry is not reported as
// unmatched because another adapter's findings were absent from the call.
func (b Baseline) UnmatchedDiagnostics(findings []Finding, now time.Time) []string {
	present := make(map[string]bool, len(findings))
	for _, f := range findings {
		present[f.Fingerprint] = true
	}
	var labels []string
	seen := make(map[string]bool, len(b.Entries))
	for _, entry := range b.Entries {
		fingerprint := strings.TrimSpace(entry.Fingerprint)
		if fingerprint == "" || strings.TrimSpace(entry.Reason) == "" ||
			strings.TrimSpace(entry.Owner) == "" || entry.ExpiresAt.IsZero() {
			continue // Apply reports invalid entries
		}
		if !entry.ExpiresAt.After(now) {
			continue // expired entries never suppress
		}
		if present[fingerprint] || seen[fingerprint] {
			continue
		}
		seen[fingerprint] = true
		label := strings.TrimSpace(entry.Path)
		if label == "" {
			label = fingerprint
		}
		if rule := strings.TrimSpace(entry.RuleID); rule != "" {
			label += " (" + rule + ")"
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return nil
	}
	sort.Strings(labels)
	listed := labels
	suffix := ""
	if len(listed) > maxUnmatchedListed {
		suffix = fmt.Sprintf("; and %d more", len(listed)-maxUnmatchedListed)
		listed = listed[:maxUnmatchedListed]
	}
	noun := "entries"
	if len(labels) == 1 {
		noun = "entry"
	}
	return []string{fmt.Sprintf(
		"baseline: %d reviewed %s matched no finding at the recorded fingerprint (drifted line/column or a corrected finding); review and remove: %s%s",
		len(labels), noun, strings.Join(listed, ", "), suffix)}
}

// workspaceScopeMode is the only scope in which a scan sees the whole
// repository, so it is the only scope in which "no finding matched this entry"
// means the entry is really inert.
const workspaceScopeMode = "workspace"

// BaselineDriftDiagnostics returns the unmatched-entry diagnostic for a scan that
// observed the whole baseline. It is the entry point callers should use: the
// suppression method alone cannot tell a drifted entry from an entry the scan
// never looked for.
//
// The observation is complete only when the scope is the full workspace and every
// adapter the baseline holds entries for completed with complete coverage. A
// changed-files scan omits unchanged files by design, and a partial, failed,
// disabled, or no-op adapter leaves its entries unobserved, so a valid entry
// would then look unmatched. The function returns nothing in those cases instead
// of warning about entries it did not actually test.
//
// The result is advisory: it cannot suppress, block, or change a verdict.
func BaselineDriftDiagnostics(b Baseline, scan Scan, now time.Time) []string {
	if !baselineFullyObserved(b, scan) {
		return nil
	}
	return b.UnmatchedDiagnostics(scan.Findings, now)
}

// baselineFullyObserved reports whether this scan covered every adapter that owns
// a reviewed entry across the whole workspace.
func baselineFullyObserved(b Baseline, scan Scan) bool {
	if strings.TrimSpace(scan.Scope.Mode) != workspaceScopeMode {
		return false
	}
	required := baselineAdapters(b)
	if len(required) == 0 {
		return true // no reviewed entry can drift
	}
	observed := make(map[string]bool, len(required))
	for _, exec := range scan.Adapters {
		if observed[exec.AdapterID] {
			continue
		}
		if !exec.Disabled && exec.Status == StatusCompleted &&
			exec.Coverage == CoverageComplete && !exec.NoApplicableContent {
			observed[exec.AdapterID] = true
		}
	}
	for adapter := range required {
		if !observed[adapter] {
			return false
		}
	}
	return true
}

// baselineAdapters is the set of adapter IDs the baseline holds entries for. A
// finding fingerprint is namespaced with its adapter ID by StableFingerprint, so
// the prefix is the same identifier the scan reports in Execution.AdapterID.
func baselineAdapters(b Baseline) map[string]bool {
	out := make(map[string]bool, len(b.Entries))
	for _, entry := range b.Entries {
		adapter, _, ok := strings.Cut(strings.TrimSpace(entry.Fingerprint), ":")
		if !ok || adapter == "" {
			continue
		}
		out[adapter] = true
	}
	return out
}

// Identity returns a deterministic policy identity that includes the baseline
// content, so changing a suppression invalidates resumed evidence.
func (b Baseline) Identity() string {
	data, err := json.Marshal(b)
	if err != nil {
		return ""
	}
	return DigestBytes(data)
}
