package toolharness

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// AuditRecord is one durable, append-only record of a tool execution or a
// policy decision. It identifies the tool, the request that was made, and the
// outcome, so an operator can review what the model did without reading the
// model's own prose.
type AuditRecord struct {
	// Timestamp is when the decision or execution happened (UTC).
	Timestamp time.Time `json:"timestamp"`
	// Tool is the invoked tool name (for example "read_file").
	Tool string `json:"tool"`
	// Action is the decision the harness reached: "allow" or "deny".
	Action string `json:"action"`
	// Request summarizes the (path/command) requested. It never carries file
	// content, so the audit stays small and does not leak repository bodies.
	Request string `json:"request,omitempty"`
	// Outcome is "ok" or "error" for an executed tool, and "denied" for a
	// policy refusal.
	Outcome string `json:"outcome"`
	// Detail is a short human-readable note (an error message or policy reason).
	Detail string `json:"detail,omitempty"`
}

// Action and outcome values recorded in an AuditRecord.
const (
	ActionAllow = "allow"
	ActionDeny  = "deny"

	OutcomeOK     = "ok"
	OutcomeError  = "error"
	OutcomeDenied = "denied"
)

// Auditor receives an audit record for every tool invocation and policy
// decision. It must never return an error that stops work: auditing is
// observational, so a failure to record is reported on stderr by an adapter, not
// raised as a tool error. Implementations must be safe for concurrent use.
type Auditor interface {
	Record(AuditRecord)
}

// MultiAuditor fans a record out to several auditors. It lets a caller keep an
// in-memory trail for immediate review while also writing a durable one, without
// either sink knowing about the other.
type MultiAuditor struct {
	auditors []Auditor
}

// NewMultiAuditor returns an Auditor that records to every non-nil auditor in
// order.
func NewMultiAuditor(auditors ...Auditor) *MultiAuditor {
	kept := make([]Auditor, 0, len(auditors))
	for _, a := range auditors {
		if a != nil {
			kept = append(kept, a)
		}
	}
	return &MultiAuditor{auditors: kept}
}

// Record sends r to every contained auditor.
func (m *MultiAuditor) Record(r AuditRecord) {
	for _, a := range m.auditors {
		a.Record(r)
	}
}

// AuditLog is an in-memory, bounded, inspectable auditor. It is the default sink
// so a harness run is auditable without writing to disk or to SOP state.
type AuditLog struct {
	mu      sync.Mutex
	max     int
	records []AuditRecord
}

// NewAuditLog returns an empty audit log retaining at most max records (a
// non-positive max keeps all records).
func NewAuditLog(max int) *AuditLog {
	return &AuditLog{max: max}
}

// Record appends a record, dropping the oldest when the bound is reached.
func (l *AuditLog) Record(r AuditRecord) {
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, r)
	if l.max > 0 && len(l.records) > l.max {
		l.records = l.records[len(l.records)-l.max:]
	}
}

// Records returns a copy of the recorded events in order.
func (l *AuditLog) Records() []AuditRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AuditRecord, len(l.records))
	copy(out, l.records)
	return out
}

// FileAuditor appends records as JSON lines to a file. The path is chosen by the
// operator (for example through an environment variable) and must not be SOP's
// state database; inspecting the audit never touches .agent-sdlc/state.db.
type FileAuditor struct {
	path string
	mu   sync.Mutex
}

// NewFileAuditor returns a FileAuditor writing to path.
func NewFileAuditor(path string) *FileAuditor { return &FileAuditor{path: path} }

// Record appends r as a single JSON line. A write failure is swallowed (auditing
// is best-effort and must not fail a tool call), but the file handle is closed so
// a later record can retry.
func (a *FileAuditor) Record(r AuditRecord) {
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
