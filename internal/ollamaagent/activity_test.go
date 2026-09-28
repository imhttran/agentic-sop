package ollamaagent

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// recordSink captures activity events for assertions.
type recordSink struct{ events []activity.Event }

func (s *recordSink) Emit(e activity.Event) { s.events = append(s.events, e) }

// TestToolActivityMapping pins the tool-type mapping that turns a tool call into
// a concise activity summary rather than raw arguments.
func TestToolActivityMapping(t *testing.T) {
	cases := []struct {
		name   string
		args   map[string]any
		stage  string
		action string
		detail string
	}{
		{toolharness.ToolReadFile, map[string]any{"path": "internal/run/jev.go"}, activity.StageDiscover, "reading", "internal/run/jev.go"},
		{toolharness.ToolWriteFile, map[string]any{"path": "internal/run/jev.go", "content": "secret body"}, activity.StageChange, "editing", "internal/run/jev.go"},
		{toolharness.ToolCreateFile, map[string]any{"path": "new.go", "content": "x"}, activity.StageChange, "creating", "new.go"},
		{toolharness.ToolListFiles, map[string]any{"path": "internal"}, activity.StageDiscover, "listing", "internal"},
		{toolharness.ToolSearchFiles, map[string]any{"pattern": "func main"}, activity.StageDiscover, "searching", "func main"},
		{toolharness.ToolRunCommand, map[string]any{"command": "go test ./internal/run/..."}, activity.StageValidate, "go test ./internal/run/...", ""},
		{"unknown_tool", map[string]any{"path": "x"}, "", "", ""},
	}
	for _, tc := range cases {
		stage, action, detail := toolActivity(tc.name, tc.args)
		if stage != tc.stage || action != tc.action || detail != tc.detail {
			t.Errorf("toolActivity(%s) = (%q,%q,%q), want (%q,%q,%q)",
				tc.name, stage, action, detail, tc.stage, tc.action, tc.detail)
		}
	}
}

// TestToolActivityNeverLeaksFileContent proves a write's content never reaches
// the activity detail: only the path is reported.
func TestToolActivityNeverLeaksFileContent(t *testing.T) {
	_, _, detail := toolActivity(toolharness.ToolWriteFile, map[string]any{
		"path":    "secret.go",
		"content": "API_TOKEN=supersecret",
	})
	if strings.Contains(detail, "supersecret") {
		t.Errorf("detail = %q, must not contain file content", detail)
	}
}

// TestRedactCommandMasksSecrets proves a sensitive KEY=VALUE assignment is masked
// and the command is bounded to a single line.
func TestRedactCommandMasksSecrets(t *testing.T) {
	got := redactCommand("deploy --api-key=abc123 API_TOKEN=deadbeef run")
	if strings.Contains(got, "abc123") || strings.Contains(got, "deadbeef") {
		t.Errorf("redactCommand = %q, leaked a secret", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("redactCommand = %q, must be one line", got)
	}

	long := redactCommand("run " + strings.Repeat("x", 500))
	if len([]rune(long)) > maxActivityDetail+1 {
		t.Errorf("redactCommand result = %d runes, want bounded", len([]rune(long)))
	}
}

// TestHarnessEmitsToolActivity proves the in-process harness reports tool activity
// through the recorder carried by the context, without a live Ollama server.
func TestHarnessEmitsToolActivity(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	sink := &recordSink{}
	ctx := activity.WithRecorder(context.Background(), activity.New("TASK1", sink))

	if _, err := New(cfg, dir).Execute(ctx, implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	assertEvent(t, sink.events, activity.StageDiscover, "reading", "pkg/a.go")
	assertEvent(t, sink.events, activity.StageChange, "editing", "out.txt")
	for _, e := range sink.events {
		if e.TaskID != "TASK1" {
			t.Errorf("event %+v has TaskID %q, want TASK1", e, e.TaskID)
		}
		if e.Timestamp.IsZero() {
			t.Errorf("event %+v has no timestamp", e)
		}
	}
}

// TestHarnessEmitsFinalizeActivity proves forcing finalization is visible as a
// FINALIZE activity event, without a live server.
func TestHarnessEmitsFinalizeActivity(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, fixMutationWrites("act", implementForceFinalizeAfter+3)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	sink := &recordSink{}
	ctx := activity.WithRecorder(context.Background(), activity.New("TASK2", sink))

	// The run is expected to fail with a finalization limit; the activity is still
	// emitted. The assertion is about the event, not the outcome.
	_, _ = New(cfg, dir).Execute(ctx, implementRequest())

	found := false
	for _, e := range sink.events {
		if e.Stage == activity.StageFinalize {
			found = true
		}
	}
	if !found {
		t.Errorf("no FINALIZE activity event in %+v", sink.events)
	}
}

// TestHarnessWithoutRecorderIsUnchanged proves a context with no recorder emits
// nothing and does not affect execution.
func TestHarnessWithoutRecorderIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`,
		`{"status":"completed","summary":"done","changes_expected":false}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	if _, err := New(cfg, dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute without a recorder failed: %v", err)
	}
}

func assertEvent(t *testing.T, events []activity.Event, stage, action, detail string) {
	t.Helper()
	for _, e := range events {
		if e.Stage == stage && e.Action == action && e.Detail == detail {
			return
		}
	}
	t.Errorf("no event (%s, %s, %s) in %+v", stage, action, detail, events)
}
