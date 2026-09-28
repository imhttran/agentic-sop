package domain

import "testing"

// TestIsBlockedSelectableSemantics pins the single definition of "safe to
// re-evaluate": a BLOCKED task is selectable for recovery only when it is still
// recoverable through the requeue path (retry budget left) and all of its
// dependencies are satisfied.
func TestIsBlockedSelectableSemantics(t *testing.T) {
	done := &Task{ID: "T000", Status: DONE}
	blocked := &Task{ID: "T100", Status: BLOCKED, Attempt: 3, MaxAttempts: 3}

	cases := []struct {
		name string
		task *Task
		want bool
	}{
		{
			name: "blocked with budget and satisfied dependencies",
			task: &Task{ID: "T1", Status: BLOCKED, Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{"T000"}},
			want: true,
		},
		{
			name: "blocked with budget and no dependencies",
			task: &Task{ID: "T1", Status: BLOCKED, Attempt: 1, MaxAttempts: 0},
			want: true,
		},
		{
			name: "blocked with unsatisfied dependency",
			task: &Task{ID: "T1", Status: BLOCKED, Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{"T100"}},
			want: false,
		},
		{
			name: "blocked with exhausted budget",
			task: &Task{ID: "T1", Status: BLOCKED, Attempt: 3, MaxAttempts: 3},
			want: false,
		},
	}

	for _, c := range cases {
		tasks := map[string]*Task{"T000": done, "T100": blocked, c.task.ID: c.task}
		if got := c.task.IsBlockedSelectable(tasks); got != c.want {
			t.Errorf("%s: IsBlockedSelectable = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestIsBlockedSelectableOnlyForRecoverableBlocked asserts the predicate is false
// for every status in allStatuses() that is not a recoverable BLOCKED task, and
// that it agrees with IsBlockedRecoverable/IsTerminalBlocked for all of them.
func TestIsBlockedSelectableOnlyForRecoverableBlocked(t *testing.T) {
	for _, status := range allStatuses() {
		task := &Task{ID: "T1", Status: status, Attempt: 1, MaxAttempts: 3}
		tasks := map[string]*Task{"T1": task}

		got := task.IsBlockedSelectable(tasks)
		want := task.IsBlockedRecoverable()
		if got != want {
			t.Errorf("IsBlockedSelectable(%s) = %v, want %v (must agree with IsBlockedRecoverable)", status, got, want)
		}
		if task.IsTerminalBlocked() && got {
			t.Errorf("terminally blocked %s must not be selectable", status)
		}
		if status != BLOCKED && got {
			t.Errorf("non-BLOCKED status %s must not be selectable", status)
		}
	}
}

// TestIsBlockedSelectableNeverSelectsUnsatisfiedDependencies asserts the predicate
// never treats a blocked or in-flight dependency as satisfied, matching
// TestResolveDependenciesNeverTreatsBlockedOrInflightAsSatisfied.
func TestIsBlockedSelectableNeverSelectsUnsatisfiedDependencies(t *testing.T) {
	notSatisfying := []TaskStatus{
		BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED, IMPLEMENTING,
		LOCAL_TESTS_PASS, REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS,
		FIX_REQUIRED, BLOCKED, PLANNED, READY,
	}
	for _, status := range notSatisfying {
		task := &Task{ID: "T2", Status: BLOCKED, Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{"T1"}}
		tasks := map[string]*Task{
			"T1": {ID: "T1", Status: status},
			"T2": task,
		}
		if task.IsBlockedSelectable(tasks) {
			t.Errorf("dependency in %s must not make a blocked task selectable", status)
		}
	}
}
