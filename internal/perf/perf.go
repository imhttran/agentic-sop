// Package perf is SOP's single performance model: where a task and a run spend
// their time, and how many agent calls, validations, reviews, and fix cycles they
// cost. It is diagnostic metadata only — timing never influences a workflow
// decision, a gate, or task state.
//
// Durations are recorded in milliseconds and measured with the monotonic clock
// (time.Since), so a wall-clock adjustment cannot make a stage look wrong. The
// schema is a single documented representation reused by the per-task summary,
// the persisted metrics.json, and `sop report`.
package perf

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Top-level stage names. They are the single vocabulary for where a task spends
// its time; a stage is only recorded when its work actually ran.
const (
	StagePlan       = "plan"
	StageImplement  = "implement"
	StageValidation = "validation"
	StageReview     = "review"
	StageFix        = "fix"
)

// Validation category names, derived from the runner's per-check durations.
const (
	CategoryBuild = "build"
	CategoryTest  = "test"
	CategoryLint  = "lint"
)

// Counts are the non-duration operation counts for one task or a whole run.
type Counts struct {
	// AgentCalls is the number of implementation-agent calls (PLAN, IMPLEMENT,
	// FIX). REVIEW is an agent call too but is counted separately as ReviewRuns.
	AgentCalls int `json:"agent_calls"`
	// AgentCallsAvoided is the number of implementation-agent calls the
	// verify-first fast path skipped.
	AgentCallsAvoided int `json:"agent_calls_avoided"`
	// ValidationRuns and ValidationReused count executed versus safely reused
	// validation suites.
	ValidationRuns   int `json:"validation_runs"`
	ValidationReused int `json:"validation_reused"`
	// ReviewRuns and ReviewReused count executed versus safely reused reviews.
	ReviewRuns   int `json:"review_runs"`
	ReviewReused int `json:"review_reused"`
	// FixCycles is the number of fix iterations taken.
	FixCycles int `json:"fix_cycles"`
	// PlanRepairs is the number of times an invalid generated plan was returned
	// to the PLAN agent for correction before generation succeeded or failed.
	PlanRepairs int `json:"plan_repairs"`
}

// JEV is the diagnostic record of the optional JEV analysis invoked at the
// quality seam. It captures the JEV invocation count, duration, provider/model,
// tool calls, finding count, and blocking finding count, so the cost/time of JEV
// is distinguishable from PLAN, IMPLEMENT, FIX, validation, and review (which
// are accounted for separately in StagesMS/Counts).
//
// Like every other value in this package it is diagnostic metadata only: it is
// never read back to drive a decision, and a missing or zero JEV record never
// implies PASS or FAIL. A task where JEV did not run simply carries no JEV
// record.
type JEV struct {
	// Invocations is the number of JEV invocations for the task. JEV reruns
	// after a fix, so this can exceed one.
	Invocations int `json:"invocations"`
	// TotalMS is the time spent across all of the task's JEV invocations, in
	// milliseconds.
	TotalMS int64 `json:"total_ms"`
	// Provider and Model identify what performed the analysis. They are empty
	// when the analyzer did not name them, which is not an error.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// ToolCalls is the number of tool/provider calls JEV made.
	ToolCalls int `json:"tool_calls"`
	// Findings is the finding count of the most recent JEV invocation.
	Findings int `json:"findings"`
	// BlockingFindings is the count of the most recent invocation's findings
	// that are blocking under the configured JEV policy.
	BlockingFindings int `json:"blocking_findings"`
}

// Task is the persisted performance record for one task lifecycle.
type Task struct {
	ID string `json:"id"`
	// TotalMS is the task's wall-clock duration in milliseconds.
	TotalMS int64 `json:"total_ms"`
	// StagesMS is the time spent in each top-level stage.
	StagesMS map[string]int64 `json:"stages_ms,omitempty"`
	// ValidationMS is the validation stage broken down by category (its values
	// sum to StagesMS[validation]).
	ValidationMS map[string]int64 `json:"validation_ms,omitempty"`
	Counts       Counts           `json:"counts"`
	// JEV is the task's JEV diagnostic record. It is omitted entirely when JEV
	// did not run, so an existing report without JEV metrics stays compatible.
	JEV *JEV `json:"jev,omitempty"`
}

// Run aggregates the tasks of one sop invocation.
type Run struct {
	StartedAt time.Time `json:"started_at"`
	TotalMS   int64     `json:"total_ms"`
	Tasks     []Task    `json:"tasks"`
	Counts    Counts    `json:"counts"`
	// JEV is the run's JEV diagnostic record, aggregated from its tasks. It is
	// omitted when no task ran JEV.
	JEV *JEV `json:"jev,omitempty"`
}

