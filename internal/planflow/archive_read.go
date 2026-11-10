package planflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// ArchiveRecord is a read-only view of a historicalized plan archive. It is used
// by historical-plan acceptance; it never mutates the archive or lifecycle.
type ArchiveRecord struct {
	// ArchiveDir is the directory holding plan.json, tasks.json and lifecycle.json.
	ArchiveDir string
	// RunsDir is the directory holding runs/<task>/ evidence. For a normal archive
	// it is <project>/.agent-sdlc/runs; for preserved worktree evidence it is the
	// evidence root's runs directory.
	RunsDir string
	PlanID  string
	Source  string
	// Disposition is the recorded terminal lifecycle disposition (COMPLETE or
	// SUPERSEDED). It is read-only evidence, not an acceptance decision.
	Disposition string
	RecordedAt  string
	Plan        planner.Plan
	Tasks       []domain.Task
}

type archivedLifecycle struct {
	PlanID      string `json:"plan_id"`
	Source      string `json:"source"`
	Disposition string `json:"disposition"`
	RecordedAt  string `json:"recorded_at"`
}

// LoadArchivedPlan reads one archive directory. It does not require or consult
// active SOP state; the archive is immutable evidence.
func LoadArchivedPlan(archiveDir string) (ArchiveRecord, error) {
	archiveDir = filepath.Clean(archiveDir)
	rec := ArchiveRecord{ArchiveDir: archiveDir}

	life, err := readArchivedLifecycle(filepath.Join(archiveDir, lifecycleFile))
	if err != nil {
		return ArchiveRecord{}, err
	}
	rec.PlanID = strings.TrimSpace(life.PlanID)
	rec.Source = strings.TrimSpace(life.Source)
	rec.Disposition = strings.TrimSpace(life.Disposition)
	rec.RecordedAt = strings.TrimSpace(life.RecordedAt)
	if rec.PlanID == "" || rec.Source == "" {
		return ArchiveRecord{}, fmt.Errorf("archive %s: lifecycle is missing plan_id or source", archiveDir)
	}

	planData, err := os.ReadFile(filepath.Join(archiveDir, planFileName))
	if err != nil {
		return ArchiveRecord{}, fmt.Errorf("archive %s: read plan: %w", archiveDir, err)
	}
	if err := json.Unmarshal(planData, &rec.Plan); err != nil {
		return ArchiveRecord{}, fmt.Errorf("archive %s: decode plan: %w", archiveDir, err)
	}

	tasksData, err := os.ReadFile(filepath.Join(archiveDir, archivedTasksFile))
	if err != nil {
		return ArchiveRecord{}, fmt.Errorf("archive %s: read tasks: %w", archiveDir, err)
	}
	if err := json.Unmarshal(tasksData, &rec.Tasks); err != nil {
		return ArchiveRecord{}, fmt.Errorf("archive %s: decode tasks: %w", archiveDir, err)
	}

	return rec, nil
}

func readArchivedLifecycle(path string) (archivedLifecycle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return archivedLifecycle{}, fmt.Errorf("read archive lifecycle: %w", err)
	}
	var life archivedLifecycle
	if err := json.Unmarshal(data, &life); err != nil {
		return archivedLifecycle{}, fmt.Errorf("decode archive lifecycle: %w", err)
	}
	return life, nil
}

// FindArchivedPlan locates the historicalized archive for source without
// consulting active state. It searches the normal archive slots first, then the
// preserved worktree-evidence copies. It returns an error listing the candidates
// when no archive matches.
func FindArchivedPlan(projectDir, source string) (ArchiveRecord, error) {
	source = filepath.ToSlash(filepath.Clean(strings.TrimSpace(source)))
	if source == "" {
		return ArchiveRecord{}, errors.New("find archived plan: plan source is required")
	}
	candidates, err := archivedPlanCandidates(projectDir)
	if err != nil {
		return ArchiveRecord{}, err
	}
	var matches []ArchiveRecord
	for _, c := range candidates {
		rec, err := LoadArchivedPlan(c.archiveDir)
		if err != nil {
			continue
		}
		rec.RunsDir = c.runsDir
		if filepath.ToSlash(filepath.Clean(rec.Source)) == source {
			matches = append(matches, rec)
		}
	}
	if len(matches) == 0 {
		return ArchiveRecord{}, fmt.Errorf("find archived plan %s: no archived lifecycle matches (looked in %s)", source, describeCandidates(candidates))
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].RecordedAt > matches[j].RecordedAt
	})
	return matches[0], nil
}

type archiveCandidate struct {
	archiveDir string
	runsDir    string
}

// archivedPlanCandidates lists the read-only archive locations this repository
// owns: the normal historicalized slots and the preserved worktree-evidence
// copies.
func archivedPlanCandidates(projectDir string) ([]archiveCandidate, error) {
	stateDir := filepath.Join(projectDir, config.DirName)
	var out []archiveCandidate

	archiveRoot := filepath.Join(stateDir, archiveDirName)
	entries, err := os.ReadDir(archiveRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read archive directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(archiveRoot, entry.Name())
		if fileExists(filepath.Join(dir, lifecycleFile)) {
			out = append(out, archiveCandidate{archiveDir: dir, runsDir: filepath.Join(stateDir, runsDirName)})
		}
	}

	evidenceRoot := filepath.Join(archiveRoot, "worktree-evidence")
	entries, err = os.ReadDir(evidenceRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read worktree-evidence directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(evidenceRoot, entry.Name())
		dir := filepath.Join(root, archiveDirName)
		if fileExists(filepath.Join(dir, lifecycleFile)) {
			out = append(out, archiveCandidate{archiveDir: dir, runsDir: filepath.Join(root, runsDirName)})
		}
	}
	return out, nil
}

func describeCandidates(candidates []archiveCandidate) string {
	if len(candidates) == 0 {
		return "no archive directories"
	}
	var dirs []string
	for _, c := range candidates {
		dirs = append(dirs, c.archiveDir)
	}
	return strings.Join(dirs, ", ")
}
