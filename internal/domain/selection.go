package domain

// IsBlockedSelectable reports whether a BLOCKED task is a candidate for
// recovery selection: it is BLOCKED, still recoverable through the existing
// requeue path (IsBlockedRecoverable, i.e. it has retry budget left), and every
// dependency it declares is satisfied. It is the single definition of "safe to
// re-evaluate" for recovery: the scheduler may only pick a BLOCKED task up again
// when doing so cannot bypass dependency ordering and cannot loop forever, and
// the pick is always executed through the existing Requeue path, so this
// predicate neither changes, duplicates, nor bypasses Requeue's budget
// accounting.
//
// A BLOCKED task with an unsatisfied dependency is not selectable, and neither
// is a terminally exhausted BLOCKED task (IsTerminalBlocked). Non-BLOCKED
// statuses are never selectable this way.
func (t *Task) IsBlockedSelectable(tasks map[string]*Task) bool {
	if !t.IsBlockedRecoverable() {
		return false
	}
	_, satisfied := t.ResolveDependencies(tasks)
	return satisfied
}
