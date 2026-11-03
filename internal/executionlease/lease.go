// Package executionlease provides the project-scoped execution lease that makes
// "one active execution per project" hold across the CLI, MCP and ACP interfaces
// (docs/plans/PROPOSAL-SHARED-EXECUTION-EXCLUSION.md).
//
// Three locks in SOP answer three different questions:
//
//	projectlock state.lock      may I write project metadata?
//	started-work work/.lock     may I decide to launch a job?
//	executionlease state.exec.lock   is an execution already running?
//
// This package owns the third. It is a file holding an owner record: the lease is
// created exclusively, so exactly one execution holds it, and it stays held for the
// lifetime of that execution rather than for the lifetime of the request that
// started it.
//
// Nothing calls this package yet. It is landed behind the "off" configuration
// (rollout step 1 of the proposal) so the wiring in the CLI, MCP and ACP interfaces
// can be reviewed on its own.
package executionlease

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// FileName is the lease file, relative to the project state directory. It is
// separate from projectlock's state.lock and from the started-work start lock, so
// an execution lease never serializes a metadata write and never rewrites state.
const FileName = "state.exec.lock"

// Defaults for staleness. A cross-host lease with no heartbeat for longer than
// HeartbeatLimit is stale; a lease file whose record cannot be parsed is treated as
// in-progress for Grace before it counts as abandoned.
const (
	HeartbeatInterval = 30 * time.Second
	HeartbeatLimit    = 5 * time.Minute
	Grace             = 10 * time.Second
)

// Mode is the configured strength of the lease: off changes no behaviour, advisory
// records contention without blocking, required refuses a competing execution.
type Mode string

const (
	ModeOff      Mode = "off"
	ModeAdvisory Mode = "advisory"
	ModeRequired Mode = "required"
)

// ParseMode reads a configured mode. The empty string is ModeOff, so an omitted
// configuration block leaves execution unchanged.
func ParseMode(s string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return ModeOff, nil
	case ModeOff, ModeAdvisory, ModeRequired:
		return Mode(strings.ToLower(strings.TrimSpace(s))), nil
	}
	return "", fmt.Errorf("execution.lease: unknown mode %q (want off, advisory, required)", s)
}

// Kind names what holds the lease.
const (
	KindRun    = "run"
	KindPrompt = "prompt"
)

