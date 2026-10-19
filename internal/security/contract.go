// Package security implements SOP's optional security scanner add-on. It
// orchestrates replaceable external scanner adapters, normalizes their evidence,
// and produces a policy verdict. It never implements a scanner engine or a
// second workflow: execution, artifacts, retries, approvals, and the final
// quality verdict remain SOP-owned.
package security

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProtocolVersion is the only adapter contract version this package speaks.
const ProtocolVersion = "security-scan/v1"

// Category is a scanner capability category. Categories are interchangeable
// within a category; a secret scanner never satisfies a SAST requirement.
type Category string

const (
	CategorySAST         Category = "sast"
	CategorySecrets      Category = "secrets"
	CategoryDependencies Category = "dependencies"
)

// isKnownCategory reports whether c is a category this contract understands. A
// category outside this set is rejected rather than silently accepted.
func isKnownCategory(c Category) bool {
	switch c {
	case CategorySAST, CategorySecrets, CategoryDependencies:
		return true
	}
	return false
}

// isKnownLanguage reports whether a language is part of the contract. Unknown
// language metadata is rejected so it cannot be mistaken for coverage.
func isKnownLanguage(language Language) bool {
	switch language {
	case LanguageGo, LanguageRust:
		return true
	}
	return false
}

// ExecutionStatus is an adapter's execution outcome. It is independent of the
// policy verdict: a COMPLETED execution can still fail policy, and a FAILED
// execution can never produce PASS.
type ExecutionStatus string

const (
	StatusCompleted   ExecutionStatus = "COMPLETED"
	StatusFailed      ExecutionStatus = "FAILED"
	StatusTimedOut    ExecutionStatus = "TIMED_OUT"
	StatusUnavailable ExecutionStatus = "UNAVAILABLE"
	StatusSkipped     ExecutionStatus = "SKIPPED"
	StatusCancelled   ExecutionStatus = "CANCELLED"
)

// Coverage reports how much of the requested scope the adapter actually
// inspected. It is reported independently of the execution status.
type Coverage string

const (
	CoverageComplete Coverage = "COMPLETE"
	CoveragePartial  Coverage = "PARTIAL"
	CoverageUnknown  Coverage = "UNKNOWN"
)

// Verdict is the policy verdict. NOT_APPLICABLE means the add-on is disabled.
type Verdict string

const (
	VerdictPass          Verdict = "PASS"
	VerdictFail          Verdict = "FAIL"
	VerdictIndeterminate Verdict = "INDETERMINATE"
	VerdictNotApplicable Verdict = "NOT_APPLICABLE"
)

// Severity is the normalized severity. Unknown is explicit and never silently
// becomes low severity.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
	SeverityUnknown  Severity = "UNKNOWN"
)

// Descriptor declares what one adapter is and what it can do. It is the
// configuration-facing half of the adapter contract.
type Descriptor struct {
	ProtocolVersion string     `json:"protocol_version"`
	AdapterID       string     `json:"adapter_id"`
	AdapterVersion  string     `json:"adapter_version"`
	Categories      []Category `json:"categories"`
	ScopeModes      []string   `json:"scope_modes"`
	Locality        string     `json:"locality"`
	Network         string     `json:"network"`
	// Languages declares the Go/Rust coverage this adapter provides. An empty
	// list means language coverage is undeclared, unless RepositoryWide is set.
	Languages []Language `json:"languages,omitempty"`
	// RepositoryWide declares that the adapter inspects the repository as a whole
	// rather than a language set (for example a secrets scanner).
	RepositoryWide bool `json:"repository_wide,omitempty"`
}