// Recorder accumulates timing and counts for one task lifecycle. Its clock is
// injectable so tests can supply deterministic timing.
type Recorder struct {
	id     string
	now    func() time.Time
	start  time.Time
	stages map[string]int64
	perCat map[string]int64
	counts Counts
	jev    *JEV
}

// NewRecorder returns a Recorder for the task, measuring with the monotonic clock.
func NewRecorder(id string) *Recorder {
	return WithClock(id, time.Now)
}

// WithClock returns a Recorder that reads elapsed time from now. It exists so
// tests can drive timing without sleeping.
func WithClock(id string, now func() time.Time) *Recorder {
	return &Recorder{
		id:     id,
		now:    now,
		start:  now(),
		stages: map[string]int64{},
		perCat: map[string]int64{},
	}
}

// Measure starts a stage timer and returns the function that stops it and records
// the elapsed time. It wraps an existing operation without changing its control
// flow.
func (r *Recorder) Measure(stage string) func() {
	start := r.now()
	return func() { r.Add(stage, r.now().Sub(start)) }
}

// Add records d against a stage.
func (r *Recorder) Add(stage string, d time.Duration) {
	if d < 0 {
		d = 0
	}
	r.stages[stage] += d.Milliseconds()
}

// AddValidation records a validation category's duration. The validation stage
// total is recorded separately through Measure(StageValidation), and this keeps
// the categories as a breakdown of it.
func (r *Recorder) AddValidation(category string, d time.Duration) {
	if d < 0 {
		d = 0
	}
	r.perCat[category] += d.Milliseconds()
}

// Count methods. Each records one occurrence of a measured operation.
func (r *Recorder) AgentCall()        { r.counts.AgentCalls++ }
func (r *Recorder) AgentCallAvoided() { r.counts.AgentCallsAvoided++ }
func (r *Recorder) ValidationRun()    { r.counts.ValidationRuns++ }
func (r *Recorder) ValidationReused() { r.counts.ValidationReused++ }
func (r *Recorder) ReviewRun()        { r.counts.ReviewRuns++ }
func (r *Recorder) ReviewReused()     { r.counts.ReviewReused++ }
func (r *Recorder) FixCycle()         { r.counts.FixCycles++ }
func (r *Recorder) PlanRepair()       { r.counts.PlanRepairs++ }

// JEVInvocation records one JEV invocation's diagnostics. Invocations, duration,
// and tool calls accumulate across a task's fix loop; the finding counts reflect
// the most recent invocation. It records metadata only — it performs no decision
// and a missing/zero value never implies PASS or FAIL.
func (r *Recorder) JEVInvocation(inv JEV) {
	if r.jev == nil {
		r.jev = &JEV{}
	}
	r.jev.Invocations++
	r.jev.TotalMS += inv.TotalMS
	if inv.Provider != "" {
		r.jev.Provider = inv.Provider
	}
	if inv.Model != "" {
		r.jev.Model = inv.Model
	}
	r.jev.ToolCalls += inv.ToolCalls
	r.jev.Findings = inv.Findings
	r.jev.BlockingFindings = inv.BlockingFindings
}

// Task snapshots the recorder into a persistable record.
func (r *Recorder) Task() Task {
	return Task{
		ID:           r.id,
		TotalMS:      r.now().Sub(r.start).Milliseconds(),
		StagesMS:     copyMap(r.stages),
		ValidationMS: copyMap(r.perCat),
		Counts:       r.counts,
		JEV:          r.jev,
	}
}

// NewRun returns a run aggregate started at start.
func NewRun(start time.Time) *Run { return &Run{StartedAt: start} }

// Add folds a task record into the run aggregate.
func (r *Run) Add(t Task) {
	r.Tasks = append(r.Tasks, t)
	r.Counts = addCounts(r.Counts, t.Counts)
	r.JEV = addJEV(r.JEV, t.JEV)
}

// Finish records the run's total wall-clock duration.
func (r *Run) Finish(now time.Time) {
	r.TotalMS = now.Sub(r.StartedAt).Milliseconds()
}

