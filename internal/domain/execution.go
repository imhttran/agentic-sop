package domain

import "strings"

// ExecutionMode decides how a task reaches its deterministic gates. It is part of
// the plan/task metadata, never inferred from a task's title or prose.
type ExecutionMode string

const (
	// ExecutionImplement is the default: the implementation agent runs first and
	// its result is validated. The zero value ("") is treated as this mode, so
	// tasks created before the mode existed keep their behaviour.
	ExecutionImplement ExecutionMode = "implement"
	// ExecutionVerifyFirst runs the configured deterministic validation before any
	// agent is invoked. When it passes, no agent runs at all; when it fails, the
	// failure is handed to the implementation agent as context.
	ExecutionVerifyFirst ExecutionMode = "verify-first"
	// ExecutionDone declares that a stage's work already exists in the tree, so the
	// plan does not ask SOP to implement it again. A stage declared done is recorded
	// as already satisfied when its task is built, and is never selected for
	// execution: the operator declared the work complete and there is no
	// implementation work left to request. It is explicit plan metadata, never
	// inferred from a title or prose, and a model may never set it.
	ExecutionDone ExecutionMode = "done"
)

// VerifyFirst reports whether the mode runs deterministic verification before the
// implementation agent. The zero value (implement) is not verify-first.
func (m ExecutionMode) VerifyFirst() bool { return m == ExecutionVerifyFirst }

// Done reports whether the mode declares the stage's work already exists, so its
// task is recorded as satisfied instead of implemented.
func (m ExecutionMode) Done() bool { return m == ExecutionDone }

// KnownExecutionMode reports whether m is a recognized execution mode, treating
// the zero value as the implement default.
func KnownExecutionMode(m ExecutionMode) bool {
	switch m {
	case "", ExecutionImplement, ExecutionVerifyFirst, ExecutionDone:
		return true
	default:
		return false
	}
}

// ParseExecutionMode normalizes a human-written execution-mode value and reports
// whether it is recognized. "verify first", "Verify_First", and "verify-first"
// all parse to ExecutionVerifyFirst; "implement" parses to ExecutionImplement, and
// the completion spellings ("done", "complete", "completed", "already done",
// "implemented") parse to ExecutionDone. An unrecognized value reports false so
// the caller decides whether that is an error (a plan) or something to ignore (a
// task file).
func ParseExecutionMode(s string) (ExecutionMode, bool) {
	norm := strings.ToLower(strings.TrimSpace(s))
	norm = strings.Join(strings.FieldsFunc(norm, func(r rune) bool {
		return r == ' ' || r == '_' || r == '\t'
	}), "-")
	switch ExecutionMode(norm) {
	case ExecutionImplement:
		return ExecutionImplement, true
	case ExecutionVerifyFirst:
		return ExecutionVerifyFirst, true
	case ExecutionDone, "complete", "completed", "already-done", "implemented":
		return ExecutionDone, true
	default:
		return "", false
	}
}
