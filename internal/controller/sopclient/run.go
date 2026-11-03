package sopclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run artifacts live under <root>/.agent-sdlc/runs/<task-id>/ and are read-only here.

// Run stages mirror agentic-sop run.Stage.
const (
	StageCreated         = "CREATED"
	StagePlanning        = "PLANNING"
	StageImplementing    = "IMPLEMENTING"
	StageValidating      = "VALIDATING"
	StageReviewing       = "REVIEWING"
	StageFixing          = "FIXING"
	StageWaitingForHuman = "WAITING_FOR_HUMAN"
	StagePassed          = "PASSED"
	StageFailed          = "FAILED"
)

// Failure dispositions mirror agentic-sop failure.Disposition.
const (
	DispositionAutoFix    = "AUTO_FIX"
	DispositionContinue   = "CONTINUE"
	DispositionRetry      = "RETRY"
	DispositionReplan     = "REPLAN"
	DispositionNeedsHuman = "NEEDS_HUMAN"
)

// Activity action kinds (SOP's vocabulary) a consumer can recognize.
const (
	// ActionInspect: inspecting files/state (read-only observation).
	ActionInspect = "inspect"
	// ActionMutate: mutating the repository (edit/write/patch).
	ActionMutate = "mutate"
	// ActionCommand: executing a command.
	ActionCommand = "command"
	// ActionValidate: running validation (build/test/lint).
	ActionValidate = "validate"
	// ActionReview: running review.
	ActionReview = "review"
	// ActionJEV: a JEV (independent verification) analysis.
	ActionJEV = "jev"
	// ActionQuality: a quality decision (accept/reject a result).
	ActionQuality = "quality"
	// ActionRecover: recovering from a failure (retry/auto-fix).
	ActionRecover = "recover"
	// ActionComplete: completion / lifecycle transition.
	ActionComplete = "complete"
)

// ActivityEvent is one SOP activity event (agentic-sop activity.Event). Detail
// is a safe summary produced only through sanitizeDetail: never a prompt,
// secret, env dump, or raw command/file content.
type ActivityEvent struct {
	TaskID    string
	Stage     string
	Action    string
	Detail    string
	Timestamp time.Time
}

// Classification mirrors agentic-sop failure.Classification: why a run stopped
// short of a pass and the disposition SOP applied.
type Classification struct {
	Kind        string
	Disposition string
	Confidence  string
	Reason      string
}

// HumanRequired reports whether SOP classified the failure as needing a human.
func (c *Classification) HumanRequired() bool {
	return c != nil && c.Disposition == DispositionNeedsHuman
}

// JEV is SOP's optional JEV analysis summary from a run report.
type JEV struct {
	Status   string
	Reason   string
	Summary  string
	Findings int
}

// ModelSelection is SOP's recorded model selection for a run (report.json
// "model_selection"), carried verbatim. The controller reports what SOP
// selected; it never selects a model or provider itself (see
// docs/specs/AGENT-PROVIDER.md). Class is SOP's vocabulary (for example SMALL /
// MEDIUM / LARGE) and is displayed as persisted, without re-deriving it.
type ModelSelection struct {
	// Present is true when SOP persisted a model-selection record.
	Present  bool
	Class    string
	Provider string
	Model    string
	Locality string
	Source   string
	Fallback bool
	Reason   string
}

// RunInfo is the latest run's record for one task; Present when any artifact exists.
type RunInfo struct {
	Present        bool
	Stage          string
	Decision       string
	Reasons        []string
	FixCycles      int
	Provider       string
	Model          string
	ModelSelection ModelSelection
	ExecutionMode  string
	VerifiedFirst  bool
	GeneratedAt    time.Time
	UpdatedAt      time.Time
	Classification *Classification
	JEV            *JEV
}

type stateDoc struct {
	ID        string    `json:"id"`
	Stage     string    `json:"stage"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type classificationDoc struct {
	Kind        string `json:"kind"`
	Disposition string `json:"disposition"`
	Confidence  string `json:"confidence"`
	Reason      string `json:"reason"`
}

type activityDoc struct {
	Stage     string    `json:"stage"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	Timestamp time.Time `json:"timestamp"`
}