// Validate rejects a descriptor that cannot satisfy the contract.
func (d Descriptor) Validate() error {
	if d.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("descriptor: unsupported protocol version %q (want %s)", d.ProtocolVersion, ProtocolVersion)
	}
	if strings.TrimSpace(d.AdapterID) == "" {
		return fmt.Errorf("descriptor: adapter id is required")
	}
	if len(d.Categories) == 0 {
		return fmt.Errorf("descriptor: adapter %s declares no categories", d.AdapterID)
	}
	for _, c := range d.Categories {
		if !isKnownCategory(c) {
			return fmt.Errorf("descriptor: adapter %s has unknown category %q", d.AdapterID, c)
		}
	}
	if len(d.ScopeModes) == 0 {
		return fmt.Errorf("descriptor: adapter %s declares no scope modes", d.AdapterID)
	}
	for _, language := range d.Languages {
		if !isKnownLanguage(language) {
			return fmt.Errorf("descriptor: adapter %s has unknown language %q", d.AdapterID, language)
		}
	}
	if d.RepositoryWide && len(d.Languages) > 0 {
		return fmt.Errorf("descriptor: adapter %s cannot be both repository-wide and language-scoped", d.AdapterID)
	}
	return nil
}

// ContentIdentity records the exact content the scan observed. A clean tree is
// identified by commit and tree; a dirty tree additionally carries a content
// digest and a dirty marker.
type ContentIdentity struct {
	Commit string `json:"commit,omitempty"`
	Tree   string `json:"tree,omitempty"`
	Digest string `json:"digest,omitempty"`
	Dirty  bool   `json:"dirty"`
}

// Scope describes the requested scan scope. Workspace is the MVP default. A
// changed-files scope is recorded with its file list so exclusions and
// limitations are explicit. Excluded lists repository-relative paths or glob
// patterns that are deliberately outside the requested scope; the list is
// evidence of the declared boundary, not an account of files the scanner failed
// to read.
type Scope struct {
	Mode     string   `json:"mode"`
	Files    []string `json:"files,omitempty"`
	Excluded []string `json:"excluded,omitempty"`
}

// Request is the versioned request sent to an adapter.
type Request struct {
	ProtocolVersion string            `json:"protocol_version"`
	RunID           string            `json:"run_id"`
	TaskID          string            `json:"task_id,omitempty"`
	AttemptID       string            `json:"attempt_id,omitempty"`
	WorkspaceRoot   string            `json:"workspace_root"`
	Content         ContentIdentity   `json:"content"`
	Scope           Scope             `json:"scope"`
	Configuration   map[string]string `json:"configuration,omitempty"`
	DeadlineMS      int64             `json:"deadline_ms,omitempty"`
	OutputPath      string            `json:"output_path,omitempty"`
}

