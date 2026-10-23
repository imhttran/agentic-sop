package hipcompare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LedgerVersion identifies the shared experiment-budget ledger schema.
const LedgerVersion = "hip-003.budget.v1"

// Ledger entry states. Every reservation counts against the ceiling, including a
// reservation whose process crashed before completion.
const (
	LedgerReserved     = "reserved"
	LedgerCompleted    = "completed"
	LedgerFailed       = "failed"
	LedgerLimitFailure = "limit_failure"
)

// LedgerEntry is one reserved arm execution.
type LedgerEntry struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	TaskID      string    `json:"task_id"`
	Arm         Arm       `json:"arm"`
	OutputDir   string    `json:"output_dir,omitempty"`
	Status      string    `json:"status"`
	ReservedAt  time.Time `json:"reserved_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// ledgerFile is the durable shared budget state. It is a file, not a service or
// database, and is intentionally independent of any one output directory.
type ledgerFile struct {
	Version       string        `json:"version"`
	MaxExecutions int           `json:"max_executions"`
	Entries       []LedgerEntry `json:"entries"`
}

// ExperimentLedger is a path-backed, process-safe reservation ledger. The first
// run fixes the ceiling; later runs cannot raise it. A reservation is written
// before an arm launches, so a crash, timeout, or failed arm remains counted.
type ExperimentLedger struct {
	Path string
	Max  int
}

// NewExperimentLedger returns a ledger at path with the requested fixed ceiling.
func NewExperimentLedger(path string, max int) *ExperimentLedger {
	if max <= 0 {
		max = DefaultBudget().MaxExecutions
	}
	return &ExperimentLedger{Path: path, Max: max}
}

// Reserve counts an arm execution before it launches and returns its reservation ID.
func (l *ExperimentLedger) Reserve(fingerprint, taskID string, arm Arm, outputDir string) (string, error) {
	if l == nil || l.Path == "" {
		return "", errors.New("ledger: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return "", fmt.Errorf("ledger: create directory: %w", err)
	}
	f, err := os.OpenFile(l.Path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return "", fmt.Errorf("ledger: open: %w", err)
	}
	defer f.Close()
	if err := lockLedger(f); err != nil {
		return "", fmt.Errorf("ledger: lock: %w", err)
	}
	defer unlockLedger(f)

	state, err := readLedger(f)
	if err != nil {
		return "", err
	}
	if state.Version == "" {
		state.Version = LedgerVersion
	}
	if state.Version != LedgerVersion {
		return "", fmt.Errorf("ledger: unsupported version %q", state.Version)
	}
	if state.MaxExecutions == 0 {
		state.MaxExecutions = l.Max
	}
	if state.MaxExecutions != l.Max {
		return "", fmt.Errorf("ledger: ceiling is already fixed at %d, requested %d", state.MaxExecutions, l.Max)
	}
	if len(state.Entries) >= state.MaxExecutions {
		return "", &BudgetError{Reason: fmt.Sprintf(
			"budget exhaustion: %d reservations already recorded of the shared maximum %d; refusing to launch another arm",
			len(state.Entries), state.MaxExecutions)}
	}
	id := fmt.Sprintf("%s:%s:%d", fingerprint, arm, time.Now().UTC().UnixNano())
	state.Entries = append(state.Entries, LedgerEntry{
		ID:          id,
		Fingerprint: fingerprint,
		TaskID:      taskID,
		Arm:         arm,
		OutputDir:   outputDir,
		Status:      LedgerReserved,
		ReservedAt:  time.Now().UTC(),
	})
	if err := writeLedger(f, state); err != nil {
		return "", err
	}
	return id, nil
}

// Complete records the final state of a reservation. A missing reservation is an
// error: completion must never create an unaccounted execution.
func (l *ExperimentLedger) Complete(id, status string) error {
	switch status {
	case LedgerCompleted, LedgerFailed, LedgerLimitFailure:
	default:
		return fmt.Errorf("ledger: invalid completion status %q", status)
	}
	if l == nil || l.Path == "" {
		return errors.New("ledger: path is required")
	}
	f, err := os.OpenFile(l.Path, os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("ledger: open: %w", err)
	}
	defer f.Close()
	if err := lockLedger(f); err != nil {
		return fmt.Errorf("ledger: lock: %w", err)
	}
	defer unlockLedger(f)
	state, err := readLedger(f)
	if err != nil {
		return err
	}
	for i := range state.Entries {
		if state.Entries[i].ID != id {
			continue
		}
		if state.Entries[i].Status != LedgerReserved && state.Entries[i].Status != status {
			return fmt.Errorf("ledger: reservation %s already finished as %s", id, state.Entries[i].Status)
		}
		state.Entries[i].Status = status
		state.Entries[i].CompletedAt = time.Now().UTC()
		return writeLedger(f, state)
	}
	return fmt.Errorf("ledger: reservation %s not found", id)
}

// Count returns the number of reservations and the fixed ceiling. A missing
// ledger means no reservations and the configured default ceiling.
func (l *ExperimentLedger) Count() (int, int, error) {
	if l == nil || l.Path == "" {
		return 0, 0, errors.New("ledger: path is required")
	}
	f, err := os.Open(l.Path)
	if os.IsNotExist(err) {
		return 0, l.Max, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("ledger: open: %w", err)
	}
	defer f.Close()
	if err := lockLedger(f); err != nil {
		return 0, 0, fmt.Errorf("ledger: lock: %w", err)
	}
	defer unlockLedger(f)
	state, err := readLedger(f)
	if err != nil {
		return 0, 0, err
	}
	max := state.MaxExecutions
	if max == 0 {
		max = l.Max
	}
	return len(state.Entries), max, nil
}

// Read returns the current ledger state.
func (l *ExperimentLedger) Read() (ledgerFile, error) {
	if l == nil || l.Path == "" {
		return ledgerFile{}, errors.New("ledger: path is required")
	}
	f, err := os.Open(l.Path)
	if err != nil {
		return ledgerFile{}, err
	}
	defer f.Close()
	return readLedger(f)
}

func readLedger(r io.Reader) (ledgerFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return ledgerFile{}, fmt.Errorf("ledger: read: %w", err)
	}
	if len(data) == 0 {
		return ledgerFile{}, nil
	}
	var state ledgerFile
	if err := json.Unmarshal(data, &state); err != nil {
		return ledgerFile{}, fmt.Errorf("ledger: decode: %w", err)
	}
	return state, nil
}

func writeLedger(f *os.File, state ledgerFile) error {
	if state.Version == "" {
		state.Version = LedgerVersion
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("ledger: encode: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("ledger: truncate: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("ledger: seek: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("ledger: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("ledger: sync: %w", err)
	}
	return nil
}
