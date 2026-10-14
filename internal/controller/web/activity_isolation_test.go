package web

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/sopclient"
)

// CTRL007 S4 — transport isolation.
//
// The live activity transport is an observer: it reads SOP's persisted activity
// and never writes SOP state. This test proves that against *real* persisted
// activity. It snapshots activity.jsonl (and the whole runs tree), drives every
// activity delivery path — the poll window, a live SSE stream cancelled mid-flight,
// and a stream whose transport write fails — then re-reads the file and asserts it
// is byte-for-byte identical, and that no command/mutation endpoint was invoked
// (the SOP binary the controller's command path drives is never run).
//
// It is deterministic and offline: it never sleeps, never waits on a ticker, and
// needs no Ollama or network.

// recordingSop writes an executable "sop" stub that touches a marker file on every
// invocation and returns it, plus a predicate for whether it has been invoked. Any
// controller-initiated SOP mutation goes through this binary (Commander.Exec), so
// an untouched marker is a meaningful "no command endpoint was invoked" assertion;
// the S4 test also runs it once at the end as a positive control.
func recordingSop(t *testing.T) (bin string, invoked func() bool) {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	bin = filepath.Join(dir, "sop")
	script := "#!/bin/sh\n: >> " + marker + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	}
}

// errWriter is an http.ResponseWriter + http.Flusher whose writes always fail, so
// the stream transport-failure path can be driven without a live connection.
type errWriter struct{ header http.Header }

func (w *errWriter) Header() http.Header       { return w.header }
func (w *errWriter) Write([]byte) (int, error) { return 0, errors.New("transport write failed") }
func (w *errWriter) WriteHeader(int)           {}
func (w *errWriter) Flush()                    {}

// TestActivityTransportIsolation is the S4 gate: driving the activity transport
// must leave SOP's persisted activity untouched and must never invoke a command.
func TestActivityTransportIsolation(t *testing.T) {
	bin, commandInvoked := recordingSop(t)
	srv, id, root := newTestServerRoot(t, bin)

	runsDir := filepath.Join(root, ".agent-sdlc", "runs")
	activityPath := filepath.Join(runsDir, "t1", "activity.jsonl")

	// (1)+(2) Start from real persisted activity and snapshot its bytes.
	beforeActivity := mustReadFile(t, activityPath)
	if len(beforeActivity) == 0 {
		t.Fatalf("fixture must persist real activity.jsonl")
	}
	beforeTree := snapshotTree(t, runsDir)

	// (3) Exercise the read path. Confirm it delivers the persisted events, so the
	// isolation assertion below is not vacuous.
	full := getWindow(t, srv, id, "", 0)
	if len(full.Events) == 0 {
		t.Fatalf("expected activity events from persisted activity.jsonl")
	}

	// (3)+(4) Open the live SSE stream, consume a cursor-keyed frame, then cancel
	// it mid-flight: a real transport failure on a live connection.
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/projects/"+id+"/activity/stream?after="+full.Events[0].Cursor, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	frame := readFrame(t, bufio.NewReader(resp.Body))
	if !strings.Contains(frame, "id: ") {
		t.Fatalf("expected a cursor-keyed activity frame, got %q", frame)
	}
	cancel()
	resp.Body.Close()

	// (4) Force a transport write failure directly against the same read path, so
	// the guarantee does not rest only on client cancellation.
	client, err := sopclient.New([]string{root}, withApprovals(t, bin), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	h := &Handlers{sop: client}
	w := &errWriter{header: http.Header{}}
	var cursor string
	if h.streamActivity(context.Background(), w, w, id, &cursor, true) {
		t.Fatalf("a transport write failure must end the stream")
	}

	// (5)+(6) Re-read and assert byte-for-byte identity, and that no persisted
	// artifact changed, appeared, or disappeared.
	afterActivity := mustReadFile(t, activityPath)
	if !bytes.Equal(beforeActivity, afterActivity) {
		t.Fatalf("activity.jsonl changed across the transport exercise:\n before=%q\n after =%q", beforeActivity, afterActivity)
	}
	compareTree(t, beforeTree, snapshotTree(t, runsDir))

	// (7) No command/mutation endpoint was invoked. Every controller mutation goes
	// through the SOP binary, so the untouched marker is authoritative here.
	if commandInvoked() {
		t.Fatalf("activity transport invoked the SOP command path")
	}
	// Positive control: the recorder would have caught an invocation.
	if err := exec.Command(bin).Run(); err != nil {
		t.Fatalf("recording sop failed to run: %v", err)
	}
	if !commandInvoked() {
		t.Fatalf("recording sop did not record an invocation; the assertion above would be vacuous")
	}
}

// mustReadFile reads a file or fails the test.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// snapshotTree reads every regular file under dir into a path->bytes map, for a
// before/after equality check that catches any added, removed, or changed
// persisted artifact.
func snapshotTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[rel] = raw
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// compareTree fails when the two snapshots differ in any file, added or removed.
func compareTree(t *testing.T, before, after map[string][]byte) {
	t.Helper()
	for name, b := range before {
		a, ok := after[name]
		if !ok {
			t.Fatalf("artifact %s disappeared after the transport exercise", name)
		}
		if !bytes.Equal(b, a) {
			t.Fatalf("artifact %s changed after the transport exercise", name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			t.Fatalf("artifact %s appeared after the transport exercise", name)
		}
	}
}
