package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileChannelConfirmsDurableAppend(t *testing.T) {
	e := mustEvent(t, completedParams())
	path := filepath.Join(t.TempDir(), "inbox", "events.jsonl")
	c := NewFileChannel(path)

	ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ack != AckConfirmed {
		t.Fatalf("ack = %q, want %q", ack, AckConfirmed)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read receiver content: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("receiver holds %d records, want 1: %q", len(lines), string(data))
	}
	var got Event
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("receiver content is not the event payload: %v", err)
	}
	if got.ID != e.ID || got.Kind != e.Kind {
		t.Fatalf("receiver event = %q/%q, want %q/%q", got.ID, got.Kind, e.ID, e.Kind)
	}
}

// TestFileChannelRecordsOneRecordPerEventID proves the file channel honours the
// idempotency key: a retry of one event confirms delivery without a second
// record, so the receiver never holds two records for one event id.
func TestFileChannelRecordsOneRecordPerEventID(t *testing.T) {
	e := mustEvent(t, completedParams())
	path := filepath.Join(t.TempDir(), "events.jsonl")
	c := NewFileChannel(path)
	for i := 0; i < 2; i++ {
		if ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil || ack != AckConfirmed {
			t.Fatalf("send %d: ack=%q err=%v", i, ack, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read receiver content: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("receiver holds %d records, want 1", len(lines))
	}
	var got Event
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("receiver line is not the event payload: %v", err)
	}
	if got.ID != e.ID {
		t.Fatalf("record id = %q, want %q", got.ID, e.ID)
	}
}

// TestFileChannelRecordsDistinctEventIDs proves two distinct events still each
// get one record: the channel deduplicates on the event id only.
func TestFileChannelRecordsDistinctEventIDs(t *testing.T) {
	first := mustEvent(t, completedParams())
	p := completedParams()
	p.Attempt = 2
	second := mustEvent(t, p)
	path := filepath.Join(t.TempDir(), "events.jsonl")
	c := NewFileChannel(path)
	for _, e := range []Event{first, second, first} {
		if ack, err := c.Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1}); err != nil || ack != AckConfirmed {
			t.Fatalf("send event %q: ack=%q err=%v", e.ID, ack, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read receiver content: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("receiver holds %d records, want 2", len(lines))
	}
}

func TestFileChannelRejectsUnwritablePath(t *testing.T) {
	e := mustEvent(t, completedParams())
	// A directory path cannot be opened for append, so the open fails with a
	// definitive error.
	ack, err := NewFileChannel(t.TempDir()).Send(Delivery{Event: e, IdempotencyKey: e.ID, Attempt: 1})
	if ack != AckRejected {
		t.Fatalf("ack = %q, want %q (err %v)", ack, AckRejected, err)
	}
	if err == nil {
		t.Fatal("expected an error for an unwritable path")
	}
}

// failingWriter is an io.Writer whose Write always fails, so the post-open write
// path is exercised deterministically without depending on file permissions.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, os.ErrClosed }
func (failingWriter) Sync() error                 { return nil }

func TestFileChannelAmbiguousOnWriteFailure(t *testing.T) {
	// A write after a successful open is the ambiguous case. Exercise it through
	// appendRecord, which the channel uses for the post-open write and sync.
	e := mustEvent(t, completedParams())
	ack, err := appendRecord(failingWriter{}, e, []byte("{}"+"\n"))
	if ack != AckAmbiguous {
		t.Fatalf("ack = %q, want %q for a post-open write failure (err %v)", ack, AckAmbiguous, err)
	}
	if err == nil {
		t.Fatal("expected an error for a post-open write failure")
	}
}

func TestFileChannelAmbiguousOnSyncFailure(t *testing.T) {
	e := mustEvent(t, completedParams())
	// A writer that accepts the bytes but cannot sync reports an unknown outcome.
	ack, err := appendRecord(noSyncWriter{}, e, []byte("{}"+"\n"))
	if ack != AckAmbiguous {
		t.Fatalf("ack = %q, want %q for a sync failure (err %v)", ack, AckAmbiguous, err)
	}
	if err == nil {
		t.Fatal("expected an error for a sync failure")
	}
}

// noSyncWriter accepts every write but fails to sync.
type noSyncWriter struct{}

func (noSyncWriter) Write(p []byte) (int, error) { return len(p), nil }
func (noSyncWriter) Sync() error                 { return os.ErrClosed }

func TestFileChannelCapabilitiesHonorKey(t *testing.T) {
	caps := NewFileChannel(filepath.Join(t.TempDir(), "events.jsonl")).Capabilities()
	if !caps.KeyHonored {
		t.Fatal("file channel does not claim key honoring, but it records one event id at most once")
	}
	if caps.Guarantee == "" {
		t.Fatal("file channel makes no guarantee statement")
	}
	if strings.Contains(strings.ToLower(caps.Guarantee), "may not honour") {
		t.Fatalf("file channel guarantee contradicts KeyHonored: %q", caps.Guarantee)
	}
}
