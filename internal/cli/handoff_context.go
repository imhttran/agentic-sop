package cli

import (
	"strings"
	"time"

	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/handoff"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// handoffReader and handoffWriter are the narrow slice of the store the handoff
// wiring needs. The concrete store implements both; a store (or a test fake) that
// does not is simply skipped, so an unwired process keeps its behavior. This is
// wiring, not a new lifecycle component.
type handoffReader interface {
	Handoffs() ([]handoff.Record, error)
}
type handoffWriter interface {
	SaveHandoff(handoff.Record) error
}

// dependencyHandoffItems builds lowest-priority context items from the handoff
// capsules of a task's direct dependencies (RBV-011). It reuses handoff.Select, so a
// task never receives the full history of prior work — only its own dependencies'
// capsules. It returns nil when the store cannot provide handoffs.
func dependencyHandoffItems(task *domain.Task, saver taskSaver) []sopctx.Item {
	hr, ok := saver.(handoffReader)
	if !ok {
		return nil
	}
	records, err := hr.Handoffs()
	if err != nil {
		return nil
	}
	var items []sopctx.Item
	for _, rec := range handoff.Select(task, records) {
		text := handoffText(rec)
		if text == "" {
			continue
		}
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceMemory,
			Identity: "handoff-" + rec.TaskID,
			Reason:   "handoff capsule from completed dependency " + rec.TaskID,
			Priority: sopctx.PriorityGeneric,
			Text:     text,
		})
	}
	return items
}

// handoffText renders a capsule into a compact, bounded context item: the compressed
// content when present, otherwise the summary and files. It never returns workflow
// truth — a capsule is a derived summary.
func handoffText(rec handoff.Record) string {
	if c := strings.TrimSpace(rec.Content); c != "" {
		return c
	}
	var b strings.Builder
	if s := strings.TrimSpace(rec.Capsule.Summary); s != "" {
		b.WriteString(s)
	}
	if files := rec.Capsule.Files; len(files) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Files: " + strings.Join(files, ", "))
	}
	return strings.TrimSpace(b.String())
}

// saveTaskHandoff persists a completed task's capsule so a dependent task's context
// can be compressed between tasks (RBV-011). It is best-effort: an unwired store or a
// builder error simply leaves no record, and it never changes task state.
func saveTaskHandoff(saver taskSaver, task *domain.Task, rn *runpkg.Run, res lifeResult) {
	hw, ok := saver.(handoffWriter)
	if !ok {
		return
	}
	capsule, err := handoff.NewBuilder().Build(task, taskHandoffFacts(rn, res))
	if err != nil {
		return
	}
	_ = hw.SaveHandoff(handoff.Record{
		TaskID:    task.ID,
		Capsule:   capsule,
		Status:    handoff.StatusNotRequested,
		CreatedAt: time.Now().UTC(),
	})
}

// taskHandoffFacts derives a capsule's explicit facts from the completed run's
// evidence. It invents nothing: only the task's own changed files, the observed
// validation results, and the gate/outcome reasons.
func taskHandoffFacts(rn *runpkg.Run, res lifeResult) handoff.Facts {
	var files []string
	if rn != nil {
		files = rn.ChangedFiles()
	}
	var verification []handoff.VerificationSummary
	for _, r := range res.suite.Results {
		verification = append(verification, handoff.VerificationSummary{
			Check:  string(r.Category) + ": " + r.Command,
			Status: string(r.Status),
		})
	}
	var decisions []string
	if res.outcome != nil {
		decisions = append(decisions, res.outcome.Reasons...)
	}
	facts := handoff.Facts{
		Summary:      firstReason(res.gate),
		Files:        files,
		Verification: verification,
		Decisions:    decisions,
	}
	if len(res.report.Findings) > 0 {
		facts.CarryForward = []string{"review findings recorded"}
	}
	return facts
}
