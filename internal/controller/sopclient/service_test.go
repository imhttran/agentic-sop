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
func TestProjectsErrorNamesProject(t *testing.T) {
	root := newProject(t)
	seed(t, root, `DROP TABLE tasks`)
	bin, _ := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Projects(context.Background())
	if err == nil {
		t.Fatal("err = nil, want the summary read to fail")
	}
	if !strings.Contains(err.Error(), config.ProjectID(root)) {
		t.Errorf("err = %v, want the failing project id", err)
	}
}
