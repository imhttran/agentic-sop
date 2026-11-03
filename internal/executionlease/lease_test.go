package executionlease

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// newProject returns a temporary project root with a state directory.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// One execution holds the project: the second acquire is refused and names the
// holder, and the refusal is not a wait.
func TestAcquireRefusesSecondExecution(t *testing.T) {
	root := newProject(t)

	first, err := Acquire(root, Owner{Kind: KindRun, TaskID: "T1"})
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Present || state.Owner.Kind != KindRun || state.Owner.TaskID != "T1" {
		t.Fatalf("read = %+v, want the run record", state)
	}
	if state.Owner.PID != os.Getpid() || state.Owner.Host == "" || state.Owner.StartedAt.IsZero() {
		t.Fatalf("owner not defaulted: %+v", state.Owner)
	}

	second, err := Acquire(root, Owner{Kind: KindPrompt})
	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("second Acquire = %v, want *BusyError", err)
	}
	if busy.Owner.TaskID != "T1" || !strings.Contains(busy.Error(), "run T1") {
		t.Fatalf("busy = %q, want it to name the holder", busy.Error())
	}
	if second != nil {
		t.Fatal("second Acquire returned a lease")
	}
}

// Releasing hands the project to the next execution; release is idempotent and safe
// on a nil receiver.
func TestReleaseFreesTheProject(t *testing.T) {
	root := newProject(t)

	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}

	var nilHeld *Held
	if err := nilHeld.Release(); err != nil {
		t.Fatalf("nil Release: %v", err)
	}

	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("lease still present after release: %+v", state)
	}

	again, err := Acquire(root, Owner{Kind: KindPrompt})
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	again.Release()
}

// The heartbeat is what tells another host the execution is alive.
func TestHeartbeatRefreshesTheRecord(t *testing.T) {
	root := newProject(t)

	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	later := time.Now().UTC().Add(time.Minute)
	if err := held.Heartbeat(later); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Owner.HeartbeatAt.Equal(later) {
		t.Fatalf("heartbeat = %s, want %s", state.Owner.HeartbeatAt, later)
	}
	if !held.Owner().Same(state.Owner) {
		t.Fatalf("owner on disk %+v differs from the held one %+v", state.Owner, held.Owner())
	}
}

// A lease held by a dead pid on this host is reported stale, and a live one is not.
func TestStaleByPIDOnThisHost(t *testing.T) {
	now := time.Now()
	host := Hostname()

	dead := State{Present: true, Owner: Owner{Kind: KindRun, PID: 999999, Host: host, HeartbeatAt: now.Add(-2 * HeartbeatLimit)}}
	if !dead.Stale(now, host, Grace, HeartbeatLimit) {
		t.Fatal("lease with a dead pid reported live")
	}

	live := State{Present: true, Owner: Owner{Kind: KindRun, PID: os.Getpid(), Host: host, HeartbeatAt: now}}
	if live.Stale(now, host, Grace, HeartbeatLimit) {
		t.Fatal("lease with a live pid reported stale")
	}
}

// On another host only the heartbeat can say, and its absence is bounded.
func TestStaleByHeartbeatAcrossHosts(t *testing.T) {
	now := time.Now()
	host := Hostname()

	old := State{Present: true, Owner: Owner{Kind: KindPrompt, Host: "elsewhere", HeartbeatAt: now.Add(-2 * HeartbeatLimit)}}
	if !old.Stale(now, host, Grace, HeartbeatLimit) {
		t.Fatal("lease with an expired heartbeat reported live")
	}

	fresh := State{Present: true, Owner: Owner{Kind: KindPrompt, Host: "elsewhere", HeartbeatAt: now.Add(-HeartbeatInterval)}}
	if fresh.Stale(now, host, Grace, HeartbeatLimit) {
		t.Fatal("lease with a fresh heartbeat reported stale")
	}
}

