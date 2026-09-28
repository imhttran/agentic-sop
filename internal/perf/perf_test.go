package perf

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRecorderMeasuresWithInjectedClock(t *testing.T) {
	now := time.Unix(0, 0)
	r := WithClock("T1", func() time.Time { return now })

	stop := r.Measure(StagePlan)
	now = now.Add(2100 * time.Millisecond)
	stop()

	valStop := r.Measure(StageValidation)
	now = now.Add(8400 * time.Millisecond)
	valStop()
	r.AddValidation(CategoryTest, 8400*time.Millisecond)

	r.ValidationRun()
	r.AgentCall()
	r.AgentCallAvoided()
	r.ReviewRun()
	r.FixCycle()

	now = now.Add(time.Second)
	task := r.Task()

	if task.ID != "T1" {
		t.Errorf("id = %q", task.ID)
	}
	if task.StagesMS[StagePlan] != 2100 || task.StagesMS[StageValidation] != 8400 {
		t.Errorf("stages = %v", task.StagesMS)
	}
	if task.ValidationMS[CategoryTest] != 8400 {
		t.Errorf("validation categories = %v", task.ValidationMS)
	}
	if task.TotalMS != 11_500 {
		t.Errorf("total = %d, want 11500", task.TotalMS)
	}
	want := Counts{AgentCalls: 1, AgentCallsAvoided: 1, ValidationRuns: 1, ReviewRuns: 1, FixCycles: 1}
	if task.Counts != want {
		t.Errorf("counts = %+v, want %+v", task.Counts, want)
	}
}

func TestRunAggregatesTasksAndCategories(t *testing.T) {
	start := time.Unix(100, 0)
	run := NewRun(start)
	run.Add(Task{
		ID:       "T1",
		StagesMS: map[string]int64{StagePlan: 1000, StageImplement: 2000, StageValidation: 4000, StageReview: 3000},
		Counts:   Counts{AgentCalls: 2, ValidationRuns: 1, ReviewRuns: 1},
	})
	run.Add(Task{
		ID:       "T2",
		StagesMS: map[string]int64{StageValidation: 1000},
		Counts:   Counts{AgentCallsAvoided: 1, ValidationRuns: 1},
	})
	run.Finish(start.Add(3 * time.Second))

	if run.TotalMS != 3000 {
		t.Errorf("total = %d", run.TotalMS)
	}
	agent, validation, review := run.CategoryMS()
	if agent != 3000 || validation != 5000 || review != 3000 {
		t.Errorf("categories = (%d,%d,%d), want (3000,5000,3000)", agent, validation, review)
	}
	if run.Counts.AgentCalls != 2 || run.Counts.AgentCallsAvoided != 1 || run.Counts.ValidationRuns != 2 {
		t.Errorf("counts = %+v", run.Counts)
	}

	var b bytes.Buffer
	WriteRun(&b, *run)
	out := b.String()
	for _, want := range []string{"Performance", "Tasks:       2", "Total:       3.0s", "Agent calls:            2", "Agent calls avoided:    1"} {
		if !strings.Contains(out, want) {
			t.Errorf("run output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteTaskRendersStagesAndCounts(t *testing.T) {
	task := Task{
		ID:           "AHV2004",
		TotalMS:      40_500,
		StagesMS:     map[string]int64{StagePlan: 2100, StageImplement: 18_700, StageValidation: 13_600, StageReview: 6100},
		ValidationMS: map[string]int64{CategoryBuild: 3200, CategoryTest: 8400, CategoryLint: 2000},
		Counts:       Counts{AgentCalls: 2, ValidationRuns: 1},
	}
	var b bytes.Buffer
	WriteTask(&b, task)
	out := b.String()
	for _, want := range []string{"PLAN", "IMPLEMENT", "BUILD", "TEST", "LINT", "REVIEW", "TOTAL", "40.5s", "Agent calls: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("task output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "VALIDATION") {
		t.Errorf("categories should replace the validation row:\n%s", out)
	}
}

func TestHumanMS(t *testing.T) {
	cases := map[int64]string{
		0:       "0ms",
		820:     "820ms",
		2100:    "2.1s",
		59_999:  "60.0s",
		60_000:  "1m 0s",
		402_000: "6m 42s",
	}
	for ms, want := range cases {
		if got := humanMS(ms); got != want {
			t.Errorf("humanMS(%d) = %q, want %q", ms, got, want)
		}
	}
}
