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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/redact"
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
	// SourceDigests records the SHA-256 of each immutable source artifact at ingest
	// time, so the gate can tell a modified source from an unchanged one. The key is
	// the absolute artifact path as stored in SourceArtifacts. Mutable lifecycle state
	// (state.json) is not digested because SOP rewrites it after capture.
	SourceDigests map[string]string `json:"source_digests,omitempty"`
	RecordedAt    string            `json:"recorded_at,omitempty"`
	AccessScope   string            `json:"access_scope,omitempty"`

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
	return ReadAllContext(context.Background(), dir)
}

// ReadAllContext checks cancellation before listing and between documents.
func ReadAllContext(ctx context.Context, dir string) ([]OutcomeRecord, map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
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
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
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
	case r.Status == StatusSuccess && verificationContradictsSuccess(r.Verification):
		// A recorded check that failed contradicts the success claim, so the record is
		// not verified evidence.
		return false, "verification contradicts success"
	}
	return true, ""
}

// verificationContradictsSuccess reports whether any recorded verification failed. A
// verification with a non-zero exit code or a non-pass result contradicts a success.
func verificationContradictsSuccess(vs []Verification) bool {
	for _, v := range vs {
		if v.ExitCode != 0 {
			return true
		}
		if !passResult(v.Result) {
			return true
		}
	}
	return false
}

// passResult reports whether a recorded check result is a pass. An empty result is not a
// pass, so an unreadable check never corroborates a success.
func passResult(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PASS", "PASSED", "OK", "SUCCESS", "VERIFIED":
		return true
	}
	return false
}

// Gate applies the eligibility gate to records and returns one Exclusion for each
// excluded record, ordered by record ID. It returns no Exclusion when every record is
// eligible.
func Gate(records []OutcomeRecord) []Exclusion {
	return GateWithSources(records, nil)
}

// ReasonSourceDeleted is the exclusion reason for a record whose source artifact is gone.
const ReasonSourceDeleted = "source deleted"

// ReasonSourceModified is the exclusion reason for a record whose source artifact content
// no longer matches the digest recorded at ingest time.
const ReasonSourceModified = "source modified"

// ReasonSourceUnverified is the exclusion reason for a record that names an immutable
// source artifact with no recorded digest, so its content cannot be compared.
const ReasonSourceUnverified = "source unverified"

// GateWithSources applies the eligibility gate and additionally excludes any record whose
// recorded source artifacts no longer exist. A nil sourceExists skips the source check. A
// deleted, moved or access-restricted source therefore invalidates its record instead of
// leaving indexable evidence behind.
//
// When a record also carries recorded source digests, a source whose content changed is
// excluded as modified, so a changed source never keeps its obsolete verified identity.
func GateWithSources(records []OutcomeRecord, sourceExists func(string) bool) []Exclusion {
	return GateWithSourcesAndDigests(records, sourceExists, nil)
}

