package outcome

import (
	"fmt"
	"strings"
)

// WaiverRecord is one designated-approver grant of a criterion-scoped waiver,
// recorded as durable, append-only audit evidence (SOP-HARD-04). Its field set is
// the pinned waiver schema: task, criterion_id, scope, reason, approver,
// recorded_at, and the waived outcome.
type WaiverRecord struct {
	Task        string `json:"task"`
	CriterionID string `json:"criterion_id"`
	Scope       string `json:"scope,omitempty"`
	Reason      string `json:"reason"`
	Approver    string `json:"approver"`
	RecordedAt  string `json:"recorded_at"`
	Outcome     Status `json:"outcome"`
}

// GrantRequest is a proposed waiver grant before it is validated or recorded. It is
// operator-owned: it is never taken from a model response.
type GrantRequest struct {
	Task        string
	CriterionID string
	Scope       string
	Reason      string
	Approver    string
}

// ValidateGrant fails closed when a proposed waiver grant is not well-formed or the
// named approver is not a designated approver. It never mutates state and never
// consults a model: only an explicit, designated, reasoned, timestamped grant is
// accepted, so a waiver is never implied and never silently applied.
func ValidateGrant(req GrantRequest, designated []string, recordedAt string) error {
	switch {
	case req.Task == "":
		return fmt.Errorf("outcome: a waiver must name the task it covers")
	case req.CriterionID == "":
		return fmt.Errorf("outcome: a waiver must name the criterion it covers")
	case req.Approver == "":
		return fmt.Errorf("outcome: waiver requires a designated approver; %w", ErrNoDesignatedApprover)
	case !IsDesignatedApprover(req.Approver, designated):
		return fmt.Errorf("outcome: %q is not a designated approver; %w", req.Approver, ErrNoDesignatedApprover)
	case req.Reason == "":
		return fmt.Errorf("outcome: waiver requires a recorded reason")
	case recordedAt == "":
		return fmt.Errorf("outcome: waiver requires a recording timestamp")
	}
	return nil
}

// IsDesignatedApprover reports whether name is one of the operator-designated
// approvers authorized to grant a waiver. Matching is exact (after trimming
// surrounding space) and case-sensitive; an empty list designates no one, so the
// waiver path fails closed by default.
func IsDesignatedApprover(name string, designated []string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, a := range designated {
		if strings.TrimSpace(a) == name {
			return true
		}
	}
	return false
}

// NewGrant builds a WaiverRecord from an already-validated request (see
// ValidateGrant) and stamps the waived outcome. It assembles only; it writes nothing.
func NewGrant(req GrantRequest, waived Status, recordedAt string) WaiverRecord {
	return WaiverRecord{
		Task:        req.Task,
		CriterionID: req.CriterionID,
		Scope:       req.Scope,
		Reason:      req.Reason,
		Approver:    req.Approver,
		RecordedAt:  recordedAt,
		Outcome:     waived,
	}
}

// WaiverFromRecords combines the append-only audit records for one task into a
// single criterion-scoped Waiver: the union of every covered criterion, with the
// attribution, reason, and timestamp of the most recent record. It returns nil when
// there are no records, so the gate's waiver is absent (and fails closed) until a
// designated approver records one.
func WaiverFromRecords(recs []WaiverRecord) *Waiver {
	if len(recs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, r := range recs {
		if r.CriterionID == "" || seen[r.CriterionID] {
			continue
		}
		seen[r.CriterionID] = true
		ids = append(ids, r.CriterionID)
	}
	if len(ids) == 0 {
		return nil
	}
	last := recs[len(recs)-1]
	return &Waiver{
		Approver:   last.Approver,
		Criteria:   ids,
		Reason:     last.Reason,
		RecordedAt: last.RecordedAt,
	}
}