// Owner is the record written into the lease file.
type Owner struct {
	Owner       string    `json:"owner"`
	PID         int       `json:"pid"`
	Host        string    `json:"host"`
	Kind        string    `json:"kind"`
	JobID       string    `json:"job_id,omitempty"`
	TaskID      string    `json:"task_id,omitempty"`
	Attempt     int       `json:"attempt,omitempty"`
	ArgvDigest  string    `json:"argv_digest,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
}

// Same reports whether two records describe the same execution: a stale release
// must not delete a lease taken by someone else in the meantime.
func (o Owner) Same(other Owner) bool {
	return o.PID == other.PID && o.Host == other.Host && o.Kind == other.Kind &&
		o.JobID == other.JobID && o.TaskID == other.TaskID && o.StartedAt.Equal(other.StartedAt)
}

// Describe renders the holder for an operator: who, what, where, since when.
func (o Owner) Describe() string {
	what := o.Kind
	if o.TaskID != "" {
		what += " " + o.TaskID
	}
	if o.JobID != "" {
		what += " (" + o.JobID + ")"
	}
	where := fmt.Sprintf("pid %d", o.PID)
	if o.Host != "" {
		where += " on " + o.Host
	}
	since := ""
	if !o.StartedAt.IsZero() {
		since = " since " + o.StartedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%s by %s%s", what, where, since)
}

// BusyError reports a project whose execution lease is held. Stale is set when the
// holder looks abandoned, which is reported rather than acted on: a start never
// steals a lease (proposal §4).
type BusyError struct {
	Owner Owner
	Stale bool
}

func (e *BusyError) Error() string {
	stale := ""
	if e.Stale {
		stale = " [stale: no live owner; `sop resume` reports and releases it]"
	}
	if e.Owner.Kind == "" {
		return "project is executing: a lease record is being written" + stale
	}
	return "project is executing: " + e.Owner.Describe() + stale
}

// Held is a held lease. Release is idempotent, so a deferred release after an
// explicit one is safe.
type Held struct {
	dir   string
	path  string
	owner Owner
	f     *os.File
	once  sync.Once
	mu    sync.Mutex
}

func leasePath(dir string) string { return filepath.Join(dir, config.DirName, FileName) }

// Acquire takes the execution lease for the project at dir. It fails with
// *BusyError when another execution holds it, and never waits: an execution start
// must learn it is second, not queue behind a run that may take hours.
//
// The record is written before Acquire returns, so a reader that sees the file also
// sees who holds it.
func Acquire(dir string, owner Owner) (*Held, error) {
	path := leasePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		state, readErr := Read(dir)
		if readErr != nil {
			return nil, readErr
		}
		return nil, &BusyError{Owner: state.Owner, Stale: state.Stale(time.Now(), Hostname(), Grace, HeartbeatLimit)}
	}

	if owner.Owner == "" {
		owner.Owner = "sop"
	}
	if owner.Host == "" {
		owner.Host, _ = os.Hostname()
	}
	if owner.PID == 0 {
		owner.PID = os.Getpid()
	}
	now := time.Now().UTC()
	if owner.StartedAt.IsZero() {
		owner.StartedAt = now
	}
	if owner.HeartbeatAt.IsZero() {
		owner.HeartbeatAt = now
	}
	if err := writeRecord(f, owner); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &Held{dir: dir, path: path, owner: owner, f: f}, nil
}

// Owner returns the record this lease holds.
func (h *Held) Owner() Owner {
	if h == nil {
		return Owner{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.owner
}

// Heartbeat refreshes the owner record's heartbeat, so a reader on another host can
// tell a live execution from an abandoned one.
func (h *Held) Heartbeat(now time.Time) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.f == nil {
		return nil
	}
	h.owner.HeartbeatAt = now.UTC()
	return writeRecord(h.f, h.owner)
}

// Release drops the lease. It is idempotent and safe on a nil receiver. The file is
// removed only if it still describes this execution, so a release can never delete
// a lease someone else took after ours was recovered.
func (h *Held) Release() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.f != nil {
			_ = h.f.Close()
			h.f = nil
		}
		if state, err := Read(h.dir); err == nil {
			if state.Present && state.Owner.Same(h.owner) {
				_ = os.Remove(h.path)
			}
		}
	})
}

// State is what a read found on disk.
type State struct {
	// Present is true when a lease file exists.
	Present bool
	// Owner is the recorded owner, zero when Unparsable.
	Owner Owner
	// Unparsable is true when a lease file exists but its record cannot be read: the
	// file was created but not yet written, or is being rewritten. It counts as held.
	Unparsable bool
	// Modified is when the file was last written. Staleness is judged against it so a
	// reader can evaluate a lease at a time of its choosing.
	Modified time.Time
}

// Read inspects the project's lease file. A missing file is not an error.
func Read(dir string) (State, error) {
	path := leasePath(dir)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("execution lease at %s: %w", path, err)
	}
	state := State{Present: true, Modified: info.ModTime()}
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("execution lease at %s: %w", path, err)
	}
	var owner Owner
	if err := json.Unmarshal(raw, &owner); err != nil || owner.Kind == "" {
		state.Unparsable = true
		return state, nil
	}
	state.Owner = owner
	return state, nil
}

// Stale reports whether the lease looks abandoned. A lease held on this host by a
// live pid is not stale; on another host only the heartbeat can say. A record that
// cannot be parsed is given Grace, because a holder writes it immediately after
// creating the file.
//
// Staleness is a report, never an action: nothing here releases a lease.
func (s State) Stale(now time.Time, hostname string, grace, heartbeatLimit time.Duration) bool {
	if !s.Present {
		return false
	}
	if s.Unparsable {
		return now.Sub(s.Modified) > grace
	}
	if s.Owner.Host != "" && s.Owner.Host == hostname && s.Owner.PID > 0 {
		return !alive(s.Owner.PID)
	}
	if s.Owner.HeartbeatAt.IsZero() {
		return now.Sub(s.Modified) > heartbeatLimit
	}
	return now.Sub(s.Owner.HeartbeatAt) > heartbeatLimit
}

// ErrLeaseChanged reports that the lease on disk is no longer the one the caller
// inspected, so a stale release would have released someone else's execution.
var ErrLeaseChanged = errors.New("execution lease changed since it was inspected")

// ReleaseStale removes an abandoned lease after an operator has inspected it. It
// refuses when the lease no longer matches expect, so a start that raced the
// recovery cannot be undone.
func ReleaseStale(dir string, expect Owner) error {
	state, err := Read(dir)
	if err != nil {
		return err
	}
	if !state.Present {
		return nil
	}
	if state.Unparsable || !state.Owner.Same(expect) {
		return ErrLeaseChanged
	}
	if err := os.Remove(leasePath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Hostname is the local host name as recorded in a lease.
func Hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// writeRecord rewrites the lease file in place. Readers may see a partial record,
// which they treat as held, never as free.
func writeRecord(f *os.File, owner Owner) error {
	raw, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}