// SkippedFile records a file omitted from the scan and why.
type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Finding is one normalized finding. Provider meaning is retained alongside the
// normalized severity. Raw secret values are never represented here.
type Finding struct {
	AdapterID        string            `json:"adapter_id"`
	Provider         string            `json:"provider"`
	Category         Category          `json:"category"`
	RuleID           string            `json:"rule_id"`
	OriginalSeverity string            `json:"original_severity,omitempty"`
	Severity         Severity          `json:"severity"`
	Message          string            `json:"message"`
	Path             string            `json:"path,omitempty"`
	Line             int               `json:"line,omitempty"`
	EndLine          int               `json:"end_line,omitempty"`
	Column           int               `json:"column,omitempty"`
	Fingerprint      string            `json:"fingerprint"`
	CWE              []string          `json:"cwe,omitempty"`
	CVE              []string          `json:"cve,omitempty"`
	Confidence       string            `json:"confidence,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	// Suppressed is true only when a reviewed, unexpired suppression matched
	// this fingerprint. The finding remains in the report either way.
	Suppressed  bool              `json:"suppressed"`
	Suppression *SuppressionMatch `json:"suppression,omitempty"`
}

// SuppressionMatch records the reviewed entry that matched a finding. An expired
// entry is reported as expired and does not suppress.
type SuppressionMatch struct {
	Reason    string    `json:"reason"`
	Owner     string    `json:"owner"`
	ExpiresAt time.Time `json:"expires_at"`
	Expired   bool      `json:"expired"`
}

// Provenance records the identities needed to reason about reproducibility.
type Provenance struct {
	ScannerVersion string `json:"scanner_version"`
	RulesIdentity  string `json:"rules_identity,omitempty"`
	// AdvisoryDatabaseIdentity records the resolved advisory database identity for
	// a dependency scan. It is separate from the rule identity so resume validation
	// covers a changed database. It is a deterministic local fingerprint, not a
	// timestamp, so an unchanged database reuses evidence.
	AdvisoryDatabaseIdentity string `json:"advisory_database_identity,omitempty"`
	// AdvisoryDatabaseMetadata records the scanner-observed database metadata
	// (last commit, last update, advisory count). It is evidence for freshness and
	// provenance; it is not used as the resume key.
	AdvisoryDatabaseMetadata string `json:"advisory_database_metadata,omitempty"`
	// AdvisoryDatabaseStale reports that the adapter observed a stale advisory
	// database. Policy decides what a stale database means.
	AdvisoryDatabaseStale bool `json:"advisory_database_stale,omitempty"`
	// LockfileIdentity records the resolved dependency input files and their
	// content digests for a dependency scan. It is a resume key: a changed
	// lockfile or go.mod must invalidate reused evidence even when the database is
	// unchanged.
	LockfileIdentity string `json:"lockfile_identity,omitempty"`
	// ObligationIdentity records the complete resolved obligation inventory,
	// including missing or unsupported inputs. It invalidates reused evidence when
	// an obligation is added, removed, or changes state.
	ObligationIdentity string `json:"obligation_identity,omitempty"`
	// Exclusions records the configured out-of-scope patterns the scan was given.
	// They are declared scope, not coverage gaps, and are visible as evidence so a
	// reader can tell a declared exclusion from an unexpected in-scope skip.
	Exclusions []string `json:"exclusions,omitempty"`
	// Selection and Languages record the runtime selection mode and declared
	// language scope. They are part of resume identity.
	Selection             string          `json:"selection,omitempty"`
	Languages             []Language      `json:"languages,omitempty"`
	ConfigurationDigest   string          `json:"configuration_digest"`
	ContentIdentity       ContentIdentity `json:"content_identity"`
	PolicyIdentity        string          `json:"policy_identity"`
	SanitizedReportDigest string          `json:"sanitized_report_digest,omitempty"`
}

// SelectionMode records how an adapter entered the run. Explicit selection is
// configuration; automatic selection fills only unconfigured, enabled coverage.
type SelectionMode string

const (
	SelectionExplicit  SelectionMode = "explicit"
	SelectionAutomatic SelectionMode = "automatic"
)

// Execution is one adapter's complete result. It satisfies the same contract for
// built-in and external adapters.
type Execution struct {
	ProtocolVersion string          `json:"protocol_version"`
	AdapterID       string          `json:"adapter_id"`
	AdapterVersion  string          `json:"adapter_version,omitempty"`
	Category        Category        `json:"category"`
	Status          ExecutionStatus `json:"status"`
	ExitCode        int             `json:"exit_code"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         time.Time       `json:"ended_at"`
	Coverage        Coverage        `json:"coverage"`
	Selection       string          `json:"selection,omitempty"`
	Languages       []Language      `json:"languages,omitempty"`
	RepositoryWide  bool            `json:"repository_wide,omitempty"`
	Disabled        bool            `json:"disabled,omitempty"`
	// NoApplicableContent marks a completed no-op: the repository has no content
	// for this adapter's capability, so no scanner ran and no language coverage is
	// claimed from selection metadata.
	NoApplicableContent bool `json:"no_applicable_content,omitempty"`
	// Obligations preserves the resolved dependency input inventory, including
	// missing/unsupported inputs.
	Obligations        []DependencyObligation `json:"obligations,omitempty"`
	Skipped            []SkippedFile          `json:"skipped,omitempty"`
	Diagnostics        []string               `json:"diagnostics,omitempty"`
	Findings           []Finding              `json:"findings,omitempty"`
	Provenance         Provenance             `json:"provenance"`
	NativeReportDigest string                 `json:"native_report_digest,omitempty"`
	// NativeReport is the sanitized native report retained for debugging. It is
	// never written into the normalized execution JSON itself and never carries a
	// detected secret value.
	NativeReport []byte `json:"-"`
	// Reused reports that resume reused a completed result whose identities still
	// matched instead of re-executing the adapter.
	Reused bool `json:"reused,omitempty"`
}