// A record that cannot be parsed is held during the grace period and stale after
// it: the holder writes the record immediately after creating the file.
func TestUnparsableLeaseIsHeldThenStale(t *testing.T) {
	root := newProject(t)
	path := filepath.Join(root, config.DirName, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Present || !state.Unparsable || state.Owner.Kind != "" {
		t.Fatalf("read = %+v, want an unparsable present lease", state)
	}
	if state.Stale(time.Now(), Hostname(), time.Minute, HeartbeatLimit) {
		t.Fatal("unparsable lease reported stale inside the grace period")
	}
	if !state.Stale(time.Now().Add(time.Hour), Hostname(), time.Second, HeartbeatLimit) {
		t.Fatal("unparsable lease reported live after the grace period")
	}

	_, err = Acquire(root, Owner{Kind: KindRun})
	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("Acquire over an unparsable lease = %v, want *BusyError", err)
	}
	if !strings.Contains(busy.Error(), "being written") {
		t.Fatalf("busy = %q, want it to describe a lease being written", busy.Error())
	}
}

// A stale release is compare-and-delete: it never removes a lease someone else took
// after the inspection.
func TestReleaseStaleRefusesAChangedLease(t *testing.T) {
	root := newProject(t)

	held, err := Acquire(root, Owner{Kind: KindRun, PID: 999999})
	if err != nil {
		t.Fatal(err)
	}
	inspected := held.Owner()
	// Simulate another execution taking the project after the inspection.
	if err := os.Remove(filepath.Join(root, config.DirName, FileName)); err != nil {
		t.Fatal(err)
	}
	other, err := Acquire(root, Owner{Kind: KindPrompt})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()

	if err := ReleaseStale(root, inspected); !errors.Is(err, ErrLeaseChanged) {
		t.Fatalf("ReleaseStale = %v, want ErrLeaseChanged", err)
	}
	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Present || state.Owner.Kind != KindPrompt {
		t.Fatalf("release removed another execution's lease: %+v", state)
	}
}

// Releasing an inspected stale lease frees the project.
func TestReleaseStaleFreesTheProject(t *testing.T) {
	root := newProject(t)

	held, err := Acquire(root, Owner{Kind: KindRun, PID: 999999})
	if err != nil {
		t.Fatal(err)
	}
	// Make the record stale even where pid liveness cannot be probed (Windows).
	if err := held.Heartbeat(time.Now().UTC().Add(-2 * HeartbeatLimit)); err != nil {
		t.Fatal(err)
	}
	inspected := held.Owner()
	if err := ReleaseStale(root, inspected); err != nil {
		t.Fatalf("ReleaseStale: %v", err)
	}
	state, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("lease still present: %+v", state)
	}
}

// The configuration gate is off unless it is asked for.
func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"":           ModeOff,
		"off":        ModeOff,
		"advisory":   ModeAdvisory,
		"required":   ModeRequired,
		" Required ": ModeRequired,
	} {
		got, err := ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseMode("sometimes"); err == nil {
		t.Error("ParseMode accepted an unknown mode")
	}
}

