package sopclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// An approvals failure keeps its cause in the chain: callers can still classify
// a timeout even though the error also reports the unavailable listing.
func TestApprovalsErrorKeepsCause(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, slowSop(t, "5"), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Approvals(context.Background(), config.ProjectID(root))
	if err == nil {
		t.Fatal("err = nil, want a timeout")
	}
	if !errors.Is(err, ErrApprovalsUnavailable) {
		t.Errorf("err = %v, want ErrApprovalsUnavailable", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the timeout cause preserved", err)
	}
}

// A SOP refusal stays a *CommandError through the approvals wrap, so callers can
// still see SOP's exit status and diagnostic.
func TestApprovalsErrorKeepsRefusal(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, rejectingSop(t, "no gate"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Approvals(context.Background(), config.ProjectID(root))
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v (%T), want *CommandError in the chain", err, err)
	}
	if !errors.Is(err, ErrApprovalsUnavailable) {
		t.Errorf("err = %v, want ErrApprovalsUnavailable", err)
	}
}

// A missing project names the id while staying matchable as ErrProjectNotFound.
func TestProjectNotFoundNamesProject(t *testing.T) {
	root := newProject(t)
	bin, _ := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()

	cases := []struct {
		name string
		run  func() error
	}{
		{"Project", func() error { _, err := c.Project(ctx, "nope"); return err }},
		{"Task", func() error { _, err := c.Task(ctx, "nope", "t1"); return err }},
		{"ChangedTasks", func() error { _, err := c.ChangedTasks(ctx, "nope"); return err }},
		{"Run", func() error { return c.Run(ctx, "nope") }},
		{"ApproveTask", func() error { return c.ApproveTask(ctx, "nope", "t1", DecisionOptions{}) }},
		{"DeclineTask", func() error { return c.DeclineTask(ctx, "nope", "t1", DecisionOptions{}) }},
		{"AcceptChangedTask", func() error { return c.AcceptChangedTask(ctx, "nope", "t1") }},
		{"ActivityWindow", func() error { _, err := c.ActivityWindow(ctx, "nope", "", 10); return err }},
		{"TaskActivityView", func() error { _, err := c.TaskActivityView(ctx, "nope", "t1"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if !errors.Is(err, ErrProjectNotFound) {
				t.Fatalf("err = %v, want ErrProjectNotFound", err)
			}
			if !strings.Contains(err.Error(), "nope") {
				t.Errorf("err = %v, want the missing project id in the message", err)
			}
		})
	}
}

// A per-project summary failure is attributed to the project instead of aborting
// the list anonymously.
func TestProjectsErrorStaysOnSummary(t *testing.T) {
	root := newProject(t)
	seed(t, root, `DROP TABLE tasks`)
	bin, _ := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	list, err := c.Projects(context.Background())
	if err != nil {
		t.Fatalf("Projects err = %v, want nil (a per-project failure must not abort the list)", err)
	}
	if len(list) != 1 {
		t.Fatalf("Projects returned %d summaries, want 1", len(list))
	}
	if list[0].StateError == "" {
		t.Fatal("StateError empty; a failed read must be reported on the summary")
	}
	if !strings.Contains(list[0].StateError, config.ProjectID(root)) {
		t.Errorf("StateError = %q, want the failing project id", list[0].StateError)
	}
	if list[0].Name == "" {
		t.Error("Name empty; the failed project must stay identifiable")
	}
}

// One unreadable project must not hide the healthy ones.
func TestProjectsKeepsHealthyProjectsWhenOneFails(t *testing.T) {
	bad := t.TempDir()
	initProjectAt(t, bad, "bad")
	seed(t, bad, `DROP TABLE tasks`)
	good := t.TempDir()
	initProjectAt(t, good, "good")
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, good, `INSERT INTO tasks VALUES ('t1','T','o','a','PLANNED','',0,3,'`+now+`','`+now+`')`)

	bin, _ := fakeSop(t)
	c, err := New([]string{bad, good}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	list, err := c.Projects(context.Background())
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("Projects returned %d summaries, want 2", len(list))
	}
	seen := map[string]ProjectSummary{}
	for _, s := range list {
		seen[s.ID] = s
	}
	badSum, ok := seen[config.ProjectID(bad)]
	if !ok {
		t.Fatal("failed project missing from the list")
	}
	if badSum.StateError == "" {
		t.Error("StateError empty for the unreadable project")
	}
	goodSum, ok := seen[config.ProjectID(good)]
	if !ok {
		t.Fatal("healthy project missing from the list")
	}
	if goodSum.StateError != "" {
		t.Errorf("StateError = %q for the healthy project, want empty", goodSum.StateError)
	}
	if goodSum.Total != 1 {
		t.Errorf("healthy project Total = %d, want 1", goodSum.Total)
	}
}
