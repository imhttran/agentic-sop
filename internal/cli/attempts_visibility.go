package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// Execution-attempt visibility (Phase 5 §18–§19).
//
// `sop report` distinguishes the initial routing decision from the attempts SOP
// actually made, and summarises the bounded-escalation metrics SOP already records
// in the attempt records. Nothing here is a source of truth: the attempt records
// are the persisted evidence, and no display value is ever fed back to a decision.
//
// Nothing is rendered when no attempt records exist (escalation OFF, or an
// unrouted task), so an existing report is unchanged.

// classTransition is one upward class change between consecutive attempts, used for
// the escalation metrics.
type classTransition struct {
	from, to model.Class
}

// writeAttemptsReport renders the task's execution attempts and the escalation
// metrics derived from them. It reads the run directory's persisted records; a
// missing directory renders nothing.
func writeAttemptsReport(w io.Writer, runDir string) {
	attempts := runpkg.ReadAttemptRecordsAt(runDir)
	if len(attempts) == 0 {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Execution attempts:")
	if initial := initialAttemptClass(attempts); initial != "" {
		fmt.Fprintf(w, "  %-14s %s\n", "Initial class:", classText(initial))
	}
	fmt.Fprintln(w)

	for _, a := range attempts {
		result := strings.ToUpper(string(a.Result))
		if a.FailureStage != "" && a.Result == runpkg.AttemptFailed {
			result += fmt.Sprintf(" (%s)", a.FailureStage)
		}
		fmt.Fprintf(w, "  #%d  %-6s %-28s %s\n", a.Attempt, classText(model.Class(a.Class)), a.Model, result)
		if a.Reason != "" {
			fmt.Fprintf(w, "        reason: %s\n", a.Reason)
		}
		if a.Action != "" && a.Action != "none" {
			fmt.Fprintf(w, "        recovery: %s\n", a.Action)
		}
	}

	writeAttemptMetrics(w, attempts)
}

// writeAttemptMetrics renders the observational escalation metrics SOP derives
// from the attempt records. They are diagnostic only: nothing reads them back to
// change routing or recovery policy.
func writeAttemptMetrics(w io.Writer, attempts []runpkg.AttemptRecord) {
	started := map[model.Class]int{}
	escalations := map[classTransition]int{}
	passed, failed := 0, 0
	var prev model.Class
	for i, a := range attempts {
		class := model.Class(a.Class)
		if class != "" {
			started[class]++
		}
		if a.Result == runpkg.AttemptPassed {
			passed++
		} else {
			failed++
		}
		if i > 0 && class != "" && prev != "" && classRank(class) > classRank(prev) {
			escalations[classTransition{from: prev, to: class}]++
		}
		prev = class
	}

	total := 0
	for _, n := range escalations {
		total += n
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Routing metrics:")
	fmt.Fprintf(w, "  attempts: %d · escalations: %d · passed: %d · failed: %d\n",
		len(attempts), total, passed, failed)
	fmt.Fprintf(w, "  started: small=%d medium=%d large=%d\n",
		started[model.ClassSmall], started[model.ClassMedium], started[model.ClassLarge])
	for _, t := range []classTransition{
		{model.ClassSmall, model.ClassMedium},
		{model.ClassMedium, model.ClassLarge},
	} {
		if n := escalations[t]; n > 0 {
			fmt.Fprintf(w, "  escalated %s→%s: %d\n", classText(t.from), classText(t.to), n)
		}
	}
	if len(attempts) > 1 && attempts[len(attempts)-1].Result == runpkg.AttemptPassed {
		fmt.Fprintln(w, "  success after escalation: yes")
	}
}

// initialAttemptClass returns the class of the first attempt, which is the class
// routing (or an operator override, or the built-in default) initially selected.
func initialAttemptClass(attempts []runpkg.AttemptRecord) model.Class {
	for _, a := range attempts {
		if a.Attempt == 1 && a.Class != "" {
			return model.Class(a.Class)
		}
	}
	return ""
}

// classRank orders the model classes for the escalation-metric comparison.
func classRank(c model.Class) int {
	switch c {
	case model.ClassSmall:
		return 1
	case model.ClassMedium:
		return 2
	case model.ClassLarge:
		return 3
	default:
		return 0
	}
}
