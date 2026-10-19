// Package outcomefile writes and reads versioned outcome documents as authoritative
// local evidence for the memory index.
//
// A write is atomic: the document is written to a temporary file in the same directory,
// flushed, and renamed over the target path, so a reader never sees a partial document.
// Each document records its content hash. The reader rejects a document with an unknown
// schema version or a hash mismatch with an explicit error. The eligibility gate passes
// only complete, eligible records and reports a reason for each excluded record.
//
// An outcome record preserves the observed result of one task attempt: identity, context,
// decision, result, recovery, provenance and lifecycle. A generated rationale is marked
// as such and stays distinguishable from observed facts. Free-form evidence is redacted
// before it is written.
package outcomefile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// SchemaVersion is the document schema version this package writes and accepts.
const SchemaVersion = "outcomefile/2"

// DocumentExt is the file extension of an outcome document. A temporary file never
// carries this extension, so an interrupted write is never selected as an outcome file.
const DocumentExt = ".json"

// tempPrefix marks an in-progress temporary file.
const tempPrefix = ".outcome-"

// Status is the observed result of one task attempt. The four values stay distinct so a
// gate pass can never be read as a verified success.
type Status string

const (
	// StatusSuccess is a verified successful outcome.
	StatusSuccess Status = "success"
	// StatusFailed is a failed outcome.
	StatusFailed Status = "failed"
	// StatusPartial is a completed attempt with unmet or unverifiable criteria.
	StatusPartial Status = "partial"
	// StatusBlocked is an attempt stopped at a boundary.
	StatusBlocked Status = "blocked"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusPartial, StatusBlocked:
		return true
	}
	return false
}

// Rationale sources. A generated explanation never counts as an observed fact.
const (
	RationaleObserved  = "observed"
	RationaleGenerated = "generated"
	RationaleUnknown   = "unknown"
)

// Verification is one recorded check and its observed result.
type Verification struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Result   string `json:"result"`
}

// OutcomeRecord is the identity, scope, lifecycle and searchable content of one
// outcome. It is the document body, apart from the hash, which is computed over it.
type OutcomeRecord struct {
	SchemaVersion string                   `json:"schema_version"`
	RecordID      string                   `json:"record_id"`
	Corpus        retrievalboundary.Corpus `json:"corpus"`

	// Identity.
	Repository     string `json:"repository,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`

	// Context.
	TaskCategory       string            `json:"task_category,omitempty"`
	Environment        string            `json:"environment,omitempty"`
	DependencyVersions map[string]string `json:"dependency_versions,omitempty"`

	// Decision. RationaleSource records whether Rationale was observed or generated.
	Action          string `json:"action,omitempty"`
	Rationale       string `json:"rationale,omitempty"`
	RationaleSource string `json:"rationale_source,omitempty"`

	// Result.
	Status       Status         `json:"status,omitempty"`
	Verification []Verification `json:"verification,omitempty"`
	EvidenceRefs []string       `json:"evidence_refs,omitempty"`

	// Recovery.
	FailureSignature string `json:"failure_signature,omitempty"`
	Correction       string `json:"correction,omitempty"`
	Uncertainty      string `json:"uncertainty,omitempty"`

	// Provenance.
	SourceArtifacts []string `json:"source_artifacts,omitempty"`
	SourceHash      string   `json:"source_hash,omitempty"`
	RecordedAt      string   `json:"recorded_at,omitempty"`
	AccessScope     string   `json:"access_scope,omitempty"`

	// Lifecycle.
	Lifecycle    string `json:"lifecycle,omitempty"`
	Complete     bool   `json:"complete"`
	Superseded   bool   `json:"superseded"`
	SupersededBy string `json:"superseded_by,omitempty"`
	Authorized   bool   `json:"authorized"`

	// Citation and Text are the human citation and the searchable body.
	Citation string `json:"citation,omitempty"`
	Text     string `json:"text,omitempty"`

	// ContentHash is the hash of the document body. It is populated by Read and is not
	// part of the hashed body, so it is excluded from the body hash computation.
	ContentHash string `json:"-"`
}