// GateWithSourcesAndDigests applies GateWithSources and additionally excludes a record
// whose source content no longer matches the digest recorded at ingest time, or whose
// immutable source artifact has no recorded digest at all. A nil sourceDigest skips the
// content checks, which keeps existence-only behaviour as the fallback when no digest
// comparison is requested. A record with no digest map at all is legacy and is not
// excluded here; the rebuild compatibility rule downgrades a digest-less success.
func GateWithSourcesAndDigests(records []OutcomeRecord, sourceExists func(string) bool, sourceDigest func(string) (string, error)) []Exclusion {
	var out []Exclusion
	for _, r := range records {
		ok, reason := r.Eligible()
		if ok && sourceExists != nil {
			for _, src := range r.SourceArtifacts {
				if strings.TrimSpace(src) == "" {
					continue
				}
				if !sourceExists(src) {
					ok, reason = false, ReasonSourceDeleted
					break
				}
			}
		}
		if !ok {
			out = append(out, Exclusion{RecordID: r.RecordID, Reason: reason})
			continue
		}
		if sourceDigest != nil && len(r.SourceDigests) > 0 {
			switch {
			case sourceContentModified(r, sourceDigest):
				out = append(out, Exclusion{RecordID: r.RecordID, Reason: ReasonSourceModified})
			case sourceArtifactUnverified(r):
				out = append(out, Exclusion{RecordID: r.RecordID, Reason: ReasonSourceUnverified})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out
}

// GateWithSourcesAndDigestsContext is GateWithSourcesAndDigests with a caller context.
// It checks the context before each record and each source artifact, and the supplied
// sourceDigest may itself observe the context (FileSourceDigestContext does). A bounded
// retrieval or capture deadline can therefore abort instead of walking an unbounded
// history. A nil context is treated as context.Background().
func GateWithSourcesAndDigestsContext(ctx context.Context, records []OutcomeRecord, sourceExists func(string) bool, sourceDigest func(string) (string, error)) ([]Exclusion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out []Exclusion
	for _, r := range records {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("outcomefile: gate: %w", err)
		}
		ok, reason := r.Eligible()
		if ok && sourceExists != nil {
			for _, src := range r.SourceArtifacts {
				if strings.TrimSpace(src) == "" {
					continue
				}
				if err := ctx.Err(); err != nil {
					return nil, fmt.Errorf("outcomefile: gate: %w", err)
				}
				if !sourceExists(src) {
					ok, reason = false, ReasonSourceDeleted
					break
				}
			}
		}
		if !ok {
			out = append(out, Exclusion{RecordID: r.RecordID, Reason: reason})
			continue
		}
		if sourceDigest != nil && len(r.SourceDigests) > 0 {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("outcomefile: gate: %w", err)
			}
			switch {
			case sourceContentModified(r, sourceDigest):
				// A canceled digest read is reported as a modification by
				// sourceContentModified; surface the cancellation instead of
				// treating the record as merely modified.
				if err := ctx.Err(); err != nil {
					return nil, fmt.Errorf("outcomefile: gate: %w", err)
				}
				out = append(out, Exclusion{RecordID: r.RecordID, Reason: ReasonSourceModified})
			case sourceArtifactUnverified(r):
				out = append(out, Exclusion{RecordID: r.RecordID, Reason: ReasonSourceUnverified})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out, nil
}

// sourceContentModified reports whether any recorded source digest disagrees with the
// current content of that artifact. A digest that cannot be recomputed is treated as a
// modification, so the record fails closed.
func sourceContentModified(r OutcomeRecord, sourceDigest func(string) (string, error)) bool {
	for src, want := range r.SourceDigests {
		if strings.TrimSpace(src) == "" || strings.TrimSpace(want) == "" {
			continue
		}
		if mutableLifecycleArtifact(src) {
			// SOP rewrites lifecycle state after capture (for example when parking a
			// run at the human boundary). It is not evidence, so it never invalidates
			// the record's provenance, including for records written before this rule.
			continue
		}
		got, err := sourceDigest(src)
		if err != nil || got != want {
			return true
		}
	}
	return false
}

// sourceArtifactUnverified reports whether an eligible record names an immutable source
// artifact with no recorded digest. It applies only to a record that already carries a
// digest map: a record with no digests at all is legacy and is handled by the rebuild
// compatibility rule, not by this check. An artifact that could not be read at ingest
// therefore cannot later escape the content check.
func sourceArtifactUnverified(r OutcomeRecord) bool {
	if len(r.SourceDigests) == 0 {
		return false
	}
	for _, src := range r.SourceArtifacts {
		if strings.TrimSpace(src) == "" || mutableLifecycleArtifact(src) {
			continue
		}
		if _, ok := r.SourceDigests[src]; !ok {
			return true
		}
	}
	return false
}

// FileSourceExists reports whether a source artifact path still exists. It is the
// existence predicate the gate uses on a local run tree.
func FileSourceExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileSourceDigest returns the SHA-256 of a source artifact's current content.
func FileSourceDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// FileSourceDigestContext returns the SHA-256 of a source artifact's current content
// while observing ctx. It reads in bounded chunks, so a canceled retrieval deadline
// stops a large hash instead of waiting for the entire file to be read.
func FileSourceDigestContext(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Redact removes credential-shaped substrings from free-form evidence. It is applied to
// every free-form field this package writes from ingested evidence.
//
// The shapes are decided by internal/redact; this package keeps its own typed markers,
// because they are written into persisted outcome documents. Redaction is deterministic
// and idempotent.
func Redact(s string) string {
	return redact.ShapesValues(s, evidenceMarker)
}

// evidenceMarker renders the marker this package has always persisted per kind.
func evidenceMarker(k redact.Kind) string {
	switch k {
	case redact.KindKey:
		return "<redacted-key>"
	case redact.KindToken:
		return "<redacted-token>"
	case redact.KindAccessKey:
		return "<redacted-access-key>"
	default:
		return "<redacted>"
	}
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
			Boundary    bool   `json:"Boundary"`
			Met         bool   `json:"Met"`
			Description string `json:"Description"`
			Evidence    string `json:"Evidence"`
		} `json:"criteria"`
		Validation struct {
			Passed *bool `json:"passed"`
		} `json:"validation"`
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
	// The report must claim the identity of the run directory it was read from. A report
	// substituted under another task's directory is rejected, so its content is never
	// ingested under a different identity.
	declaredID := strings.TrimSpace(rep.ID)
	if declaredID == "" {
		return OutcomeRecord{}, fmt.Errorf("%w: %s report.json has no id", ErrIncompleteRecord, taskID)
	}
	if declaredID != taskID {
		return OutcomeRecord{}, fmt.Errorf("%w: report id %q does not match run directory %q", ErrIncompleteRecord, declaredID, taskID)
	}

	status, ok := outcomeStatus(rep.Outcome.Status)
	if !ok {
		return OutcomeRecord{}, fmt.Errorf("%w: %s has unknown outcome status %q", ErrIncompleteRecord, taskID, rep.Outcome.Status)
	}
	// A self-declared success is corroborated from the report's own required criteria and
	// validation results. An uncorroborated claim stays usable as advisory data, but it is
	// never recorded as a verified success.
	if status == StatusSuccess && !successCorroborated(rep) {
		status = StatusPartial
	}

	// counts.json supplies the record attempt and revision. When present it is parsed
	// strictly: a malformed file is an explicit rejection, so identity never comes from
	// unverified metadata. A missing file contributes no metadata, so the attempt
	// defaults and the record stays usable as advisory data.
	var counts runCounts
	countsData, err := os.ReadFile(filepath.Join(runDir, "counts.json"))
	switch {
	case err == nil:
		if err := json.Unmarshal(countsData, &counts); err != nil {
			return OutcomeRecord{}, fmt.Errorf("outcomefile: ingest %s: decode counts.json: %w", taskID, err)
		}
	case os.IsNotExist(err):
		// No counts metadata; the attempt stays zero.
	default:
		return OutcomeRecord{}, fmt.Errorf("outcomefile: ingest %s: read counts.json: %w", taskID, err)
	}

	sum := sha256.Sum256(raw)
	rev := counts.Binary.Revision
	attempt := counts.Counts.Attempts
	if attempt < 0 {
		attempt = 0
	}

	artifacts := existingArtifacts(runDir)
	sourceArtifacts := absoluteArtifacts(artifacts)
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
		SourceArtifacts:  sourceArtifacts,
		SourceHash:       hex.EncodeToString(sum[:]),
		SourceDigests:    sourceDigests(sourceArtifacts),
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

// successCorroborated reports whether the report's own required criteria and validation
// results agree with a claimed success. At least one required criterion must be declared
// and every required criterion must be met, every boundary criterion must be met, and at
// least one validation result must be recorded and passing. An empty or optional-only
// criteria set cannot corroborate a success, and an unmet boundary criterion blocks it
// even when it is not required, because the outcome verifier treats that case as HOLD.
// Anything else leaves the claim uncorroborated.
func successCorroborated(rep runReport) bool {
	// The quality gate's verdict is separate from the command results. A report that
	// records a failed gate (for example, a blocking review finding) cannot corroborate
	// a success even when every command passed. An absent verdict stays compatible with
	// reports written before the field existed.
	if rep.Outcome.Validation.Passed != nil && !*rep.Outcome.Validation.Passed {
		return false
	}
	if decision := strings.ToUpper(strings.TrimSpace(rep.Decision)); decision != "" && decision != "PASS" {
		return false
	}
	if len(rep.Validation) == 0 {
		return false
	}
	for _, v := range rep.Validation {
		if v.ExitCode != 0 || !passResult(v.Status) {
			return false
		}
	}
	required := 0
	for _, c := range rep.Outcome.Criteria {
		if c.Boundary && !c.Met {
			return false
		}
		if !c.Required {
			continue
		}
		required++
		if !c.Met {
			return false
		}
	}
	return required > 0
}

// mutableLifecycleArtifacts names run files SOP rewrites after outcome capture. They
// carry lifecycle state, not evidence, so their content is not part of the provenance
// identity: hashing them would invalidate a valid record on an ordinary lifecycle
// update (for example parking a run at the human boundary).
var mutableLifecycleArtifacts = map[string]bool{
	"state.json": true,
}

// mutableLifecycleArtifact reports whether the artifact path names mutable lifecycle
// state rather than immutable evidence.
func mutableLifecycleArtifact(path string) bool {
	return mutableLifecycleArtifacts[filepath.Base(filepath.FromSlash(path))]
}

// sourceDigests records the SHA-256 of every named immutable evidence artifact that
// exists and can be read. An artifact that cannot be read is omitted, and the gate then
// excludes the record as unverified rather than falling back to an existence-only check.
// Mutable lifecycle state is skipped: an ordinary lifecycle update must not invalidate a
// record whose evidence is unchanged.
func sourceDigests(artifacts []string) map[string]string {
	out := make(map[string]string, len(artifacts))
	for _, a := range artifacts {
		if mutableLifecycleArtifact(a) {
			continue
		}
		h, err := FileSourceDigest(a)
		if err != nil {
			continue
		}
		out[a] = h
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	names := []string{"report.json", "report.md", "state.json", "counts.json", "trace.json", "rag.json"}
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

// absoluteArtifacts resolves each artifact path against the ingest process's working
// directory. A record persisted with a relative path would otherwise be resolved
// against whatever directory a later rebuild runs in, wrongly excluding an unchanged
// source as deleted. A path that cannot be made absolute is kept as-is.
func absoluteArtifacts(artifacts []string) []string {
	out := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		if abs, err := filepath.Abs(filepath.FromSlash(a)); err == nil {
			out = append(out, filepath.ToSlash(abs))
			continue
		}
		out = append(out, a)
	}
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
