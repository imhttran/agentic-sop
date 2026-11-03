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

// HeartbeatEvery is how often a held lease refreshes its record: well inside
// HeartbeatLimit, so a healthy run longer than the limit never looks stale where
// pid liveness cannot be probed (Windows, or a holder on another host).
const HeartbeatEvery = HeartbeatLimit / 5

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
	// Recovering is true when an explicit stale-lease recovery is in progress and
	// this start backed out rather than race it.
	Recovering bool
}

func (e *BusyError) Error() string {
	if e.Recovering {
		return "project is executing: a lease recovery is in progress; retry after it completes"
	}
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
	path     string
	owner    Owner
	f        *os.File
	released bool
	relErr   error
	stop     chan struct{}
	mu       sync.Mutex
}

func leasePath(dir string) string { return filepath.Join(dir, config.DirName, FileName) }

// Acquire takes the execution lease for the project at dir. It fails with
// *BusyError when another execution holds it, and never waits: an execution start
// must learn it is second, not queue behind a run that may take hours.
//
// The record is written before Acquire returns, so a reader that sees the file also
// sees who holds it. A start that races an explicit stale-lease recovery backs out
// (BusyError.Recovering) rather than create a second lease during the recovery.
func Acquire(dir string, owner Owner) (*Held, error) {
	if strings.TrimSpace(owner.Kind) == "" {
		return nil, errors.New("executionlease: owner kind is required")
	}
	path := leasePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if !errors.Is(err, os.ErrExist) {
				return nil, err
			}
			state, readErr := Read(dir)
			if readErr != nil {
				return nil, readErr
			}
			if !state.Present {
				// The holder released between the failed create and the read.
				continue
			}
			return nil, &BusyError{Owner: state.Owner, Stale: state.Stale(time.Now(), Hostname(), Grace, HeartbeatLimit)}
		}

		if owner.Owner == "" {
			owner.Owner = "sop"
		}
		if owner.Host == "" {
			owner.Host = Hostname()
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

		// A recovery claim means an explicit release of an older lease is in
		// progress. Back out rather than race it: the recovery may restore a lease
		// it found live, and two holders must never coexist.
		recovering, claimErr := claimsInProgress(dir, now, Grace)
		if claimErr != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, claimErr
		}
		if recovering {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, &BusyError{Recovering: true}
		}
		return &Held{path: path, owner: owner, f: f}, nil
	}
	return nil, errors.New("executionlease: the lease kept changing; retry")
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
	if err := h.holdsPathLocked(); err != nil {
		return err
	}
	h.owner.HeartbeatAt = now.UTC()
	return writeRecord(h.f, h.owner)
}

// WatchHeartbeat refreshes the lease every interval until Release or until a
// heartbeat fails, whichever comes first. It is the caller's job to invoke it on a
// lease it holds; an interval that is not positive disables it. onError, when set,
// receives the one heartbeat failure that stopped the watch, so a caller can report
// a lost lease without cancelling the execution. The returned channel is closed
// when the watch stops.
func (h *Held) WatchHeartbeat(interval time.Duration, onError func(error)) <-chan struct{} {
	done := make(chan struct{})
	if h == nil || interval <= 0 {
		close(done)
		return done
	}

	h.mu.Lock()
	if h.released || h.stop != nil {
		h.mu.Unlock()
		close(done)
		return done
	}
	stop := make(chan struct{})
	h.stop = stop
	h.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				if err := h.Heartbeat(now); err != nil {
					if onError != nil {
						onError(err)
					}
					return
				}
			}
		}
	}()
	return done
}

// holdsPathLocked reports whether the path still names the inode this execution
// created. A recovery that removed or replaced the lease must not be heartbeated
// silently: the execution no longer holds the project.
func (h *Held) holdsPathLocked() error {
	heldInfo, err := h.f.Stat()
	if err != nil {
		return fmt.Errorf("execution lease: stat held file: %w", err)
	}
	pathInfo, err := os.Stat(h.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrLeaseLost
		}
		return fmt.Errorf("execution lease at %s: %w", h.path, err)
	}
	if !os.SameFile(heldInfo, pathInfo) {
		return ErrLeaseLost
	}
	return nil
}