// Document is the on-disk form: the body plus the hash of that body.
type Document struct {
	Body        OutcomeRecord `json:"body"`
	ContentHash string        `json:"content_hash"`
}

// ErrUnknownSchema is returned when a document declares a schema this package does not
// know. The document is rejected; it is never eligible.
var ErrUnknownSchema = errors.New("outcomefile: unknown schema version")

// ErrHashMismatch is returned when a document's recorded content hash does not match the
// hash of its body. The document is rejected; it is never eligible.
var ErrHashMismatch = errors.New("outcomefile: content hash mismatch")

// ErrIncompleteRecord is returned when a record is missing a required field.
var ErrIncompleteRecord = errors.New("outcomefile: incomplete record")

// Hash returns the content hash of a record body. It is deterministic: the same body
// always yields the same hash, so a repeated write has the same hash.
func Hash(r OutcomeRecord) (string, error) {
	norm := r
	norm.SchemaVersion = SchemaVersion
	data, err := json.Marshal(norm)
	if err != nil {
		return "", fmt.Errorf("outcomefile: hash: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// validateBody rejects a body that cannot be an outcome record.
func validateBody(r OutcomeRecord) error {
	if strings.TrimSpace(r.RecordID) == "" {
		return fmt.Errorf("%w: empty record id", ErrIncompleteRecord)
	}
	if !r.Corpus.Valid() {
		return fmt.Errorf("%w: invalid corpus %q", ErrIncompleteRecord, r.Corpus)
	}
	if !r.Status.Valid() {
		return fmt.Errorf("%w: invalid status %q", ErrIncompleteRecord, r.Status)
	}
	if r.RationaleSource != "" && r.RationaleSource != RationaleObserved &&
		r.RationaleSource != RationaleGenerated && r.RationaleSource != RationaleUnknown {
		return fmt.Errorf("%w: invalid rationale source %q", ErrIncompleteRecord, r.RationaleSource)
	}
	return nil
}

// PathFor returns the document path for a record ID under dir. The name is derived from
// the stable record ID, so writing the same record twice uses the same path.
func PathFor(dir, recordID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, recordID)
	if safe == "" {
		safe = "record"
	}
	return filepath.Join(dir, safe+DocumentExt)
}

// Write atomically writes r under dir and returns the path and the content hash. The
// document goes to a temporary file in the same directory, is flushed and synced, and is
// then renamed over the target. An interrupted write leaves the previous complete
// document readable and leaves only a non-document temporary file behind.
func Write(dir string, r OutcomeRecord) (string, string, error) {
	if err := validateBody(r); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("outcomefile: create dir: %w", err)
	}
	body := r
	body.SchemaVersion = SchemaVersion
	hash, err := Hash(body)
	if err != nil {
		return "", "", err
	}
	doc := Document{Body: body, ContentHash: hash}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("outcomefile: marshal: %w", err)
	}
	target := PathFor(dir, body.RecordID)
	tmp, err := os.CreateTemp(dir, tempPrefix+"*.tmp")
	if err != nil {
		return "", "", fmt.Errorf("outcomefile: temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return "", "", fmt.Errorf("outcomefile: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", "", fmt.Errorf("outcomefile: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", "", fmt.Errorf("outcomefile: close temp: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return "", "", fmt.Errorf("outcomefile: rename: %w", err)
	}
	// Sync the directory so the rename itself is durable. Some filesystems reject
	// directory sync; durability is best-effort there and the write is still atomic.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return target, hash, nil
}

// Read reads and validates one document at path. An unknown schema version or a hash
// mismatch is an explicit error.
func Read(path string) (OutcomeRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return OutcomeRecord{}, fmt.Errorf("outcomefile: read: %w", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return OutcomeRecord{}, fmt.Errorf("outcomefile: decode: %w", err)
	}
	if doc.Body.SchemaVersion != SchemaVersion {
		return OutcomeRecord{}, fmt.Errorf("%w: %q", ErrUnknownSchema, doc.Body.SchemaVersion)
	}
	hash, err := Hash(doc.Body)
	if err != nil {
		return OutcomeRecord{}, err
	}
	if hash != doc.ContentHash {
		return OutcomeRecord{}, fmt.Errorf("%w: want %s, got %s", ErrHashMismatch, hash, doc.ContentHash)
	}
	if err := validateBody(doc.Body); err != nil {
		return OutcomeRecord{}, err
	}
	rec := doc.Body
	rec.ContentHash = doc.ContentHash
	return rec, nil
}

// ReadAll reads every outcome document in dir. It returns the valid records and a map of
// rejected file names to explicit reasons. A temporary file or a file without the
// document extension is never selected.
func ReadAll(dir string) ([]OutcomeRecord, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]string{}, nil
		}
		return nil, nil, fmt.Errorf("outcomefile: list: %w", err)
	}
	rejected := map[string]string{}
	byID := map[string]OutcomeRecord{}
	fileForID := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, DocumentExt) || strings.HasPrefix(name, tempPrefix) {
			continue
		}
		rec, err := Read(filepath.Join(dir, name))
		if err != nil {
			rejected[name] = err.Error()
			continue
		}
		if prev, dup := fileForID[rec.RecordID]; dup {
			// Two documents claiming one stable identity are ambiguous evidence: keep the
			// first and reject the later file explicitly instead of silently overwriting.
			rejected[name] = fmt.Sprintf("duplicate record id %q already provided by %s", rec.RecordID, prev)
			continue
		}
		fileForID[rec.RecordID] = name
		byID[rec.RecordID] = rec
	}
	out := make([]OutcomeRecord, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out, rejected, nil
}

