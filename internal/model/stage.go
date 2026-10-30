package model

import (
	"fmt"
	"os"
	"strings"
)

// Stage names a supporting lifecycle stage whose model class can be pinned
// independently of the implementing class.
type Stage string

const (
	// StagePlan is the task-level plan generated before implementation. It runs
	// before per-task routing, so it can never follow the routed class.
	StagePlan Stage = "plan"
	// StageReview is the agent review of a task's change.
	StageReview Stage = "review"
	// StageJEV is the early JEV analysis (task triage and pre-execution) whose
	// evidence feeds the router.
	StageJEV Stage = "jev"
)

// Stages lists the pinnable stages in a stable order.
var Stages = []Stage{StagePlan, StageReview, StageJEV}

// StageClasses is the optional per-stage class table (models.stages). An empty
// entry keeps that stage's existing selection.
type StageClasses struct {
	Plan   Class `yaml:"plan"`
	Review Class `yaml:"review"`
	JEV    Class `yaml:"jev"`
}

func (s StageClasses) class(st Stage) Class {
	switch st {
	case StagePlan:
		return s.Plan
	case StageReview:
		return s.Review
	case StageJEV:
		return s.JEV
	}
	return ""
}

// StageEnvKey returns the environment variable that pins a stage's class, for
// example SOP_MODEL_STAGE_REVIEW_CLASS.
func StageEnvKey(st Stage) string {
	return "SOP_MODEL_STAGE_" + strings.ToUpper(string(st)) + "_CLASS"
}

// StageClass resolves the class pinned for a stage: the environment overrides
// models.stages, and "" means the stage is not pinned. An unknown class is an
// actionable error naming the setting.
func StageClass(config Route, st Stage, lookup func(string) string) (Class, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	if v := strings.TrimSpace(lookup(StageEnvKey(st))); v != "" {
		c, err := ParseClass(v)
		if err != nil {
			return "", fmt.Errorf("model routing: %s: %w", StageEnvKey(st), err)
		}
		return c, nil
	}
	c := Class(strings.ToLower(strings.TrimSpace(string(config.Stages.class(st)))))
	if c != "" && !c.Valid() {
		return "", fmt.Errorf("model routing: unknown stages.%s %q (want %s)", st, c, joinClasses())
	}
	return c, nil
}

// ReasonStagePin: a stage named its own provider and model.
const ReasonStagePin = "stage provider/model pin"

// StageProviderEnvKey and StageNameEnvKey name the settings that pin a stage
// straight to a provider and model (for example SOP_MODEL_STAGE_REVIEW_PROVIDER),
// bypassing the shared class table. That lets a model that cannot implement, such
// as a local read-only one, take a supporting stage without being reachable by
// task routing.
func StageProviderEnvKey(st Stage) string {
	return "SOP_MODEL_STAGE_" + strings.ToUpper(string(st)) + "_PROVIDER"
}

// StageNameEnvKey: see StageProviderEnvKey.
func StageNameEnvKey(st Stage) string {
	return "SOP_MODEL_STAGE_" + strings.ToUpper(string(st)) + "_NAME"
}

// StageDirect resolves a stage pinned straight to a provider and model, and
// reports whether one is set. Both settings are required together, the provider
// must be known, and a class pin for the same stage is a conflict rather than a
// silent precedence.
func StageDirect(st Stage, lookup func(string) string) (Selection, bool, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	provider := strings.TrimSpace(lookup(StageProviderEnvKey(st)))
	name := strings.TrimSpace(lookup(StageNameEnvKey(st)))
	if provider == "" && name == "" {
		return Selection{}, false, nil
	}
	switch {
	case provider == "" || name == "":
		return Selection{}, false, fmt.Errorf("model routing: set both %s and %s, or neither", StageProviderEnvKey(st), StageNameEnvKey(st))
	case !knownProviders[provider]:
		return Selection{}, false, fmt.Errorf("model routing: %s: unknown provider %q (want one of %s)", StageProviderEnvKey(st), provider, strings.Join(KnownProviders(), ", "))
	case strings.TrimSpace(lookup(StageEnvKey(st))) != "":
		return Selection{}, false, fmt.Errorf("model routing: stage %s is pinned by both %s and %s; set one", st, StageEnvKey(st), StageProviderEnvKey(st))
	}
	return Selection{Provider: provider, Model: name, Source: SourceEnv, Reason: ReasonStagePin}, true, nil
}
