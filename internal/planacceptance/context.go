// Package planacceptance implements read-only historical-plan acceptance on top
// of the existing SOP acceptance model. It never reopens, rewrites, or releases a
// historicalized plan; decisions and check evidence live in a plan-scoped,
// append-only audit outside the archive.
package planacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// PlanIdentityVersion versions the deterministic plan-identity serialization.
const PlanIdentityVersion = "plan-acceptance/1"

// Context is the immutable archived plan plus the plan-scoped audit location.
type Context struct {
	ProjectDir   string
	Archive      planflow.ArchiveRecord
	PlanIdentity string
	Mappings     []Mapping
	LedgerDir    string
}

// Mapping is one historical criterion bound to the archived plan identity.
type Mapping struct {
	CriterionID    string               `json:"criterion_id"`
	DisplayID      string               `json:"display_id"`
	TaskID         string               `json:"task_id"`
	Index          int                  `json:"index"`
	Description    string               `json:"description"`
	Kind           outcome.EvidenceKind `json:"kind"`
	ManualEvidence string               `json:"manual_evidence,omitempty"`
	CheckRef       string               `json:"check_ref,omitempty"`
	Command        string               `json:"command,omitempty"`
	PlanIdentity   string               `json:"plan_identity"`
}

// Counts are derived from the mapping and evaluation sets, never hard-coded.
type Counts struct {
	Mapped           int `json:"mapped"`
	Manual           int `json:"manual"`
	Check            int `json:"check"`
	DecisionEligible int `json:"decision_eligible"`
	ManualAccepted   int `json:"manual_accepted"`
	CheckPassed      int `json:"check_passed"`
	Unverified       int `json:"unverified"`
	Stale            int `json:"stale"`
	Rejected         int `json:"rejected"`
	Unmapped         int `json:"unmapped"`
}

// LoadContext resolves the historicalized archive, derives the 16 mappings, and
// computes the immutable plan identity. It is read-only. archiveOverride may be
// the archive directory (containing plan.json) or a preserved evidence root
// (containing archive/ and runs/); empty means resolve by plan source.
func LoadContext(projectDir, source, archiveOverride string) (*Context, error) {
	rec, err := resolveArchive(projectDir, source, archiveOverride)
	if err != nil {
		return nil, err
	}
	mappings, err := BuildMappings(rec.Plan)
	if err != nil {
		return nil, err
	}
	identity, err := PlanIdentity(rec)
	if err != nil {
		return nil, err
	}
	return &Context{
		ProjectDir:   projectDir,
		Archive:      rec,
		PlanIdentity: identity,
		Mappings:     mappings,
		LedgerDir:    filepath.Join(projectDir, config.DirName, "acceptance", rec.PlanID),
	}, nil
}

func resolveArchive(projectDir, source, override string) (planflow.ArchiveRecord, error) {
	if strings.TrimSpace(override) == "" {
		return planflow.FindArchivedPlan(projectDir, source)
	}
	root := filepath.Clean(override)
	if fileExists(filepath.Join(root, "plan.json")) {
		rec, err := planflow.LoadArchivedPlan(root)
		if err != nil {
			return planflow.ArchiveRecord{}, err
		}
		if err := requireMatchingSource(projectDir, rec, source); err != nil {
			return planflow.ArchiveRecord{}, err
		}
		rec.RunsDir = inferRunsDir(root, projectDir)
		return rec, nil
	}
	if fileExists(filepath.Join(root, "archive", "plan.json")) {
		rec, err := planflow.LoadArchivedPlan(filepath.Join(root, "archive"))
		if err != nil {
			return planflow.ArchiveRecord{}, err
		}
		if err := requireMatchingSource(projectDir, rec, source); err != nil {
			return planflow.ArchiveRecord{}, err
		}
		rec.RunsDir = filepath.Join(root, "runs")
		return rec, nil
	}
	return planflow.ArchiveRecord{}, fmt.Errorf("archive %s does not contain plan.json or archive/plan.json", root)
}

