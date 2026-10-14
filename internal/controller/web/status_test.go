package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The task status panel renders SOP's persisted aggregate status: run stage,
// validation, review, quality/JEV, and report presence.
func TestTaskStatusPanelRenders(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"state.json":      `{"id":"x1","stage":"VALIDATING"}`,
		"validation.json": `{"Status":"PASS","Results":[{"Category":"BUILD","Command":"go build ./...","ExitCode":0,"Status":"PASS"}]}`,
		"review.json":     `{"Summary":"looks fine","Findings":[]}`,
		"report.json":     `{"id":"x1","stage":"PASSED","decision":"PASS","jev":{"status":"PASS","findings":[]}}`,
	})
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	for _, want := range []string{"Status", "Validation", "Review", "Quality / JEV", "Report", "VALIDATING", "not a pass"} {
		if !strings.Contains(body, want) {
			t.Errorf("status panel missing %q", want)
		}
	}
	if !strings.Contains(body, `s-completed">PASS`) {
		t.Errorf("expected an explicit PASS rendering for persisted evidence")
	}
}

// With no SOP artifacts every status is UNKNOWN, and nothing renders as PASS.
func TestTaskStatusUnknownIsNotPass(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	if !strings.Contains(body, "UNKNOWN") {
		t.Errorf("expected explicit UNKNOWN for a task with no artifacts")
	}
	if strings.Contains(body, `s-completed">PASS`) {
		t.Errorf("a missing artifact must never render as PASS")
	}
}

// The project hero shows the recorded plan source and the active task, both
// derived from SOP-persisted state.
func TestProjectShowsPlanAndActiveTask(t *testing.T) {
	srv, id, root := newRunServer(t, "IMPLEMENTING", map[string]string{
		"state.json": `{"id":"x1","stage":"IMPLEMENTING"}`,
	})
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md","plan_id":"plan"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("project page: %d", code)
	}
	// The template formatter wraps long expressions across lines, so compare on
	// whitespace-collapsed text.
	flat := strings.Join(strings.Fields(body), " ")
	for _, want := range []string{"plan docs/PLAN.md", "active x1", "/tasks/x1"} {
		if !strings.Contains(flat, want) {
			t.Errorf("project hero missing %q", want)
		}
	}
}
