package sopclient

import (
	"path/filepath"
	"strings"
	"time"
)

// Performance is SOP's measurement, read from its diagnostic artifacts:
//
//	.agent-sdlc/runs/<task-id>/metrics.json   per task (agentic-sop perf.Task)
//	.agent-sdlc/runs/<plan-id>/metrics.json   per run  (agentic-sop perf.Run)
//	.agent-sdlc/runs/<task-id>/report.json    the task's "performance" field
//
// The controller starts no timer and never lets performance influence a
// lifecycle decision. A missing record is Present=false, never zero-filled.

// SOP's StagesMS / ValidationMS map keys (agentic-sop internal/perf).
const (
	perfStagePlan       = "plan"
	perfStageImplement  = "implement"
	perfStageValidation = "validation"
	perfStageReview     = "review"
	perfStageFix        = "fix"

	perfCategoryBuild = "build"
	perfCategoryTest  = "test"
	perfCategoryLint  = "lint"
)

const metricsFileName = "metrics.json"

// Performance projects SOP's per-task record (perf.Task) verbatim.
type Performance struct {
	// Present is true when the record carries at least one measurement or count.
	Present bool

	Total time.Duration

	// Stage durations (StagesMS); zero means SOP recorded none.
	Plan       time.Duration
	Implement  time.Duration
	Validation time.Duration
	Review     time.Duration
	Fix        time.Duration

	// Validation breakdown (ValidationMS).
	Build time.Duration
	Test  time.Duration
	Lint  time.Duration

	// Operation counts (perf.Counts).
	AgentCalls        int
	AgentCallsAvoided int
	ValidationRuns    int
	ValidationReused  int
	ReviewRuns        int
	ReviewReused      int
	FixCycles         int
	PlanRepairs       int
}

// Agent is plan + implement + fix, SOP's own category rule.
func (p Performance) Agent() time.Duration { return p.Plan + p.Implement + p.Fix }

// CategoryShare is one category's duration and whole-percent share of the
// measured stage time. Presentation only.
type CategoryShare struct {
	Name     string // Agent / Validation / Review
	Duration time.Duration
	Percent  int
}

// Shares returns the agent/validation/review split, or nil when nothing was
// measured (so no 0% is shown).
func (p Performance) Shares() []CategoryShare {
	return categoryShares(p.Agent(), p.Validation, p.Review)
}

// PlanPerformance projects SOP's plan-level aggregate (perf.Run) at
// .agent-sdlc/runs/<plan-id>/metrics.json.
type PlanPerformance struct {
	Present bool
	PlanID  string // plan.meta.json "plan_id"
	Tasks   int    // task records folded into the aggregate
	Total   time.Duration
	// Category totals summed over the task records (agent = plan+implement+fix).
	Agent      time.Duration
	Validation time.Duration
	Review     time.Duration

	// Counts SOP already aggregated (perf.Run.Counts).
	AgentCalls        int
	AgentCallsAvoided int
	ValidationRuns    int
	ValidationReused  int
	ReviewRuns        int
	ReviewReused      int
	FixCycles         int
	PlanRepairs       int
}

// Shares is Performance.Shares for the run.
func (p PlanPerformance) Shares() []CategoryShare {
	return categoryShares(p.Agent, p.Validation, p.Review)
}

func categoryShares(agent, validation, review time.Duration) []CategoryShare {
	measured := agent + validation + review
	if measured <= 0 {
		return nil
	}
	return []CategoryShare{
		{Name: "Agent", Duration: agent, Percent: percentOf(agent, measured)},
		{Name: "Validation", Duration: validation, Percent: percentOf(validation, measured)},
		{Name: "Review", Duration: review, Percent: percentOf(review, measured)},
	}
}

// percentOf rounds like agentic-sop's report so the numbers match.
func percentOf(part, whole time.Duration) int {
	w := whole.Milliseconds()
	if w <= 0 {
		return 0
	}
	return int((part.Milliseconds()*100 + w/2) / w)
}

// metricsCountsDoc mirrors agentic-sop perf.Counts' JSON encoding.
type metricsCountsDoc struct {
	AgentCalls        int `json:"agent_calls"`
	AgentCallsAvoided int `json:"agent_calls_avoided"`
	ValidationRuns    int `json:"validation_runs"`
	ValidationReused  int `json:"validation_reused"`
	ReviewRuns        int `json:"review_runs"`
	ReviewReused      int `json:"review_reused"`
	FixCycles         int `json:"fix_cycles"`
	PlanRepairs       int `json:"plan_repairs"`
}

// taskMetricsDoc mirrors agentic-sop perf.Task's JSON encoding.
type taskMetricsDoc struct {
	ID           string           `json:"id"`
	TotalMS      int64            `json:"total_ms"`
	StagesMS     map[string]int64 `json:"stages_ms"`
	ValidationMS map[string]int64 `json:"validation_ms"`
	Counts       metricsCountsDoc `json:"counts"`
}

// runMetricsDoc mirrors agentic-sop perf.Run's JSON encoding.
type runMetricsDoc struct {
	StartedAt time.Time        `json:"started_at"`
	TotalMS   int64            `json:"total_ms"`
	Tasks     []taskMetricsDoc `json:"tasks"`
	Counts    metricsCountsDoc `json:"counts"`
}