// requireMatchingSource rejects an explicit --archive that belongs to a
// different plan than the one requested, so a stale or mistyped archive argument
// cannot record a decision against the wrong plan.
func requireMatchingSource(projectDir string, rec planflow.ArchiveRecord, requested string) error {
	if samePlanSource(projectDir, rec.Source, requested) {
		return nil
	}
	return fmt.Errorf("archive %s belongs to plan source %s, not %s", rec.ArchiveDir, rec.Source, normalizeSource(requested))
}

// samePlanSource reports whether an archived plan source and a requested plan
// path identify the same plan. Both are compared after slash normalization; an
// absolute path is also compared relative to the project.
func samePlanSource(projectDir, archived, requested string) bool {
	a := normalizeSource(archived)
	b := normalizeSource(requested)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if filepath.IsAbs(requested) {
		if rel, err := filepath.Rel(projectDir, requested); err == nil && normalizeSource(rel) == a {
			return true
		}
	}
	if filepath.IsAbs(archived) {
		if rel, err := filepath.Rel(projectDir, archived); err == nil && normalizeSource(rel) == b {
			return true
		}
	}
	return false
}

func normalizeSource(s string) string {
	s = filepath.ToSlash(filepath.Clean(strings.TrimSpace(s)))
	if s == "." {
		return ""
	}
	return strings.TrimPrefix(s, "./")
}

func inferRunsDir(archiveDir, projectDir string) string {
	parent := filepath.Dir(archiveDir)
	if fileExists(filepath.Join(parent, "runs")) {
		return filepath.Join(parent, "runs")
	}
	return filepath.Join(projectDir, config.DirName, "runs")
}

