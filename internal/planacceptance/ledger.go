package planacceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/projectlock"
)

const (
	decisionsFile   = "decisions.jsonl"
	annotationsFile = "annotations.jsonl"
	checksFile      = "checks.jsonl"
	checkLogsDir    = "checks"
)

// CorruptError reports a ledger that cannot be read safely. The caller must not
// append after a corrupt record and must not fall back to older records.
type CorruptError struct {
	Path string
	Line int
	Err  error
}

func (e *CorruptError) Error() string {
	return fmt.Sprintf("ledger %s line %d: %v", e.Path, e.Line, e.Err)
}

func (e *CorruptError) Unwrap() error { return e.Err }

// Annotation is an auditable addendum to one archived manual-evidence mapping.
// It preserves the archived text and hash; it never replaces or weakens the
// requirement. A decision may bind to one annotation by its content hash.
type Annotation struct {
	Kind                   string                `json:"kind"`
	ID                     string                `json:"id"`
	PlanIdentity           string                `json:"plan_identity"`
	CriterionID            string                `json:"criterion_id"`
	ArchivedManualEvidence string                `json:"archived_manual_evidence"`
	ArchivedEvidenceSHA256 string                `json:"archived_evidence_sha256"`
	ReviewedEvidence       string                `json:"reviewed_evidence"`
	ReviewedEvidenceSHA256 string                `json:"reviewed_evidence_sha256"`
	Reason                 string                `json:"reason"`
	EvidenceRefs           []outcome.EvidenceRef `json:"evidence_refs"`
	Approver               string                `json:"approver"`
	RecordedAt             string                `json:"recorded_at"`
}

// CheckEvidence is one executed archived acceptance check bound to the plan
// identity and the exact workspace revision it tested.
type CheckEvidence struct {
	Kind            string   `json:"kind"`
	PlanIdentity    string   `json:"plan_identity"`
	CriterionID     string   `json:"criterion_id"`
	CheckRef        string   `json:"check_ref"`
	Command         string   `json:"command"`
	Argv            []string `json:"argv"`
	Class           string   `json:"class"`
	Executed        bool     `json:"executed"`
	Passed          bool     `json:"passed"`
	ExitCode        int      `json:"exit_code"`
	Output          string   `json:"output"`
	OutputSHA256    string   `json:"output_sha256"`
	LogPath         string   `json:"log_path,omitempty"`
	LogSHA256       string   `json:"log_sha256,omitempty"`
	TestedRevision  string   `json:"tested_revision"`
	TestedDirty     bool     `json:"tested_dirty"`
	SourceUnchanged bool     `json:"source_unchanged"`
	RecordedAt      string   `json:"recorded_at"`
	RecordedBy      string   `json:"recorded_by"`
}

func decisionsPath(ctx *Context) string   { return filepath.Join(ctx.LedgerDir, decisionsFile) }
func annotationsPath(ctx *Context) string { return filepath.Join(ctx.LedgerDir, annotationsFile) }
func checksPath(ctx *Context) string      { return filepath.Join(ctx.LedgerDir, checksFile) }

// ReadDecisions returns the plan-scoped decisions, failing closed on any corrupt
// or unterminated record.
func ReadDecisions(ctx *Context) ([]outcome.AcceptanceDecision, error) {
	return readStrictJSONL[outcome.AcceptanceDecision](decisionsPath(ctx))
}

// ReadAnnotations returns the plan-scoped annotations, failing closed on any
// corrupt or unterminated record.
func ReadAnnotations(ctx *Context) ([]Annotation, error) {
	return readStrictJSONL[Annotation](annotationsPath(ctx))
}

// ReadChecks returns the plan-scoped check evidence, failing closed on any corrupt
// or unterminated record.
func ReadChecks(ctx *Context) ([]CheckEvidence, error) {
	return readStrictJSONL[CheckEvidence](checksPath(ctx))
}

func readStrictJSONL[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, &CorruptError{Path: path, Line: countLines(data), Err: errors.New("unterminated final record")}
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	records := make([]T, 0, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			return nil, &CorruptError{Path: path, Line: i + 1, Err: errors.New("blank record")}
		}
		var rec T
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, &CorruptError{Path: path, Line: i + 1, Err: err}
		}
		records = append(records, rec)
	}
	return records, nil
}

func countLines(data []byte) int {
	return strings.Count(string(data), "\n") + 1
}

func ensureAppendable(path string) error {
	if _, err := readStrictJSONL[json.RawMessage](path); err != nil {
		return err
	}
	return nil
}

func appendRecord(path string, rec any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ensureAppendable(path); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func withProjectLock(projectDir string, fn func() error) error {
	lock, err := projectlock.AcquireWait(context.Background(), projectDir, 0)
	if err != nil {
		return fmt.Errorf("acquire project lock: %w", err)
	}
	defer lock.Release()
	return fn()
}

// WriteCheckLog stores the redacted full output under the plan-scoped audit and
// returns its project-relative path and SHA-256. It never writes into the archive.
func WriteCheckLog(ctx *Context, criterionID, output string) (string, string, error) {
	dir := filepath.Join(ctx.LedgerDir, checkLogsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	safe := sanitizeName(criterionID)
	name := fmt.Sprintf("%s-%d.log", safe, time.Now().UTC().UnixNano())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(output))
	rel, err := filepath.Rel(ctx.ProjectDir, path)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), hex.EncodeToString(sum[:]), nil
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "check"
	}
	return b.String()
}