// Exclusion is one excluded record and the reason it was excluded.
type Exclusion struct {
	RecordID string `json:"record_id"`
	Reason   string `json:"reason"`
}

// Eligible reports whether the record passes the eligibility gate.
func (r OutcomeRecord) Eligible() (bool, string) {
	switch {
	case !r.Complete:
		return false, "incomplete"
	case !r.Status.Valid():
		return false, "missing status"
	case r.Superseded:
		return false, "superseded"
	case !r.Authorized:
		return false, "unauthorized"
	case strings.TrimSpace(r.ContentHash) == "":
		return false, "missing content hash"
	case r.Status == StatusSuccess && len(r.EvidenceRefs) == 0 && len(r.Verification) == 0:
		// A success claim without evidence is not a verified success.
		return false, "missing evidence for success"
	}
	return true, ""
}

// Gate applies the eligibility gate to records and returns one Exclusion for each
// excluded record, ordered by record ID. It returns no Exclusion when every record is
// eligible.
func Gate(records []OutcomeRecord) []Exclusion {
	return GateWithSources(records, nil)
}

// GateWithSources applies the eligibility gate and additionally excludes any record whose
// recorded source artifacts no longer exist. A nil sourceExists skips the source check. A
// deleted, moved or access-restricted source therefore invalidates its record instead of
// leaving indexable evidence behind.
func GateWithSources(records []OutcomeRecord, sourceExists func(string) bool) []Exclusion {
	var out []Exclusion
	for _, r := range records {
		ok, reason := r.Eligible()
		if ok && sourceExists != nil {
			for _, src := range r.SourceArtifacts {
				if strings.TrimSpace(src) == "" {
					continue
				}
				if !sourceExists(src) {
					ok, reason = false, "source deleted"
					break
				}
			}
		}
		if !ok {
			out = append(out, Exclusion{RecordID: r.RecordID, Reason: reason})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out
}

// Redaction patterns. They cover the credential shapes most likely to appear in captured
// evidence: provider API keys, GitHub tokens, AWS access keys, bearer tokens and
// key=value secrets. Redaction is deterministic and idempotent.
var redactionRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._\-+/=]{8,}`), `$1 <redacted>`},
	{regexp.MustCompile(`\b(sk|rk)-[A-Za-z0-9_\-]{12,}`), `<redacted-key>`},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{16,}`), `<redacted-token>`},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{16,}`), `<redacted-token>`},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), `<redacted-access-key>`},
	{regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|passwd)\s*[:=]\s*"?[^\s",}]{4,}"?`), `$1=<redacted>`},
}