// BuildMappings derives one mapping per archived acceptance criterion. It never
// infers a mapping from a legacy outcome or from mutable documentation.
func BuildMappings(plan planner.Plan) ([]Mapping, error) {
	var out []Mapping
	for _, stage := range plan.Stages {
		if len(stage.AcceptanceCriteria) == 0 {
			continue
		}
		byCriterion := map[int]planner.AcceptanceMapping{}
		for _, m := range stage.AcceptanceMapping {
			if m.Criterion < 1 || m.Criterion > len(stage.AcceptanceCriteria) {
				return nil, fmt.Errorf("stage %s: acceptance mapping names criterion %d outside 1..%d", stage.ID, m.Criterion, len(stage.AcceptanceCriteria))
			}
			if _, dup := byCriterion[m.Criterion]; dup {
				return nil, fmt.Errorf("stage %s: criterion %d is mapped more than once", stage.ID, m.Criterion)
			}
			if (m.Manual == "") == (m.Check == 0) {
				return nil, fmt.Errorf("stage %s: criterion %d must map to exactly one of manual evidence or a check", stage.ID, m.Criterion)
			}
			if m.Check != 0 && (m.Check < 1 || m.Check > len(stage.AcceptanceChecks)) {
				return nil, fmt.Errorf("stage %s: criterion %d names check %d outside 1..%d", stage.ID, m.Criterion, m.Check, len(stage.AcceptanceChecks))
			}
			byCriterion[m.Criterion] = m
		}
		for i, text := range stage.AcceptanceCriteria {
			n := i + 1
			m, ok := byCriterion[n]
			if !ok {
				return nil, fmt.Errorf("stage %s: criterion %d is not mapped", stage.ID, n)
			}
			mp := Mapping{
				CriterionID:    fmt.Sprintf("%s.criterion-%d", stage.ID, n),
				DisplayID:      displayID(stage.ID, n),
				TaskID:         stage.ID,
				Index:          n,
				Description:    strings.TrimSpace(text),
				ManualEvidence: strings.TrimSpace(m.Manual),
			}
			if m.Check != 0 {
				mp.Kind = outcome.EvidenceCheck
				mp.CheckRef = fmt.Sprintf("%s.check-%d", stage.ID, m.Check)
				mp.Command = strings.TrimSpace(stage.AcceptanceChecks[m.Check-1])
				if mp.Command == "" {
					return nil, fmt.Errorf("stage %s: check %d is empty", stage.ID, m.Check)
				}
			} else {
				mp.Kind = outcome.EvidenceManual
				if mp.ManualEvidence == "" {
					return nil, fmt.Errorf("stage %s: criterion %d has empty manual evidence", stage.ID, n)
				}
			}
			out = append(out, mp)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("archived plan declares no acceptance criteria")
	}
	return out, nil
}

var disclosingID = regexp.MustCompile(`^DISCLOSE-(\d+)$`)

func displayID(stageID string, n int) string {
	if m := disclosingID.FindStringSubmatch(stageID); m != nil {
		return m[1] + "." + fmt.Sprint(n)
	}
	return stageID + "." + fmt.Sprint(n)
}

// CountsFromMappings derives the mapped, manual and check totals. Acceptance
// counts are added by the evaluator.
func CountsFromMappings(mappings []Mapping) Counts {
	c := Counts{Mapped: len(mappings)}
	for _, m := range mappings {
		switch m.Kind {
		case outcome.EvidenceManual:
			c.Manual++
			if strings.TrimSpace(m.ManualEvidence) != "" {
				c.DecisionEligible++
			}
		case outcome.EvidenceCheck:
			c.Check++
		default:
			c.Unmapped++
		}
	}
	return c
}

// PlanIdentity hashes the immutable bound content: requirements, mappings,
// checks, task IDs, and the lifecycle identity. Timestamps and task status are
// excluded so an identical archive copy retains its identity.
func PlanIdentity(rec planflow.ArchiveRecord) (string, error) {
	type stageIdentity struct {
		ID                 string                      `json:"id"`
		Dependencies       []string                    `json:"dependencies"`
		AcceptanceCriteria []string                    `json:"acceptance_criteria"`
		AcceptanceChecks   []string                    `json:"acceptance_checks"`
		AcceptanceMapping  []planner.AcceptanceMapping `json:"acceptance_mapping"`
		ExecutionMode      string                      `json:"execution_mode"`
	}
	type taskIdentity struct {
		ID                 string `json:"id"`
		AcceptanceCriteria string `json:"acceptance_criteria"`
	}
	type boundContent struct {
		Version     string          `json:"version"`
		PlanID      string          `json:"plan_id"`
		Source      string          `json:"source"`
		Disposition string          `json:"disposition"`
		Stages      []stageIdentity `json:"stages"`
		Tasks       []taskIdentity  `json:"tasks"`
	}
	bound := boundContent{
		Version:     PlanIdentityVersion,
		PlanID:      rec.PlanID,
		Source:      filepath.ToSlash(filepath.Clean(rec.Source)),
		Disposition: rec.Disposition,
	}
	for _, s := range rec.Plan.Stages {
		bound.Stages = append(bound.Stages, stageIdentity{
			ID:                 s.ID,
			Dependencies:       append([]string(nil), s.Dependencies...),
			AcceptanceCriteria: append([]string(nil), s.AcceptanceCriteria...),
			AcceptanceChecks:   append([]string(nil), s.AcceptanceChecks...),
			AcceptanceMapping:  append([]planner.AcceptanceMapping(nil), s.AcceptanceMapping...),
			ExecutionMode:      string(s.ExecutionMode),
		})
	}
	for _, t := range rec.Tasks {
		bound.Tasks = append(bound.Tasks, taskIdentity{ID: t.ID, AcceptanceCriteria: t.AcceptanceCriteria})
	}
	data, err := json.Marshal(bound)
	if err != nil {
		return "", fmt.Errorf("serialize plan identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FindMapping returns the mapping for a canonical criterion ID or its display
// alias (for example DISCLOSE-001.criterion-1 or 001.1).
func FindMapping(ctx *Context, criterionID string) (Mapping, bool) {
	for _, m := range ctx.Mappings {
		if m.CriterionID == criterionID || m.DisplayID == criterionID {
			return m, true
		}
	}
	return Mapping{}, false
}