// Validate checks the execution contract. It is deterministic and never treats
// untrusted adapter output as valid by default.
func (e Execution) Validate() error {
	if e.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("execution: unsupported protocol version %q (want %s)", e.ProtocolVersion, ProtocolVersion)
	}
	if strings.TrimSpace(e.AdapterID) == "" {
		return fmt.Errorf("execution: adapter id is required")
	}
	if !isKnownCategory(e.Category) {
		return fmt.Errorf("execution: adapter %s has unknown category %q", e.AdapterID, e.Category)
	}
	switch e.Status {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusUnavailable, StatusSkipped, StatusCancelled:
	default:
		return fmt.Errorf("execution: adapter %s has unknown status %q", e.AdapterID, e.Status)
	}
	switch e.Coverage {
	case CoverageComplete, CoveragePartial, CoverageUnknown:
	default:
		return fmt.Errorf("execution: adapter %s has unknown coverage %q", e.AdapterID, e.Coverage)
	}
	for _, language := range e.Languages {
		if !isKnownLanguage(language) {
			return fmt.Errorf("execution: adapter %s has unknown language %q", e.AdapterID, language)
		}
	}
	for i, obligation := range e.Obligations {
		if !isKnownLanguage(obligation.Language) {
			return fmt.Errorf("execution: adapter %s obligation %d has unknown language %q", e.AdapterID, i, obligation.Language)
		}
		if strings.TrimSpace(obligation.Input) == "" {
			return fmt.Errorf("execution: adapter %s obligation %d has no input", e.AdapterID, i)
		}
	}
	if e.Disabled && e.Status != StatusSkipped {
		return fmt.Errorf("execution: adapter %s is disabled but status is %s (want %s)", e.AdapterID, e.Status, StatusSkipped)
	}
	if !e.StartedAt.IsZero() && !e.EndedAt.IsZero() && e.EndedAt.Before(e.StartedAt) {
		return fmt.Errorf("execution: adapter %s ended before it started", e.AdapterID)
	}
	for i, f := range e.Findings {
		if f.AdapterID == "" {
			return fmt.Errorf("execution: adapter %s finding %d has no adapter id", e.AdapterID, i)
		}
		if strings.TrimSpace(f.RuleID) == "" {
			return fmt.Errorf("execution: adapter %s finding %d has no rule id", e.AdapterID, i)
		}
		if strings.TrimSpace(f.Fingerprint) == "" {
			return fmt.Errorf("execution: adapter %s finding %d has no fingerprint", e.AdapterID, i)
		}
		switch f.Severity {
		case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo, SeverityUnknown:
		default:
			return fmt.Errorf("execution: adapter %s finding %d has unknown severity %q", e.AdapterID, i, f.Severity)
		}
		if f.Path != "" && !SafeRelativePath(f.Path) {
			return fmt.Errorf("execution: adapter %s finding %d has unsafe path %q", e.AdapterID, i, f.Path)
		}
	}
	return nil
}

// Adapter is one replaceable scanner implementation. Built-in scanner CLIs and
// external processes implement the same interface.
type Adapter interface {
	Descriptor() Descriptor
	Scan(ctx context.Context, req Request) (Execution, error)
}
