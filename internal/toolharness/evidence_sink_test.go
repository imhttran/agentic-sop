package toolharness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEvidenceSinkCapturesRawCommandOutput(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir+"/go.mod", "module fixture\n\ngo 1.21\n")
	writeFixture(t, dir+"/p.go", "package fixture\n")
	var got []CommandEvidence
	cfg := Config{CommandTimeout: 60 * time.Second, MaxOutputBytes: 1 << 20,
		EvidenceSink: func(ev CommandEvidence) { got = append(got, ev) }}
	h := New(dir, cfg, nil)

	if _, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."}); err != nil {
		t.Fatalf("run_command: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("evidence records = %d, want 1", len(got))
	}
	ev := got[0]
	if ev.Command != "go list ./..." {
		t.Errorf("command = %q", ev.Command)
	}
	if ev.Cwd != h.Root() {
		t.Errorf("cwd = %q, want the root", ev.Cwd)
	}
	if ev.Exit != 0 {
		t.Errorf("exit = %d, want 0", ev.Exit)
	}
	if !strings.Contains(ev.Output, "fixture") {
		t.Errorf("captured output did not carry the raw result: %q", ev.Output)
	}
}

// TestEvidenceSinkRecordsFailureExit proves a non-zero exit is captured (the raw
// output is preserved) rather than dropped.
func TestEvidenceSinkRecordsFailureExit(t *testing.T) {
	dir := t.TempDir()
	var got []CommandEvidence
	cfg := Config{CommandTimeout: 60 * time.Second, MaxOutputBytes: 1 << 20,
		EvidenceSink: func(ev CommandEvidence) { got = append(got, ev) }}
	h := New(dir, cfg, nil)
	writeFixture(t, dir+"/bad.go", "not go")
	_, _ = h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go build ./..."})
	if len(got) != 1 || got[0].Exit == 0 {
		t.Fatalf("a failing command's evidence was not captured with its exit: %+v", got)
	}
}
