package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
)

func TestRenderActivityLine(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		event activity.Event
		want  string
	}{
		{
			name:  "discover read",
			event: activity.Event{Stage: activity.StageDiscover, Action: "reading", Detail: "internal/run/jev.go", Timestamp: start.Add(3 * time.Second)},
			want:  "[00:03] DISCOVER   reading internal/run/jev.go",
		},
		{
			name:  "change edit",
			event: activity.Event{Stage: activity.StageChange, Action: "editing", Detail: "internal/run/jev.go", Timestamp: start.Add(9 * time.Second)},
			want:  "[00:09] CHANGE     editing internal/run/jev.go",
		},
		{
			name:  "quality pass",
			event: activity.Event{Stage: activity.StageQuality, Action: "PASS", Timestamp: start.Add(38 * time.Second)},
			want:  "[00:38] QUALITY    PASS",
		},
		{
			name:  "negative elapsed clamps",
			event: activity.Event{Stage: activity.StageStart, Action: "x", Timestamp: start.Add(-time.Second)},
			want:  "[00:00] START      x",
		},
		{
			name:  "over a minute",
			event: activity.Event{Stage: activity.StageValidate, Action: "go test ./...", Timestamp: start.Add(72*time.Second + 5*time.Second)},
			want:  "[01:17] VALIDATE   go test ./...",
		},
		{
			name:  "multi-line action collapses",
			event: activity.Event{Stage: activity.StageValidate, Action: "go test ./...\nrm -rf /", Timestamp: start},
			want:  "[00:00] VALIDATE   go test ./...",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderActivityLine(tc.event, start); got != tc.want {
				t.Errorf("renderActivityLine = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestActivityEnabledHonorsEnv(t *testing.T) {
	var buf bytes.Buffer

	t.Setenv(envActivity, "on")
	if !activityEnabled(&buf) {
		t.Error("SOP_ACTIVITY=on did not enable activity on a non-terminal writer")
	}
	t.Setenv(envActivity, "off")
	if activityEnabled(&buf) {
		t.Error("SOP_ACTIVITY=off did not disable activity")
	}
	// An unrecognized value falls back to terminal detection, which is false for a
	// buffer: non-interactive runs stay silent.
	t.Setenv(envActivity, "nonsense")
	if activityEnabled(&buf) {
		t.Error("non-terminal writer must default to disabled")
	}
}

func TestTaskActivityContextDisabled(t *testing.T) {
	t.Setenv(envActivity, "off")
	ctx := taskActivityContext(context.Background(), t.TempDir(), &bytes.Buffer{}, "TASK", "title")
	if activity.FromContext(ctx) != nil {
		t.Error("taskActivityContext registered a recorder while disabled")
	}
}

func TestTaskActivityContextPersistsArtifact(t *testing.T) {
	t.Setenv(envActivity, "1")
	dir := t.TempDir()
	ctx := taskActivityContext(context.Background(), dir, &bytes.Buffer{}, "TASK", "title")
	rec := activity.FromContext(ctx)
	if !rec.Enabled() {
		t.Fatal("taskActivityContext did not register a recorder")
	}
	rec.Emit(activity.StageChange, "editing", "a.go")

	data, err := os.ReadFile(filepath.Join(dir, activityArtifactName))
	if err != nil {
		t.Fatalf("activity artifact not written: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("artifact lines = %d, want 2 (start + edit):\n%s", len(lines), data)
	}
	var got activity.Event
	if err := json.Unmarshal([]byte(lines[1]), &got); err != nil {
		t.Fatalf("artifact line not JSON: %v\n%s", err, lines[1])
	}
	if got.TaskID != "TASK" || got.Stage != activity.StageChange || got.Action != "editing" || got.Detail != "a.go" {
		t.Errorf("artifact event = %+v", got)
	}
}

func TestActivityStreamSerializesConcurrentEmits(t *testing.T) {
	var buf bytes.Buffer
	s := newActivityStream(&buf, time.Now())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Emit(activity.Event{Stage: activity.StageChange, Action: "editing", Detail: "a.go", Timestamp: time.Now()})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 50 {
		t.Fatalf("lines = %d, want 50", len(lines))
	}
	for _, line := range lines {
		if !strings.HasSuffix(line, "CHANGE     editing a.go") {
			t.Errorf("interleaved line: %q", line)
		}
	}
}

func TestRunStreamsActivityWhenEnabled(t *testing.T) {
	t.Setenv(envActivity, "1")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	for _, want := range []string{"START", "PLAN", "IMPLEMENT", "VALIDATE", "REVIEW", "QUALITY", "COMPLETE"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("activity output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunActivitySilentWhenDisabled(t *testing.T) {
	t.Setenv(envActivity, "off")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "[00:") {
		t.Errorf("activity was rendered while disabled:\n%s", stdout)
	}
}