// Redact removes credential-shaped substrings from free-form evidence. It is applied to
// every free-form field this package writes from ingested evidence.
func Redact(s string) string {
	for _, r := range redactionRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// Run report and counts shapes read by IngestRun. They are the persisted SOP run
// artifacts; only observed fields are consumed.
type runReport struct {
	ID         string   `json:"id"`
	Stage      string   `json:"stage"`
	Decision   string   `json:"decision"`
	Reasons    []string `json:"reasons"`
	Validation []struct {
		Category string `json:"Category"`
		Command  string `json:"Command"`
		ExitCode int    `json:"ExitCode"`
		Status   string `json:"Status"`
	} `json:"validation"`
	ModelSelection struct {
		Class    string `json:"class"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Locality string `json:"locality"`
		Source   string `json:"source"`
		Fallback bool   `json:"fallback"`
	} `json:"model_selection"`
	Outcome struct {
		Status   string `json:"status"`
		Criteria []struct {
			ID          string `json:"ID"`
			Required    bool   `json:"Required"`
			Met         bool   `json:"Met"`
			Description string `json:"Description"`
			Evidence    string `json:"Evidence"`
		} `json:"criteria"`
		Reasons       []string `json:"reasons"`
		Lifecycle     string   `json:"lifecycle"`
		Compatibility string   `json:"compatibility"`
	} `json:"outcome"`
	GeneratedAt string `json:"generated_at"`
}

type runCounts struct {
	Counts struct {
		Attempts    int `json:"attempts"`
		Invocations int `json:"invocations"`
		ModelCalls  int `json:"model_calls"`
		Approvals   int `json:"approvals"`
	} `json:"counts"`
	Binary struct {
		Revision string `json:"revision"`
		Modified string `json:"modified"`
	} `json:"binary"`
}

// outcomeStatus maps a SOP outcome verdict to the record status. The mapping is explicit
// so an unknown verdict never becomes a success.
func outcomeStatus(s string) (Status, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "VERIFIED", "PASS", "PASSED", "SUCCESS":
		return StatusSuccess, true
	case "PARTIAL":
		return StatusPartial, true
	case "FAIL", "FAILED", "FAILURE":
		return StatusFailed, true
	case "BLOCKED", "NEEDS_HUMAN", "HOLD":
		return StatusBlocked, true
	}
	return "", false
}

// IngestRun builds an outcome record from the persisted artifacts of one task attempt.
// The record is derived from observed evidence only; the recorded verdict is preserved
// verbatim, so a run that passed its gate but not its acceptance criteria is partial, not
// a success. Re-ingesting the same artifacts produces the same record ID and content
// hash, so the write is idempotent.
func IngestRun(runsDir, taskID string) (OutcomeRecord, error) {
	return IngestRunFor(runsDir, taskID, "")
}

// IngestRunFor is IngestRun with an explicit repository identity. Advisory retrieval is
// repository-scoped, so a record without one can never be returned.
func IngestRunFor(runsDir, taskID, repository string) (OutcomeRecord, error) {
	if strings.TrimSpace(taskID) == "" {
		return OutcomeRecord{}, fmt.Errorf("%w: empty task id", ErrIncompleteRecord)
	}
	runDir := filepath.Join(runsDir, taskID)
	reportPath := filepath.Join(runDir, "report.json")
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return OutcomeRecord{}, fmt.Errorf("outcomefile: ingest %s: %w", taskID, err)
	}
	var rep runReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		return OutcomeRecord{}, fmt.Errorf("outcomefile: ingest %s: decode report: %w", taskID, err)
	}
	status, ok := outcomeStatus(rep.Outcome.Status)
	if !ok {
		return OutcomeRecord{}, fmt.Errorf("%w: %s has unknown outcome status %q", ErrIncompleteRecord, taskID, rep.Outcome.Status)
	}

	var counts runCounts
	if data, err := os.ReadFile(filepath.Join(runDir, "counts.json")); err == nil {
		_ = json.Unmarshal(data, &counts)
	}

	sum := sha256.Sum256(raw)
	rev := counts.Binary.Revision
	attempt := counts.Counts.Attempts
	if attempt < 0 {
		attempt = 0
	}

	runID := fmt.Sprintf("%s-attempt-%d", taskID, attempt)
	rec := OutcomeRecord{
		SchemaVersion:    SchemaVersion,
		RecordID:         runID,
		Corpus:           retrievalboundary.CorpusOutcome,
		RunID:            runID,
		TaskID:           taskID,
		Repository:       strings.TrimSpace(repository),
		Attempt:          attempt,
		SourceRevision:   rev,
		Environment:      strings.TrimSpace(fmt.Sprintf("provider=%s model=%s locality=%s", rep.ModelSelection.Provider, rep.ModelSelection.Model, rep.ModelSelection.Locality)),
		Action:           rep.Decision,
		Rationale:        Redact(strings.Join(append(rep.Reasons, rep.Outcome.Reasons...), "; ")),
		RationaleSource:  RationaleObserved,
		Status:           status,
		Verification:     verificationFromReport(rep),
		EvidenceRefs:     existingArtifacts(runDir),
		FailureSignature: Redact(firstNonEmpty(rep.Outcome.Reasons)),
		Uncertainty:      Redact(rep.Outcome.Compatibility),
		SourceArtifacts:  existingArtifacts(runDir),
		SourceHash:       hex.EncodeToString(sum[:]),
		RecordedAt:       rep.GeneratedAt,
		AccessScope:      "local",
		Lifecycle:        rep.Outcome.Lifecycle,
		Complete:         true,
		Authorized:       true,
		Citation:         filepath.ToSlash(reportPath),
	}
	rec.Text = Redact(searchableText(rec, rep))
	// Populate the content hash here so an ingested record is immediately a complete,
	// eligible document; Write recomputes the same hash from the same body.
	hash, err := Hash(rec)
	if err != nil {
		return OutcomeRecord{}, err
	}
	rec.ContentHash = hash
	return rec, nil
}

// verificationFromReport records the observed validation commands without their stdout,
// which may contain sensitive output.
func verificationFromReport(rep runReport) []Verification {
	out := make([]Verification, 0, len(rep.Validation))
	for _, v := range rep.Validation {
		out = append(out, Verification{Command: v.Command, ExitCode: v.ExitCode, Result: v.Status})
	}
	return out
}

// existingArtifacts lists the run files that exist, so the record's provenance never
// names a file that was not captured.
func existingArtifacts(runDir string) []string {
	names := []string{"report.json", "report.md", "state.json", "counts.json", "trace.json"}
	var out []string
	for _, n := range names {
		p := filepath.Join(runDir, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, filepath.ToSlash(p))
		}
	}
	sort.Strings(out)
	return out
}

// searchableText builds the FTS body from identity, rationale, criteria and recovery
// evidence. It never includes raw stdout or full model responses.
func searchableText(rec OutcomeRecord, rep runReport) string {
	var b strings.Builder
	b.WriteString(rec.TaskID)
	for _, c := range rep.Outcome.Criteria {
		b.WriteString(" ")
		b.WriteString(c.ID)
		b.WriteString(": ")
		b.WriteString(c.Description)
		b.WriteString(" (")
		b.WriteString(c.Evidence)
		b.WriteString(")")
	}
	if rec.Rationale != "" {
		b.WriteString(" ")
		b.WriteString(rec.Rationale)
	}
	if rec.FailureSignature != "" {
		b.WriteString(" ")
		b.WriteString(rec.FailureSignature)
	}
	if rec.Uncertainty != "" {
		b.WriteString(" ")
		b.WriteString(rec.Uncertainty)
	}
	return b.String()
}

func firstNonEmpty(values []string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