func msToDuration(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// performanceFromTaskDoc returns ok=false for a record with no measurement.
func performanceFromTaskDoc(doc taskMetricsDoc) (Performance, bool) {
	p := Performance{
		Total:             msToDuration(doc.TotalMS),
		Plan:              msToDuration(doc.StagesMS[perfStagePlan]),
		Implement:         msToDuration(doc.StagesMS[perfStageImplement]),
		Validation:        msToDuration(doc.StagesMS[perfStageValidation]),
		Review:            msToDuration(doc.StagesMS[perfStageReview]),
		Fix:               msToDuration(doc.StagesMS[perfStageFix]),
		Build:             msToDuration(doc.ValidationMS[perfCategoryBuild]),
		Test:              msToDuration(doc.ValidationMS[perfCategoryTest]),
		Lint:              msToDuration(doc.ValidationMS[perfCategoryLint]),
		AgentCalls:        doc.Counts.AgentCalls,
		AgentCallsAvoided: doc.Counts.AgentCallsAvoided,
		ValidationRuns:    doc.Counts.ValidationRuns,
		ValidationReused:  doc.Counts.ValidationReused,
		ReviewRuns:        doc.Counts.ReviewRuns,
		ReviewReused:      doc.Counts.ReviewReused,
		FixCycles:         doc.Counts.FixCycles,
		PlanRepairs:       doc.Counts.PlanRepairs,
	}
	if !p.hasAny() {
		return Performance{}, false
	}
	p.Present = true
	return p, true
}

func (p Performance) hasAny() bool {
	return p.Total > 0 || p.Plan > 0 || p.Implement > 0 || p.Validation > 0 ||
		p.Review > 0 || p.Fix > 0 ||
		p.AgentCalls != 0 || p.AgentCallsAvoided != 0 || p.ValidationRuns != 0 ||
		p.ValidationReused != 0 || p.ReviewRuns != 0 || p.ReviewReused != 0 ||
		p.FixCycles != 0 || p.PlanRepairs != 0
}

// Performance reads the task's metrics.json, falling back to report.json's
// "performance" field (SOP's own read order). Missing or malformed is absent.
func (s *Store) Performance(taskID string) Performance {
	var doc taskMetricsDoc
	if readJSON(filepath.Join(s.runDir(taskID), metricsFileName), &doc) {
		if p, ok := performanceFromTaskDoc(doc); ok {
			return p
		}
	}
	var rep struct {
		Performance taskMetricsDoc `json:"performance"`
	}
	if readJSON(filepath.Join(s.runDir(taskID), reportArtifact), &rep) {
		if p, ok := performanceFromTaskDoc(rep.Performance); ok {
			return p
		}
	}
	return Performance{}
}

// PlanPerformance reads the aggregate for plan.meta.json's plan_id; no plan or
// no aggregate is absent.
func (s *Store) PlanPerformance() PlanPerformance {
	var meta planMeta
	if !readJSON(filepath.Join(s.root, ".agent-sdlc", "plan.meta.json"), &meta) {
		return PlanPerformance{}
	}
	planID := strings.TrimSpace(meta.PlanID)
	if planID == "" {
		return PlanPerformance{}
	}
	var doc runMetricsDoc
	if !readJSON(filepath.Join(s.root, ".agent-sdlc", "runs", planID, metricsFileName), &doc) {
		return PlanPerformance{}
	}
	return planPerformanceFromRunDoc(planID, doc)
}

func planPerformanceFromRunDoc(planID string, doc runMetricsDoc) PlanPerformance {
	p := PlanPerformance{
		PlanID:            planID,
		Tasks:             len(doc.Tasks),
		Total:             msToDuration(doc.TotalMS),
		AgentCalls:        doc.Counts.AgentCalls,
		AgentCallsAvoided: doc.Counts.AgentCallsAvoided,
		ValidationRuns:    doc.Counts.ValidationRuns,
		ValidationReused:  doc.Counts.ValidationReused,
		ReviewRuns:        doc.Counts.ReviewRuns,
		ReviewReused:      doc.Counts.ReviewReused,
		FixCycles:         doc.Counts.FixCycles,
		PlanRepairs:       doc.Counts.PlanRepairs,
	}
	for _, t := range doc.Tasks {
		p.Agent += msToDuration(t.StagesMS[perfStagePlan] + t.StagesMS[perfStageImplement] + t.StagesMS[perfStageFix])
		p.Validation += msToDuration(t.StagesMS[perfStageValidation])
		p.Review += msToDuration(t.StagesMS[perfStageReview])
	}
	if !p.hasAny() {
		return PlanPerformance{}
	}
	p.Present = true
	return p
}

func (p PlanPerformance) hasAny() bool {
	return len(p.PlanID) > 0 && (p.Tasks > 0 || p.Total > 0 || p.Agent > 0 ||
		p.Validation > 0 || p.Review > 0 ||
		p.AgentCalls != 0 || p.AgentCallsAvoided != 0 || p.ValidationRuns != 0 ||
		p.ValidationReused != 0 || p.ReviewRuns != 0 || p.ReviewReused != 0 ||
		p.FixCycles != 0 || p.PlanRepairs != 0)
}
