package sopclient

// SOP's approval listing (`sop approvals --json`) is the only source of gate
// presence and applicability. It feeds both Store.Tasks (NeedsHuman) and
// Store.Task (Approval), so the views cannot disagree. Field names match SOP's
// schema; reason/evidence are display-only.

import (
	"encoding/json"
	"strings"
)

// Approval status values, matched verbatim against SOP's structured fields.
const (
	ApprovalStatusPending   = "PENDING"
	ApprovalStatusApplied   = "APPLIED"
	ApprovalStatusCompleted = "COMPLETED"
	ApprovalStatusResolved  = "RESOLVED"
	ApprovalStatusStale     = "STALE"
)

// ApprovalEntry is one SOP-reported gate, decoded verbatim.
type ApprovalEntry struct {
	TaskID string
	Kind   string
	Target string
	// Reason and Evidence are display-only.
	Reason      string
	Evidence    string
	Stage       string
	Disposition string
	Status      string
	RequestedAt string
	TaskStatus  string
	// Applicable is derived only from Status/Disposition.
	Applicable bool
}

// ApprovalsListing is SOP's listing. Reported=false means SOP reported none;
// never render that as "no gates".
type ApprovalsListing struct {
	Reported bool
	// Entries are in SOP's order.
	Entries []ApprovalEntry
}

// approvalsDoc is SOP's listing schema.
type approvalsDoc struct {
	Approvals []approvalEntryDoc `json:"approvals"`
}

type approvalEntryDoc struct {
	TaskID      string `json:"task_id"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	Reason      string `json:"reason"`
	Evidence    string `json:"evidence"`
	Stage       string `json:"stage"`
	Disposition string `json:"disposition"`
	Status      string `json:"status"`
	RequestedAt string `json:"requested_at"`
	TaskStatus  string `json:"task_status"`
}

// decodeApprovals returns ok=false for malformed JSON. Blank output is a
// well-formed empty listing.
func decodeApprovals(data []byte) (ApprovalsListing, bool) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return ApprovalsListing{Reported: true}, true
	}
	var doc approvalsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return ApprovalsListing{}, false
	}
	out := ApprovalsListing{Reported: true}
	for _, e := range doc.Approvals {
		entry := ApprovalEntry{
			TaskID:      strings.TrimSpace(e.TaskID),
			Kind:        strings.TrimSpace(e.Kind),
			Target:      strings.TrimSpace(e.Target),
			Reason:      sanitizeDetail(e.Reason),
			Evidence:    sanitizeDetail(e.Evidence),
			Stage:       strings.TrimSpace(e.Stage),
			Disposition: strings.TrimSpace(e.Disposition),
			Status:      strings.TrimSpace(e.Status),
			RequestedAt: strings.TrimSpace(e.RequestedAt),
			TaskStatus:  strings.TrimSpace(e.TaskStatus),
		}
		entry.Applicable = applicableStatus(entry.Status, entry.Disposition)
		out.Entries = append(out.Entries, entry)
	}
	return out, true
}

// applicableStatus: SOP must affirmatively report the gate open; closed or
// blank never manufactures a gate.
func applicableStatus(status, disposition string) bool {
	if closedStatus(status) || closedStatus(disposition) {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case ApprovalStatusPending, "OPEN", "REQUESTED", "REQUESTED_APPROVAL":
		return true
	default:
		return false
	}
}

// closedStatus reports whether a value marks the gate answered or stale.
func closedStatus(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case ApprovalStatusApplied, ApprovalStatusCompleted, ApprovalStatusResolved, ApprovalStatusStale,
		"CLOSED", "DONE", "CANCELLED", "CANCELED":
		return true
	default:
		return false
	}
}

// Lookup returns the first applicable entry for taskID; resolved gates are never
// offered.
func (l ApprovalsListing) Lookup(taskID string) (ApprovalEntry, bool) {
	if !l.Reported {
		return ApprovalEntry{}, false
	}
	for _, e := range l.Entries {
		if e.TaskID != taskID {
			continue
		}
		if !e.Applicable {
			continue
		}
		return e, true
	}
	return ApprovalEntry{}, false
}
