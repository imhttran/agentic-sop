package sopclient

// Changed executed tasks awaiting reconcile come from SOP's listing,
// `sop reconcile <PLAN.md> --list-changed --json`, with the plan path taken from
// plan.meta.json. The listing is a pure read; the controller never diffs the
// plan or edits state.db. Title/Stage are enriched from SOP's task store.

import (
	"encoding/json"
	"errors"
	"strings"
)

const listingSource = "sop reconcile --list-changed --json"

// ErrTaskNotInChangedSet: the id is not in SOP's reported changed set.
var ErrTaskNotInChangedSet = errors.New("task is not in SOP's reported changed executed task set")

// ErrChangedTasksNotReported: SOP reported no changed set to accept from.
var ErrChangedTasksNotReported = errors.New("SOP reports no changed executed task set to approve")

// ErrNoExplicitTaskIDs: nothing was explicitly selected.
var ErrNoExplicitTaskIDs = errors.New("no explicitly selected task ids to accept")

// ErrNoActivePlan: SOP records no active plan, so there is nothing to reconcile.
var ErrNoActivePlan = errors.New("no active plan recorded by SOP")

// ChangedExecutedTask is one SOP-reported changed executed task.
type ChangedExecutedTask struct {
	TaskID string
	// Title and Stage come from SOP's task store; empty when unknown.
	Title string
	Stage string
	// ChangeSummary is SOP's description of the change, sanitized.
	ChangeSummary string
	// Approved mirrors SOP's recorded acceptance.
	Approved bool
}

// ChangedTasks is SOP's changed set. Reported=false means SOP reported none;
// never render that as an empty success.
type ChangedTasks struct {
	Reported bool
	// Source is the listing verb, for traceability.
	Source string
	Tasks  []ChangedExecutedTask
}

// Pending returns tasks not yet accepted; nil when SOP reported no set.
func (r ChangedTasks) Pending() []ChangedExecutedTask {
	if !r.Reported {
		return nil
	}
	var out []ChangedExecutedTask
	for _, t := range r.Tasks {
		if !t.Approved {
			out = append(out, t)
		}
	}
	return out
}

// Has reports membership; false when SOP reported no set.
func (r ChangedTasks) Has(taskID string) bool {
	for _, t := range r.Tasks {
		if t.TaskID == taskID {
			return true
		}
	}
	return false
}

// listingDoc is the `--list-changed --json` schema, decoded verbatim.
type listingDoc struct {
	Version         int      `json:"version"`
	Source          string   `json:"source"`
	PlanID          string   `json:"plan_id"`
	PlanChanged     bool     `json:"plan_changed"`
	Unchanged       []string `json:"unchanged"`
	Updated         []string `json:"updated"`
	Added           []string `json:"added"`
	Removed         []string `json:"removed"`
	ChangedExecuted []string `json:"changed_executed"`
	RemovedExecuted []string `json:"removed_executed"`
	AutoReconciled  []string `json:"auto_reconciled"`
}

// decodeListing returns ok=false for malformed or blank output (a real listing
// always carries the envelope).
func decodeListing(data []byte) (listingDoc, bool) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return listingDoc{}, false
	}
	var doc listingDoc
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return listingDoc{}, false
	}
	return doc, true
}

// changedTasksFromListing keeps the changed_executed ids in SOP's order; an id
// the task store doesn't know keeps an empty title.
func (s *Store) changedTasksFromListing(doc listingDoc) ChangedTasks {
	out := ChangedTasks{Reported: true, Source: listingSource}
	for _, rawID := range doc.ChangedExecuted {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		task := ChangedExecutedTask{TaskID: id}
		if title, ok := s.taskTitle(id); ok {
			task.Title = sanitizeDetail(title)
		}
		task.Stage = strings.TrimSpace(s.runInfo(id).Stage)
		out.Tasks = append(out.Tasks, task)
	}
	return out
}

// taskTitle returns ok=false when SOP has no row for the id.
func (s *Store) taskTitle(taskID string) (string, bool) {
	var title string
	err := s.db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, taskID).Scan(&title)
	if err != nil {
		return "", false
	}
	return title, true
}