// Release drops the lease. It is idempotent and safe on a nil receiver. The file is
// removed only if it still names the inode this execution created, so a release can
// never delete a lease someone else took after ours was recovered; that also lets a
// holder clean up its own partial record after a failed heartbeat write. Any
// WatchHeartbeat watcher is stopped.
func (h *Held) Release() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released {
		return h.relErr
	}
	h.released = true
	if h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
	var errs []error
	if h.f != nil {
		heldInfo, statErr := h.f.Stat()
		if statErr != nil {
			errs = append(errs, fmt.Errorf("execution lease: stat held file: %w", statErr))
		}
		if closeErr := h.f.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("execution lease: close: %w", closeErr))
		}
		h.f = nil
		if heldInfo != nil {
			pathInfo, pathErr := os.Stat(h.path)
			switch {
			case errors.Is(pathErr, os.ErrNotExist):
				// Already released or recovered.
			case pathErr != nil:
				errs = append(errs, fmt.Errorf("execution lease at %s: %w", h.path, pathErr))
			case os.SameFile(heldInfo, pathInfo):
				if rmErr := os.Remove(h.path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
					errs = append(errs, fmt.Errorf("execution lease remove %s: %w", h.path, rmErr))
				}
			}
		}
	}
	h.relErr = errors.Join(errs...)
	return h.relErr
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
	return readStateFile(leasePath(dir))
}

// maxRecordBytes bounds a lease record; a larger file is treated as unparsable
// (held during grace) rather than read into memory.
const maxRecordBytes = 64 << 10

// readStateFile inspects one lease record; the recovery claim is read through the
// same parser. A missing file is not an error.
func readStateFile(path string) (State, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("execution lease at %s: %w", path, err)
	}
	state := State{Present: true, Modified: info.ModTime()}
	if info.Size() > maxRecordBytes {
		state.Unparsable = true
		return state, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
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
// live pid is not stale; where pid liveness cannot be probed (Windows) or the lease
// is held elsewhere, only the heartbeat can say. A record that cannot be parsed is
// given Grace, because a holder writes it immediately after creating the file.
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
		if live, known := alive(s.Owner.PID); known {
			return !live
		}
		// Liveness cannot be probed on this platform (Windows): the heartbeat is the
		// only signal, so fall through to it.
	}
	if s.Owner.HeartbeatAt.IsZero() {
		return now.Sub(s.Modified) > heartbeatLimit
	}
	return now.Sub(s.Owner.HeartbeatAt) > heartbeatLimit
}

var (
	// ErrLeaseChanged reports that the lease on disk is no longer the one the caller
	// inspected, so a stale release would have released someone else's execution.
	ErrLeaseChanged = errors.New("execution lease changed since it was inspected")
	// ErrLeaseNotStale reports that the lease still has a live owner (a running pid
	// or a fresh heartbeat), so an explicit recovery must leave it alone.
	ErrLeaseNotStale = errors.New("execution lease is not stale")
	// ErrLeaseLost reports that the path no longer names the inode this holder
	// created; the execution no longer holds the project.
	ErrLeaseLost = errors.New("execution lease was lost")
)

// ReleaseStale removes an inspected stale lease. It first links the lease to a
// private claim (the lease path stays in place), re-reads that inode, and removes the
// path only if the captured record is still the expected owner and still stale. A
// start that races the recovery backs out while the claim is fresh (Acquire), so a
// release cannot delete, or restore over, a lease taken in the meantime.
//
// Pass the zero Owner to release an unparsable record that is stale by mtime; a
// zero Owner never matches a parsed record.
func ReleaseStale(dir string, expect Owner) error {
	path := leasePath(dir)
	claim := fmt.Sprintf("%s.recover.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.Link(path, claim); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("execution lease claim %s: %w", claim, err)
	}
	removeClaim := func() { _ = os.Remove(claim) }
	defer removeClaim()

	state, err := readStateFile(claim)
	if err != nil {
		return err
	}
	if !state.Present {
		return nil
	}
	if state.Unparsable {
		if expect.Kind != "" {
			return ErrLeaseChanged
		}
	} else if !state.Owner.Same(expect) {
		return ErrLeaseChanged
	}
	if !state.Stale(time.Now(), Hostname(), Grace, HeartbeatLimit) {
		return ErrLeaseNotStale
	}

	// The path must still name the inode we claimed. If it does not, a recovery or a
	// start moved on and this release must not touch it.
	claimInfo, err := os.Stat(claim)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("execution lease claim %s: %w", claim, err)
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("execution lease at %s: %w", path, err)
	}
	if !os.SameFile(claimInfo, pathInfo) {
		return ErrLeaseChanged
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("execution lease remove %s: %w", path, err)
	}
	return nil
}

// claimsInProgress reports whether a lease recovery is in progress (a claim younger
// than grace). Older claims are a crashed recovery's hard link; removing a link
// never affects the lease path, so they are cleaned up and ignored.
func claimsInProgress(dir string, now time.Time, grace time.Duration) (bool, error) {
	claims, err := filepath.Glob(filepath.Join(dir, config.DirName, FileName+".recover.*"))
	if err != nil {
		return false, err
	}
	fresh := false
	for _, claim := range claims {
		info, statErr := os.Stat(claim)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return false, fmt.Errorf("execution lease claim %s: %w", claim, statErr)
		}
		if now.Sub(info.ModTime()) <= grace {
			fresh = true
			continue
		}
		_ = os.Remove(claim)
	}
	return fresh, nil
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