// Releasing an unparsable record is allowed only after it is stale by mtime, and
// only with the zero Owner (there is no parsed owner to compare).
func TestReleaseStaleReleasesAnUnparsableLease(t *testing.T) {
	root := newProject(t)
	path := filepath.Join(root, config.DirName, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseStale(root, Owner{}); !errors.Is(err, ErrLeaseNotStale) {
		t.Fatalf("ReleaseStale within grace = %v, want ErrLeaseNotStale", err)
	}
	if state, err := Read(root); err != nil || !state.Present {
		t.Fatalf("lease removed within grace: state=%+v err=%v", state, err)
	}

	old := time.Now().Add(-2 * Grace)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseStale(root, Owner{}); err != nil {
		t.Fatalf("ReleaseStale after grace: %v", err)
	}
	if state, err := Read(root); err != nil || state.Present {
		t.Fatalf("lease still present after release: state=%+v err=%v", state, err)
	}
}

// A live lease is never released, even when the caller passes its exact owner.
func TestReleaseStaleRefusesLiveLease(t *testing.T) {
	root := newProject(t)
	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	if err := ReleaseStale(root, held.Owner()); !errors.Is(err, ErrLeaseNotStale) {
		t.Fatalf("ReleaseStale = %v, want ErrLeaseNotStale", err)
	}
	if state, err := Read(root); err != nil || !state.Present {
		t.Fatalf("live lease was removed: state=%+v err=%v", state, err)
	}
}

// An owner without a kind would be written as a record Read treats as unparsable,
// which nothing could release: reject it before creating the lease.
func TestAcquireRequiresKind(t *testing.T) {
	root := newProject(t)
	if _, err := Acquire(root, Owner{}); err == nil {
		t.Fatal("Acquire accepted an owner without a kind")
	}
	if state, err := Read(root); err != nil || state.Present {
		t.Fatalf("Acquire created a lease for an invalid owner: state=%+v err=%v", state, err)
	}
}

// A start backs out while a recovery claim is fresh, and cleans up a stale claim.
func TestAcquireBacksOutDuringRecovery(t *testing.T) {
	root := newProject(t)
	claim := filepath.Join(root, config.DirName, FileName+".recover.test")
	if err := os.WriteFile(claim, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Acquire(root, Owner{Kind: KindRun})
	var busy *BusyError
	if !errors.As(err, &busy) || !busy.Recovering {
		t.Fatalf("Acquire with a fresh claim = %v, want a recovering *BusyError", err)
	}
	if state, readErr := Read(root); readErr != nil || state.Present {
		t.Fatalf("backed-out start left a lease: state=%+v err=%v", state, readErr)
	}

	old := time.Now().Add(-2 * Grace)
	if err := os.Chtimes(claim, old, old); err != nil {
		t.Fatal(err)
	}
	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatalf("Acquire with a stale claim: %v", err)
	}
	defer held.Release()
	if _, err := os.Stat(claim); !errors.Is(err, os.ErrNotExist) {
		t.Error("stale recovery claim was not cleaned up")
	}
}

// A holder that no longer owns the path learns it lost the lease instead of
// heartbeating an orphaned inode.
func TestHeartbeatDetectsLostLease(t *testing.T) {
	root := newProject(t)
	path := filepath.Join(root, config.DirName, FileName)

	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := held.Heartbeat(time.Now()); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Heartbeat after removal = %v, want ErrLeaseLost", err)
	}

	other, err := Acquire(root, Owner{Kind: KindPrompt})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	if err := held.Heartbeat(time.Now()); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Heartbeat after replacement = %v, want ErrLeaseLost", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("stale holder Release: %v", err)
	}
	if state, err := Read(root); err != nil || !state.Present || state.Owner.Kind != KindPrompt {
		t.Fatalf("stale holder removed the new lease: state=%+v err=%v", state, err)
	}
}

// A holder can clean up its own record even if a failed heartbeat write left it
// unparsable: ownership is the inode, not the JSON.
func TestReleaseRemovesPartialOwnRecord(t *testing.T) {
	root := newProject(t)
	path := filepath.Join(root, config.DirName, FileName)

	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if state, err := Read(root); err != nil || state.Present {
		t.Fatalf("partial own record left behind: state=%+v err=%v", state, err)
	}
}

// Release never removes a lease another execution acquired after ours was lost.
func TestReleaseDoesNotRemoveANewLease(t *testing.T) {
	root := newProject(t)
	path := filepath.Join(root, config.DirName, FileName)

	held, err := Acquire(root, Owner{Kind: KindRun})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	other, err := Acquire(root, Owner{Kind: KindPrompt})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()

	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if state, err := Read(root); err != nil || !state.Present || state.Owner.Kind != KindPrompt {
		t.Fatalf("release removed the new lease: state=%+v err=%v", state, err)
	}
}
