package sopclient

import (
	"bufio"
	"bytes"
	"encoding/json"
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

// readJSON decodes a JSON file into v, reporting whether it existed and parsed.
func readJSON(path string, v any) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// runInfo assembles the latest run's record; a task that never ran is a zero RunInfo.
func (s *Store) runInfo(taskID string) RunInfo {
	var ri RunInfo
	dir := s.runDir(taskID)

	var st stateDoc
	if readJSON(filepath.Join(dir, "state.json"), &st) {
		ri.Present = true
		ri.Stage = st.Stage
		ri.UpdatedAt = st.UpdatedAt
	}

	var rep reportDoc
	if readJSON(filepath.Join(dir, "report.json"), &rep) {
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
		if readJSON(filepath.Join(dir, "classification.json"), &cls) && cls.Disposition != "" {
			ri.Present = true
			ri.Classification = toClassification(cls)
		}
	}
	return ri
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

// activity returns the task's events in activity.jsonl append order (oldest
// first), capped to maxActivityEvents, skipping malformed lines. Detail goes
// through sanitizeDetail.
func (s *Store) activity(taskID string) []ActivityEvent {
	raw, ok := s.runFile(taskID, "activity.jsonl")
	if !ok {
		return nil
	}
	var out []ActivityEvent
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e activityDoc
		if json.Unmarshal(line, &e) != nil {
			continue // skip a malformed line rather than fail the whole view
		}
		out = append(out, ActivityEvent{
			TaskID:    taskID,
			Stage:     e.Stage,
			Action:    e.Action,
			Detail:    sanitizeDetail(e.Detail),
			Timestamp: e.Timestamp,
		})
	}
	if len(out) > maxActivityEvents {
		out = out[len(out)-maxActivityEvents:]
	}
	return out
}

// PlanSource returns the plan path SOP recorded in plan.meta.json.
func (s *Store) PlanSource() (string, bool) {
	var meta planMeta
	if !readJSON(filepath.Join(s.root, ".agent-sdlc", "plan.meta.json"), &meta) {
		return "", false
	}
	src := strings.TrimSpace(meta.Source)
	return src, src != ""
}

// Plan returns the active plan and final gate; without metadata, Recorded is
// false and FinalGate is StatusUnknown.
func (s *Store) Plan() PlanGate {
	var meta planMeta
	if !readJSON(filepath.Join(s.root, ".agent-sdlc", "plan.meta.json"), &meta) {
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

// ReportRef locates the task's report artifact; missing is Present=false with no path.
func (s *Store) ReportRef(taskID string) ReportRef {
	ref := ReportRef{TaskID: taskID}
	path := filepath.Join(s.runDir(taskID), reportArtifact)
	if _, err := os.Stat(path); err != nil {
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
}
