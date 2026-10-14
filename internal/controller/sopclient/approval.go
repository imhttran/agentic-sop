package sopclient

// Approval projects the SOP-reported approval gate for one task. Presence comes
// only from SOP's `sop approvals --json` listing (see approvals.go); BLOCKED
// status, prose, attempt counts, or inactivity never produce a gate. Fields are
// carried verbatim from the listing entry; Evidence is sanitized for display.
type Approval struct {
	// Present is true when SOP reports an applicable gate for this task. Key
	// absence checks off Present: Target and Stage are filled either way.
	Present     bool
	Kind        string
	Target      string
	Evidence    string // SOP's `reason`, display-only
	Stage       string
	Disposition string
	Status      string
	RequestedAt string
	TaskStatus  string
	// FinalGate is SOP's recorded final plan gate, or StatusUnknown.
	FinalGate         string
	FinalGateRecorded bool
}

func (s *Store) approval(task TaskDetail, listing ApprovalsListing) Approval {
	plan := s.Plan()
	a := Approval{Target: task.ID, Stage: task.Stage, FinalGate: plan.FinalGate, FinalGateRecorded: plan.Recorded}
	entry, present := listing.Lookup(task.ID)
	if !present {
		return a
	}
	a.Present = true
	a.Kind = entry.Kind
	if entry.Target != "" {
		a.Target = entry.Target
	}
	if entry.Stage != "" {
		a.Stage = entry.Stage
	}
	a.Evidence = entry.Reason
	a.Disposition = entry.Disposition
	a.Status = entry.Status
	a.RequestedAt = entry.RequestedAt
	a.TaskStatus = entry.TaskStatus
	return a
}

// taskNeedsHuman is the list/summary projection of the same listing entry
// Store.approval uses, so list and detail views cannot disagree.
func taskNeedsHuman(listing ApprovalsListing, taskID string) (bool, string) {
	entry, ok := listing.Lookup(taskID)
	if !ok {
		return false, ""
	}
	return true, entry.Kind
}
