package egmmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// SchemaVersion is the only manifest schema this package accepts.
const SchemaVersion = 1

// CorpusName is the corpus identifier recorded in a manifest.
const CorpusName = "egm-independent-design-review"

// Case strata.
const (
	StratumSeeded        = "seeded"
	StratumNoKnownDefect = "no_known_defect"
)

// Allowed severity values for an expected finding.
var allowedSeverities = map[string]bool{
	"INFO": true, "LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true,
}

var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var hex64Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Ref is a permitted repository reference pinned by blob hash.
type Ref struct {
	Path       string `json:"path"`
	BlobSHA256 string `json:"blob_sha256"`
}

// Alternative is an acceptable alternative statement for an expected finding.
type Alternative struct {
	Location string `json:"location"`
	Severity string `json:"severity"`
}

// ExpectedFinding is the sealed answer for a seeded case. It is private: it must never
// appear in a reviewer-visible export.
type ExpectedFinding struct {
	Location               string        `json:"location"`
	Severity               string        `json:"severity"`
	AcceptableAlternatives []Alternative `json:"acceptable_alternatives,omitempty"`
}

// LeakChecks records the case's own leakage checks. Every field must be false for a valid
// case; a true value means the case is known to contain the answer.
type LeakChecks struct {
	ArtifactContainsAnswer           bool `json:"artifact_contains_answer"`
	PermittedReferenceContainsAnswer bool `json:"permitted_reference_contains_answer"`
	LaterEvidenceInTree              bool `json:"later_evidence_in_tree"`
}

// Case is one sealed case record, mirroring the case-construction specification §6.1.
type Case struct {
	ID                       string           `json:"id"`
	Stratum                  string           `json:"stratum"`
	ArtifactPath             string           `json:"artifact_path"`
	ArtifactSHA256           string           `json:"artifact_sha256"`
	ExportedTreeSHA256       string           `json:"exported_tree_sha256"`
	RevisionPin              string           `json:"revision_pin"`
	PermittedReferences      []Ref            `json:"permitted_references,omitempty"`
	InstructionBlockSHA256   string           `json:"instruction_block_sha256"`
	SeededDefect             string           `json:"seeded_defect"`
	DeterminabilityRationale string           `json:"determinability_rationale,omitempty"`
	ExpectedFinding          *ExpectedFinding `json:"expected_finding,omitempty"`
	Distractors              []string         `json:"distractors,omitempty"`
	ControlCheck             string           `json:"control_check,omitempty"`
	LeakChecks               LeakChecks       `json:"leak_checks"`
	AuthoredBy               string           `json:"authored_by,omitempty"`
	SealedSHA256             string           `json:"sealed_sha256"`
}

// Manifest is the sealed case manifest.
type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Corpus        string `json:"corpus"`
	Cases         []Case `json:"cases"`
}

// Validate checks the manifest and every case. It fails closed: any missing, malformed,
// duplicated, or contradictory field is an error.
func (m *Manifest) Validate() error {
	if m == nil {
		return errors.New("egmmanifest: nil manifest")
	}
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("egmmanifest: unsupported schema_version %d (want %d)", m.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(m.Corpus) == "" {
		return errors.New("egmmanifest: corpus is required")
	}
	if len(m.Cases) == 0 {
		return errors.New("egmmanifest: at least one case is required")
	}
	seen := make(map[string]bool, len(m.Cases))
	for i := range m.Cases {
		c := &m.Cases[i]
		if err := c.validate(); err != nil {
			return fmt.Errorf("egmmanifest: case %d (%q): %w", i, c.ID, err)
		}
		if seen[c.ID] {
			return fmt.Errorf("egmmanifest: duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

func (c *Case) validate() error {
	if !caseIDPattern.MatchString(c.ID) {
		return errors.New("id must match [A-Za-z0-9][A-Za-z0-9._-]*")
	}
	switch c.Stratum {
	case StratumSeeded:
		if strings.TrimSpace(c.SeededDefect) == "" || c.SeededDefect == "none" {
			return errors.New("a seeded case requires a seeded_defect class other than \"none\"")
		}
		if c.ExpectedFinding == nil {
			return errors.New("a seeded case requires expected_finding")
		}
	case StratumNoKnownDefect:
		if c.SeededDefect != "none" {
			return errors.New("a no_known_defect case requires seeded_defect \"none\"")
		}
		if strings.TrimSpace(c.ControlCheck) == "" {
			return errors.New("a no_known_defect case requires control_check")
		}
	default:
		return fmt.Errorf("unsupported stratum %q", c.Stratum)
	}
	if !safeRelativePath(c.ArtifactPath) {
		return fmt.Errorf("artifact_path %q must be a clean relative path", c.ArtifactPath)
	}
	if !hex64Pattern.MatchString(c.ArtifactSHA256) {
		return errors.New("artifact_sha256 must be 64 lowercase hex characters")
	}
	if !hex64Pattern.MatchString(c.ExportedTreeSHA256) {
		return errors.New("exported_tree_sha256 must be 64 lowercase hex characters")
	}
	if strings.TrimSpace(c.RevisionPin) == "" {
		return errors.New("revision_pin is required")
	}
	if !hex64Pattern.MatchString(c.InstructionBlockSHA256) {
		return errors.New("instruction_block_sha256 must be 64 lowercase hex characters")
	}
	if !hex64Pattern.MatchString(c.SealedSHA256) {
		return errors.New("sealed_sha256 must be 64 lowercase hex characters")
	}
	for j, ref := range c.PermittedReferences {
		if !safeRelativePath(ref.Path) {
			return fmt.Errorf("permitted_references[%d].path %q must be a clean relative path", j, ref.Path)
		}
		if !hex64Pattern.MatchString(ref.BlobSHA256) {
			return fmt.Errorf("permitted_references[%d].blob_sha256 must be 64 lowercase hex characters", j)
		}
	}
	if c.ExpectedFinding != nil {
		if strings.TrimSpace(c.ExpectedFinding.Location) == "" {
			return errors.New("expected_finding.location is required")
		}
		if !allowedSeverities[c.ExpectedFinding.Severity] {
			return fmt.Errorf("expected_finding.severity %q is not allowed", c.ExpectedFinding.Severity)
		}
		for j, alt := range c.ExpectedFinding.AcceptableAlternatives {
			if strings.TrimSpace(alt.Location) == "" {
				return fmt.Errorf("expected_finding.acceptable_alternatives[%d].location is required", j)
			}
			if !allowedSeverities[alt.Severity] {
				return fmt.Errorf("expected_finding.acceptable_alternatives[%d].severity %q is not allowed", j, alt.Severity)
			}
		}
	}
	if c.LeakChecks.ArtifactContainsAnswer || c.LeakChecks.PermittedReferenceContainsAnswer || c.LeakChecks.LaterEvidenceInTree {
		return errors.New("leak_checks must all be false for an eligible case")
	}
	return nil
}

func safeRelativePath(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) {
		return false
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return false
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	return true
}

// Load reads and validates a manifest from path. Unknown fields, trailing data, malformed
// JSON, an unsupported schema, and duplicate ids are errors.
func Load(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("egmmanifest: open manifest: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("egmmanifest: decode manifest: %w", err)
	}
	if dec.More() {
		return nil, errors.New("egmmanifest: trailing data after the manifest")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