// CategoryMS returns the run's time split into the agent (PLAN/IMPLEMENT/FIX),
// validation, and review.
func (r Run) CategoryMS() (agent, validation, review int64) {
	for _, t := range r.Tasks {
		agent += t.StagesMS[StagePlan] + t.StagesMS[StageImplement] + t.StagesMS[StageFix]
		validation += t.StagesMS[StageValidation]
		review += t.StagesMS[StageReview]
	}
	return
}

// WriteTask renders one task's stage timings and counts, matching the shape of
// the per-task summary.
func WriteTask(w io.Writer, t Task) {
	for _, row := range taskRows(t) {
		fmt.Fprintf(w, "%-11s %s\n", row.name, humanMS(row.ms))
	}
	fmt.Fprintf(w, "%s\n", "----------------")
	fmt.Fprintf(w, "%-11s %s\n", "TOTAL", humanMS(t.TotalMS))

	fmt.Fprintf(w, "\nAgent calls: %d\n", t.Counts.AgentCalls)
	if t.Counts.AgentCallsAvoided > 0 {
		fmt.Fprintf(w, "Agent calls avoided: %d\n", t.Counts.AgentCallsAvoided)
	}
	fmt.Fprintf(w, "Validation runs: %d\n", t.Counts.ValidationRuns)
	if t.Counts.ValidationReused > 0 {
		fmt.Fprintf(w, "Validation reused: %d\n", t.Counts.ValidationReused)
	}
	fmt.Fprintf(w, "Review runs: %d\n", t.Counts.ReviewRuns)
	if t.Counts.ReviewReused > 0 {
		fmt.Fprintf(w, "Review reused: %d\n", t.Counts.ReviewReused)
	}
	fmt.Fprintf(w, "Fix cycles: %d\n", t.Counts.FixCycles)
	if t.Counts.PlanRepairs > 0 {
		fmt.Fprintf(w, "Plan repairs: %d\n", t.Counts.PlanRepairs)
	}
	WriteJEV(w, t.JEV)
}

// WriteRun renders a run-level performance summary. Percentages are derived from
// the measured categories and are omitted when no stage time was recorded.
func WriteRun(w io.Writer, r Run) {
	fmt.Fprintf(w, "Performance\n\n")
	fmt.Fprintf(w, "Tasks:       %d\n", len(r.Tasks))
	if r.TotalMS > 0 {
		fmt.Fprintf(w, "Total:       %s\n", humanMS(r.TotalMS))
	}

	agent, validation, review := r.CategoryMS()
	if measured := agent + validation + review; measured > 0 {
		fmt.Fprintf(w, "\nAgent:       %-10s (%d%%)\n", humanMS(agent), pct(agent, measured))
		fmt.Fprintf(w, "Validation:  %-10s (%d%%)\n", humanMS(validation), pct(validation, measured))
		fmt.Fprintf(w, "Review:      %-10s (%d%%)\n", humanMS(review), pct(review, measured))
	}

	fmt.Fprintf(w, "\nAgent calls:            %d\n", r.Counts.AgentCalls)
	fmt.Fprintf(w, "Agent calls avoided:    %d\n", r.Counts.AgentCallsAvoided)
	fmt.Fprintf(w, "Validation runs:        %d\n", r.Counts.ValidationRuns)
	fmt.Fprintf(w, "Validation reused:      %d\n", r.Counts.ValidationReused)
	fmt.Fprintf(w, "Review runs:            %d\n", r.Counts.ReviewRuns)
	fmt.Fprintf(w, "Review reused:          %d\n", r.Counts.ReviewReused)
	fmt.Fprintf(w, "Fix cycles:             %d\n", r.Counts.FixCycles)
	if r.Counts.PlanRepairs > 0 {
		fmt.Fprintf(w, "Plan repairs:           %d\n", r.Counts.PlanRepairs)
	}
	WriteJEV(w, r.JEV)
}

// WriteJEV renders a JEV diagnostic record, kept separate from the
// PLAN/IMPLEMENT/FIX and validation/review accounting so JEV cost/time is
// distinguishable from them. Nothing is printed when JEV did not run, so a
// report without JEV is unchanged. The values are diagnostics: they never assert
// PASS or FAIL.
func WriteJEV(w io.Writer, j *JEV) {
	if j == nil {
		return
	}
	fmt.Fprintf(w, "\nJEV (diagnostic)\n")
	fmt.Fprintf(w, "  Invocations: %d\n", j.Invocations)
	fmt.Fprintf(w, "  Duration: %s\n", humanMS(j.TotalMS))
	if line := strings.TrimSpace(j.Provider + " " + j.Model); line != "" {
		fmt.Fprintf(w, "  Provider/model: %s\n", line)
	}
	fmt.Fprintf(w, "  Tool calls: %d\n", j.ToolCalls)
	fmt.Fprintf(w, "  Findings: %d\n", j.Findings)
	fmt.Fprintf(w, "  Blocking findings: %d\n", j.BlockingFindings)
}

