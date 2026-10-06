package ollamaagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/activity"
)

// TestHarnessSignalsProgressOfToolActivity proves the in-process harness marks a
// first-seen inspection and a verified mutation with the progress signal it
// already substantiated, and that a repeated inspection carries none. This is the
// producer side of AGENT-002's activity-vs-progress distinction; the runtrace
// collector consumes exactly these markers.
func TestHarnessSignalsProgressOfToolActivity(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`, // repeated → no progress signal
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	sink := &recordSink{}
	ctx := activity.WithRecorder(context.Background(), activity.New("TASK", sink))
	if _, err := New(cfg, dir).Execute(ctx, implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var novel, verified, reads int
	for _, e := range sink.events {
		switch e.Signal {
		case activity.SignalDiscoveryNovel:
			novel++
		case activity.SignalMutationVerified:
			verified++
		}
		if e.Stage == activity.StageDiscover && e.Action == "reading" {
			reads++
		}
	}
	if reads != 2 {
		t.Fatalf("reading events = %d, want 2 (both reads are activity)", reads)
	}
	if novel != 1 {
		t.Errorf("novel discovery signals = %d, want 1 (the repeated read is activity only)", novel)
	}
	if verified != 1 {
		t.Errorf("verified mutation signals = %d, want 1", verified)
	}
}
