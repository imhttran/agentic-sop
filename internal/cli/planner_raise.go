package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/durablefs"
	"github.com/imhttran/agentic-sop/internal/question"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// supportedReturnStage reports whether the resumed lifecycle can enter a return stage
// directly without repeating earlier completed stages. Only IMPLEMENTING is supported.
func supportedReturnStage(stage string) bool {
	return runpkg.Stage(strings.TrimSpace(stage)) == runpkg.Implementing
}

// plannerEffectPath is the durable effect marker for one continuation, inside the
// task's run directory. Its name is sanitized because a continuation ID contains ':'.
func plannerEffectPath(dir, taskID, contID string) string {
	name := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(contID)
	return filepath.Join(dir, config.DirName, "runs", taskID, "planner-effect-"+name+".json")
}

// writePlannerEffect durably records that a continuation's stage work succeeded, so a
// crash before completion is recovered without replaying the effect. It is written only
// after the run checkpoint has been durably persisted.
func writePlannerEffect(dir, taskID, contID string, generation int) error {
	rec := map[string]any{"continuation_id": contID, "generation": generation, "at": time.Now().UTC()}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return durablefs.WriteFile(plannerEffectPath(dir, taskID, contID), data)
}

// markerState is the durable state of a marker file: absent, well-formed, or uncertain
// (present but unreadable or malformed). Uncertainty must NEVER be read as "absent", so a
// corrupt marker is treated as uncertain execution requiring review rather than as a
// missing marker that would allow a replay.
type markerState int

const (
	markerAbsent markerState = iota
	markerOK
	markerUncertain
)

// readMarker classifies a durable marker file. A missing file is absent; a present,
// well-formed marker is ok; a present-but-unreadable or malformed marker is uncertain.
func readMarker(path string) markerState {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return markerAbsent
		}
		return markerUncertain
	}
	var rec struct {
		ContinuationID string `json:"continuation_id"`
	}
	if err := json.Unmarshal(data, &rec); err != nil || strings.TrimSpace(rec.ContinuationID) == "" {
		return markerUncertain
	}
	return markerOK
}

// plannerEffectState reports the state of a continuation's success marker.
func plannerEffectState(dir, taskID, contID string) markerState {
	return readMarker(plannerEffectPath(dir, taskID, contID))
}

// plannerEffectStartedState reports the state of a continuation's start marker.
func plannerEffectStartedState(dir, taskID, contID string) markerState {
	return readMarker(plannerEffectStartedPath(dir, taskID, contID))
}

// plannerEffectStartedPath is the durable marker written BEFORE a resumed attempt runs
// stage work, so a crash AFTER the effect but BEFORE the success marker is detected as
// uncertain rather than silently replayed. It is a separate file from the success marker.
func plannerEffectStartedPath(dir, taskID, contID string) string {
	name := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(contID)
	return filepath.Join(dir, config.DirName, "runs", taskID, "planner-effect-"+name+".started.json")
}

// writePlannerEffectStarted durably records that a resumed attempt is BEGINNING. Its
// presence WITHOUT a success marker is the signal that the effect's outcome is uncertain.
func writePlannerEffectStarted(dir, taskID, contID string) error {
	rec := map[string]any{"continuation_id": contID, "at": time.Now().UTC()}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return durablefs.WriteFile(plannerEffectStartedPath(dir, taskID, contID), data)
}

// raisePlannerQuestion persists the structured question an executor raised during
// implementation, bound to live state. It enforces the opt-in toggle and the bounded
// planner round limit. It carries no approval authority.
func raisePlannerQuestion(dir, taskID string, req *agent.PlannerRequest) (*question.Question, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("planner: %w", err)
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return nil, err
	}
	settings := plannerSettings(cfg)
	if !settings.Enabled {
		return nil, fmt.Errorf("planner: mode is disabled (set planner.enabled: true)")
	}
	timeout, terr := plannerTimeout(cfg)
	if terr != nil {
		return nil, terr
	}
	svc, _, err := plannerService(dir)
	if err != nil {
		return nil, err
	}
	existing, err := questionsForTask(svc, taskID)
	if err != nil {
		return nil, err
	}
	if len(existing)+1 > settings.RoundLimit {
		return nil, fmt.Errorf("planner: round limit %d reached for task %s", settings.RoundLimit, taskID)
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = fmt.Sprintf("planner-round-%d", len(existing)+1)
	}
	stage := strings.TrimSpace(req.ReturnStage)
	if stage == "" {
		stage = string(runpkg.Implementing)
	}
	if !supportedReturnStage(stage) {
		return nil, fmt.Errorf("planner: unsupported return stage %q", stage)
	}
	return svc.CreateQuestion(&question.Question{
		Identity:        taskID,
		Kind:            kind,
		Body:            strings.TrimSpace(req.Question),
		Options:         req.Options,
		SubmittedBy:     "executor",
		Project:         projectIdentity(dir),
		RunID:           taskID,
		RunEpoch:        currentRunEpoch(dir, taskID),
		PlanRevision:    currentPlanRevision(dir),
		RepoFingerprint: repoFingerprint(dir),
		ReturnStage:     stage,
		Revision:        1,
		DeadlineAfter:   timeout,
	})
}
