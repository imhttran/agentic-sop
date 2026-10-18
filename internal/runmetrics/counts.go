package runmetrics

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/durablefs"
	"github.com/imhttran/agentic-sop/internal/perf"
)

// UNKNOWN is the count value for a kind with neither a recorded count nor a
// historical source artifact. It is distinct from a recorded zero.
const UNKNOWN = "UNKNOWN"

// CountKind is one separately counted event kind. No kind is ever folded into
// another.
type CountKind string

const (
	CountInvocations         CountKind = "invocations"
	CountAttempts            CountKind = "attempts"
	CountContinuations       CountKind = "continuations"
	CountModelCalls          CountKind = "model_calls"
	CountApprovals           CountKind = "approvals"
	CountDirectInterventions CountKind = "direct_interventions"
)

// CountKinds lists every kind in report order.
var CountKinds = []CountKind{CountInvocations, CountAttempts, CountContinuations, CountModelCalls, CountApprovals, CountDirectInterventions}

// countsFileName is the restart-durable counts artifact under the run directory.
const countsFileName = "counts.json"

// approvalHistoryFileName mirrors internal/run's append-only approval history.
const approvalHistoryFileName = "approval-history.json"

// countsDoc is the persisted counts.json: the recorded total per kind and the
// report ids already applied, so a repeated report is never counted twice.
type countsDoc struct {
	Counts  map[CountKind]int `json:"counts"`
	Reports []string          `json:"reports"`
	// Binary is the provenance of the candidate binary that recorded the counts.
	Binary *Provenance `json:"binary,omitempty"`
}

// countsMu serializes the read-modify-write of counts.json within one process.
// Cross-process writers are not expected for a task's run directory (SOP drives
// one task lifecycle at a time); the atomic durable write still prevents a
// truncated artifact.
var countsMu sync.Mutex

// Count is one kind's value; Known is false when no history exists for it.
type Count struct {
	Value int
	Known bool
}

// String renders the count, or UNKNOWN when there is no history.
func (c Count) String() string {
	if !c.Known {
		return UNKNOWN
	}
	return strconv.Itoa(c.Value)
}

// LoadCounts returns every kind's count for a run directory. A kind recorded in
// counts.json uses that value; otherwise it is seeded from the existing artifact
// for that kind (attempts/, continuations.txt, approval-history.json, metrics.json);
// a kind with neither is UNKNOWN. Invocations and direct interventions have no
// historical source, so they are known only once recorded.
func LoadCounts(runDir string) map[CountKind]Count {
	doc := readCounts(runDir)
	out := make(map[CountKind]Count, len(CountKinds))
	for _, k := range CountKinds {
		if v, ok := doc.Counts[k]; ok {
			out[k] = Count{Value: v, Known: true}
			continue
		}
		v, ok := seedCount(runDir, k)
		out[k] = Count{Value: v, Known: ok}
	}
	return out
}

// RecordReport applies one report's per-kind TOTALS exactly once, keyed by
// reportID, and persists counts.json atomically. The values are absolute totals,
// not increments: a seed read from an existing artifact (attempts/, continuations,
// metrics, approval history) is never added to a reported count, which is what
// previously double-counted the current attempt. It returns false without writing
// when the report was already applied, including by an earlier process.
func RecordReport(runDir, reportID string, totals map[CountKind]int) (bool, error) {
	if reportID == "" {
		return false, fmt.Errorf("runmetrics: empty report id")
	}
	countsMu.Lock()
	defer countsMu.Unlock()

	doc := readCounts(runDir)
	if slices.Contains(doc.Reports, reportID) {
		return false, nil
	}
	for k, n := range totals {
		if !slices.Contains(CountKinds, k) {
			return false, fmt.Errorf("runmetrics: unknown count kind %q", k)
		}
		if n < 0 {
			return false, fmt.Errorf("runmetrics: count %q cannot be negative", k)
		}
		doc.Counts[k] = n
	}
	doc.Reports = append(doc.Reports, reportID)
	if err := writeCounts(runDir, doc); err != nil {
		return false, err
	}
	return true, nil
}

// RecordBinaryProvenance records the candidate binary's path, checksum, and
// embedded source identity once per run directory. A previously recorded
// provenance is preserved (the first binary that produced the run's evidence is
// the one the evidence belongs to); the existing value is returned with stored=true.
func RecordBinaryProvenance(runDir, path string) (Provenance, bool, error) {
	p, err := BinaryProvenance(path)
	if err != nil {
		return Provenance{}, false, err
	}
	countsMu.Lock()
	defer countsMu.Unlock()

	doc := readCounts(runDir)
	if doc.Binary != nil {
		return *doc.Binary, true, nil
	}
	doc.Binary = &p
	if err := writeCounts(runDir, doc); err != nil {
		return Provenance{}, false, err
	}
	return p, false, nil
}

// LoadBinaryProvenance returns the recorded candidate-binary provenance, if any.
func LoadBinaryProvenance(runDir string) (Provenance, bool) {
	doc := readCounts(runDir)
	if doc.Binary == nil {
		return Provenance{}, false
	}
	return *doc.Binary, true
}

func writeCounts(runDir string, doc countsDoc) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return durablefs.WriteFile(filepath.Join(runDir, countsFileName), append(data, '\n'))
}

func readCounts(runDir string) countsDoc {
	var doc countsDoc
	readJSON(filepath.Join(runDir, countsFileName), &doc)
	if doc.Counts == nil {
		doc.Counts = map[CountKind]int{}
	}
	return doc
}

// seedCount reads a kind's historical value from its existing source artifact.
func seedCount(runDir string, k CountKind) (int, bool) {
	switch k {
	case CountAttempts:
		entries, err := os.ReadDir(filepath.Join(runDir, attemptsDirName))
		if err != nil {
			return 0, false
		}
		return len(entries), true
	case CountContinuations:
		return readInt(filepath.Join(runDir, continuationsFileName))
	case CountApprovals:
		var history []domain.ApprovalRequest
		if !readJSON(filepath.Join(runDir, approvalHistoryFileName), &history) {
			return 0, false
		}
		// Approvals are decisions, not requests: a pending request is not an
		// approval. Count only requests a human actually approved.
		n := 0
		for _, req := range history {
			if req.Status == domain.ApprovalApproved {
				n++
			}
		}
		return n, true
	case CountModelCalls:
		var task perf.Task
		if !readJSON(filepath.Join(runDir, metricsFileName), &task) {
			return 0, false
		}
		return task.Counts.AgentCalls + task.Counts.ReviewRuns, true
	}
	return 0, false
}

// Provenance identifies a candidate binary: its path, SHA-256 checksum, and the
// source identity embedded in its Go build info. Absent identity fields are UNKNOWN.
type Provenance struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Module   string `json:"module"`
	Revision string `json:"revision"`
	Modified string `json:"modified"`
}

// BinaryProvenance reads a binary's checksum and embedded source identity. A
// missing or unreadable binary is an error, never a record with an empty checksum.
func BinaryProvenance(path string) (Provenance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Provenance{}, fmt.Errorf("runmetrics: read candidate binary: %w", err)
	}
	sum := sha256.Sum256(data)
	p := Provenance{Path: path, SHA256: hex.EncodeToString(sum[:]), Module: UNKNOWN, Revision: UNKNOWN, Modified: UNKNOWN}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return p, nil // not a Go binary: the source identity is unknown, never guessed
	}
	if info.Main.Path != "" {
		p.Module = info.Main.Path
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			p.Revision = s.Value
		case "vcs.modified":
			p.Modified = s.Value
		}
	}
	return p, nil
}