type row struct {
	name string
	ms   int64
}

// taskRows orders a task's stages for display: plan, implement, validation (or
// its categories), review, then fix. Zero-duration stages are omitted.
func taskRows(t Task) []row {
	var rows []row
	add := func(name string, ms int64) {
		if ms > 0 {
			rows = append(rows, row{name, ms})
		}
	}
	add(upper(StagePlan), t.StagesMS[StagePlan])
	add(upper(StageImplement), t.StagesMS[StageImplement])
	if len(t.ValidationMS) > 0 {
		add(upper(CategoryBuild), t.ValidationMS[CategoryBuild])
		add(upper(CategoryTest), t.ValidationMS[CategoryTest])
		add(upper(CategoryLint), t.ValidationMS[CategoryLint])
	} else {
		add(upper(StageValidation), t.StagesMS[StageValidation])
	}
	add(upper(StageReview), t.StagesMS[StageReview])
	add(upper(StageFix), t.StagesMS[StageFix])
	if len(rows) == 0 {
		rows = append(rows, row{upper(StageValidation), 0})
	}
	return rows
}

func upper(name string) string { return strings.ToUpper(name) }

// humanMS renders milliseconds compactly: "820ms", "2.1s", "1m 42s".
func humanMS(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm %ds", ms/60_000, (ms%60_000)/1000)
	}
}

// pct rounds part/whole to a whole percent.
func pct(part, whole int64) int64 {
	if whole <= 0 {
		return 0
	}
	return (part*100 + whole/2) / whole
}

func copyMap(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func addCounts(a, b Counts) Counts {
	return Counts{
		AgentCalls:        a.AgentCalls + b.AgentCalls,
		AgentCallsAvoided: a.AgentCallsAvoided + b.AgentCallsAvoided,
		ValidationRuns:    a.ValidationRuns + b.ValidationRuns,
		ValidationReused:  a.ValidationReused + b.ValidationReused,
		ReviewRuns:        a.ReviewRuns + b.ReviewRuns,
		ReviewReused:      a.ReviewReused + b.ReviewReused,
		FixCycles:         a.FixCycles + b.FixCycles,
		PlanRepairs:       a.PlanRepairs + b.PlanRepairs,
	}
}

// addJEV folds a task's JEV record into a run aggregate. A task with no JEV
// record (nil) leaves the aggregate unchanged, so a run where JEV never ran
// carries no JEV record.
func addJEV(a, b *JEV) *JEV {
	if b == nil {
		return a
	}
	if a == nil {
		a = &JEV{}
	}
	a.Invocations += b.Invocations
	a.TotalMS += b.TotalMS
	if b.Provider != "" {
		a.Provider = b.Provider
	}
	if b.Model != "" {
		a.Model = b.Model
	}
	a.ToolCalls += b.ToolCalls
	a.Findings += b.Findings
	a.BlockingFindings += b.BlockingFindings
	return a
}

// SortedStageNames returns the recorded stage names in a stable order.
func (t Task) SortedStageNames() []string {
	names := make([]string, 0, len(t.StagesMS))
	for name := range t.StagesMS {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Measured reports whether any timing or count was recorded, so a caller can
// avoid printing an empty summary.
func (t Task) Measured() bool {
	return t.TotalMS > 0 || len(t.StagesMS) > 0 || t.Counts != (Counts{}) || t.JEV != nil
}

// Line renders a one-line task summary for concise run output.
func (t Task) Line() string {
	agent, validation, review := Run{Tasks: []Task{t}}.CategoryMS()
	line := fmt.Sprintf("%s total (agent %s, validation %s, review %s) | agent calls %d, validation runs %d, fix cycles %d",
		humanMS(t.TotalMS), humanMS(agent), humanMS(validation), humanMS(review),
		t.Counts.AgentCalls, t.Counts.ValidationRuns, t.Counts.FixCycles)
	if t.Counts.PlanRepairs > 0 {
		line += fmt.Sprintf(", plan repairs %d", t.Counts.PlanRepairs)
	}
	if t.JEV != nil {
		line += fmt.Sprintf(", JEV invocations %d (%s)", t.JEV.Invocations, humanMS(t.JEV.TotalMS))
	}
	return line
}