// modelSelectionDoc mirrors agentic-sop's report "model_selection" object.
type modelSelectionDoc struct {
	Class    string `json:"class"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Locality string `json:"locality"`
	Source   string `json:"source"`
	Fallback bool   `json:"fallback"`
	Reason   string `json:"reason"`
}

type reportDoc struct {
	ID             string             `json:"id"`
	Stage          string             `json:"stage"`
	Provider       string             `json:"provider"`
	Model          string             `json:"model"`
	ModelSelection *modelSelectionDoc `json:"model_selection"`
	Decision       string             `json:"decision"`
	Reasons        []string           `json:"reasons"`
	FixCycles      int                `json:"fix_cycles"`
	ExecutionMode  string             `json:"execution_mode"`
	VerifiedFirst  bool               `json:"verified_first"`
	Classification *classificationDoc `json:"classification"`
	JEV            *struct {
		Status   string            `json:"status"`
		Reason   string            `json:"reason"`
		Summary  string            `json:"summary"`
		Findings []json.RawMessage `json:"findings"`
	} `json:"jev"`
	GeneratedAt time.Time `json:"generated_at"`
}

type planMeta struct {
	Source    string `json:"source"`
	PlanID    string `json:"plan_id"`
	FinalGate string `json:"final_gate"`
}

func (s *Store) runDir(taskID string) string {
	return filepath.Join(s.root, ".agent-sdlc", "runs", taskID)
}

// readJSON decodes a JSON file into v. found is false when the file does not
// exist, so absence stays distinguishable from damage: a file that exists but
// cannot be read or parsed returns an error naming it, never a silent zero value.
func readJSON(path string, v any) (found bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return true, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return true, nil
}

// oneLine flattens a joined error into one message, because a template renders it
// as a single line of text.
func oneLine(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}

// runInfo assembles the latest run's record; a task that never ran is a zero
// RunInfo. An artifact that exists but cannot be read or parsed is returned as an
// error (one per artifact, joined) so a caller can report it instead of presenting
// the run as one that never happened.
func (s *Store) runInfo(taskID string) (RunInfo, error) {
	var ri RunInfo
	var errs []error
	dir := s.runDir(taskID)

	var st stateDoc
	switch found, err := readJSON(filepath.Join(dir, "state.json"), &st); {
	case err != nil:
		errs = append(errs, err)
	case found:
		ri.Present = true
		ri.Stage = st.Stage
		ri.UpdatedAt = st.UpdatedAt
	}

	var rep reportDoc
	switch found, err := readJSON(filepath.Join(dir, "report.json"), &rep); {
	case err != nil:
		errs = append(errs, err)
	case found:
		ri.Present = true
		if ri.Stage == "" {
			ri.Stage = rep.Stage
		}
		ri.Decision = rep.Decision
		ri.Reasons = rep.Reasons
		ri.FixCycles = rep.FixCycles
		ri.Provider = rep.Provider
		ri.Model = rep.Model
		if rep.ModelSelection != nil {
			ri.ModelSelection = ModelSelection{
				Present:  true,
				Class:    rep.ModelSelection.Class,
				Provider: rep.ModelSelection.Provider,
				Model:    rep.ModelSelection.Model,
				Locality: rep.ModelSelection.Locality,
				Source:   rep.ModelSelection.Source,
				Fallback: rep.ModelSelection.Fallback,
				Reason:   rep.ModelSelection.Reason,
			}
		}
		ri.ExecutionMode = rep.ExecutionMode
		ri.VerifiedFirst = rep.VerifiedFirst
		ri.GeneratedAt = rep.GeneratedAt
		if rep.Classification != nil {
			ri.Classification = toClassification(*rep.Classification)
		}
		if rep.JEV != nil {
			ri.JEV = &JEV{
				Status:   rep.JEV.Status,
				Reason:   rep.JEV.Reason,
				Summary:  rep.JEV.Summary,
				Findings: len(rep.JEV.Findings),
			}
		}
	}

	// classification.json is written whenever a run did not pass, even when no
	// report.json exists (infrastructure errors).
	if ri.Classification == nil {
		var cls classificationDoc
		switch found, err := readJSON(filepath.Join(dir, "classification.json"), &cls); {
		case err != nil:
			errs = append(errs, err)
		case found && cls.Disposition != "":
			ri.Present = true
			ri.Classification = toClassification(cls)
		}
	}
	return ri, errors.Join(errs...)
}

func toClassification(c classificationDoc) *Classification {
	return &Classification{
		Kind:        c.Kind,
		Disposition: c.Disposition,
		Confidence:  c.Confidence,
		Reason:      c.Reason,
	}
}

// maxActivityEvents keeps the most recent events; activity.jsonl grows across retries.
const maxActivityEvents = 200

// maxActivityLine bounds one activity line. A larger line is a dump; it is
// skipped so it cannot hide the events after it.
const maxActivityLine = 1024 * 1024

// readActivity returns the task's events in activity.jsonl append order (oldest
// first), capped to maxActivityEvents, plus how many lines were skipped as malformed
// or oversized. A missing artifact is no events and no error; an existing one that
// cannot be read is an error, never an empty history.
func (s *Store) readActivity(taskID string) (events []ActivityEvent, skipped int, err error) {
	raw, found, err := s.runFile(taskID, "activity.jsonl")
	if err != nil || !found {
		return nil, 0, err
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if len(line) > maxActivityLine {
			skipped++
			continue
		}
		var e activityDoc
		if json.Unmarshal(line, &e) != nil {
			skipped++ // skip a malformed line rather than fail the whole view
			continue
		}
		events = append(events, ActivityEvent{
			TaskID:    taskID,
			Stage:     e.Stage,
			Action:    e.Action,
			Detail:    sanitizeDetail(e.Detail),
			Timestamp: e.Timestamp,
		})
	}
	if len(events) > maxActivityEvents {
		events = events[len(events)-maxActivityEvents:]
	}
	return events, skipped, nil
}

// planMetaFile reads .agent-sdlc/plan.meta.json. found is false when SOP recorded
// no metadata; a file that exists but cannot be read or parsed is an error, so a
// broken plan record is never presented as "no active plan".
func (s *Store) planMetaFile() (meta planMeta, found bool, err error) {
	n, err := readJSON(filepath.Join(s.root, ".agent-sdlc", "plan.meta.json"), &meta)
	if err != nil {
		return planMeta{}, false, err
	}
	return meta, n, nil
}

// PlanSource returns the plan path SOP recorded in plan.meta.json. No metadata, or
// metadata that records no source, is an absence; a damaged file is an error.
func (s *Store) PlanSource() (string, bool, error) {
	meta, found, err := s.planMetaFile()
	if err != nil || !found {
		return "", false, err
	}
	src := strings.TrimSpace(meta.Source)
	return src, src != "", nil
}

// Plan returns the active plan and final gate. Without readable metadata,
// Recorded is false; Error names the damaged file so a view can tell a broken
// plan record from a project that has no plan.
func (s *Store) Plan() PlanGate {
	meta, found, err := s.planMetaFile()
	if err != nil {
		return PlanGate{FinalGate: StatusUnknown, Error: oneLine(err)}
	}
	if !found {
		return PlanGate{FinalGate: StatusUnknown}
	}
	src := strings.TrimSpace(meta.Source)
	gate := strings.TrimSpace(meta.FinalGate)
	if gate == "" {
		gate = StatusUnknown
	}
	return PlanGate{
		Recorded:   true,
		Source:     src,
		PlanID:     meta.PlanID,
		FinalGate:  gate,
		InProgress: gate == StatusUnknown,
	}
}

const reportArtifact = "report.json"

// ReportRef locates the task's report artifact. Missing is Present=false with no
// path and no error; any other stat failure is recorded in Error, so an artifact
// that exists but cannot be reached is never reported as absent.
func (s *Store) ReportRef(taskID string) ReportRef {
	ref := ReportRef{TaskID: taskID}
	path := filepath.Join(s.runDir(taskID), reportArtifact)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			ref.Error = fmt.Sprintf("%s: %v", reportArtifact, err)
		}
		return ref
	}
	ref.Present = true
	ref.Path = path
	ref.Name = reportArtifact
	return ref
}

// ReportRef locates SOP's report under .agent-sdlc/runs/<task-id>/.
type ReportRef struct {
	Present bool
	TaskID  string
	// Path is absolute, or "" when absent.
	Path string
	Name string
	// Error is set when the artifact could not be reached (for example a
	// permission or path error). Present is false in that case, but the artifact
	// is not absent, so a reader must not present the missing link as "no report".
	Error string
}
