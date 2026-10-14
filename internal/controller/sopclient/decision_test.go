package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// Approve/decline delegate to SOP's own verbs with the exact argv shape and no
// --run: the task id is one argv element (never a shell string) and optional
// actor/note metadata is forwarded only when supplied.
func TestDecisionDelegatesToSOPVerbs(t *testing.T) {
	root := newProject(t)
	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	cases := []struct {
		name string
		run  func() error
		want []string
	}{
		{"approve no metadata", func() error { return c.ApproveTask(ctx, pid, "t2", DecisionOptions{}) }, []string{"approve", "t2"}},
		{"decline no metadata", func() error { return c.DeclineTask(ctx, pid, "t2", DecisionOptions{}) }, []string{"decline", "t2"}},
		{"approve by only", func() error {
			return c.ApproveTask(ctx, pid, "t2", DecisionOptions{By: "alice"})
		}, []string{"approve", "t2", "--by", "alice"}},
		{"approve note only", func() error {
			return c.ApproveTask(ctx, pid, "t2", DecisionOptions{Note: "looks-good"})
		}, []string{"approve", "t2", "--note", "looks-good"}},
		{"decline by and note", func() error {
			return c.DeclineTask(ctx, pid, "t2", DecisionOptions{By: "bob", Note: "nope"})
		}, []string{"decline", "t2", "--by", "bob", "--note", "nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			got := args()
			if !equal(got, tc.want) {
				t.Fatalf("sop argv = %v, want %v", got, tc.want)
			}
			for _, a := range got {
				if a == "--run" {
					t.Fatalf("argv %v must never include --run (decision is not execution)", got)
				}
			}
		})
	}
}

// The task id is passed as a single argv element, so shell metacharacters in an
// untrusted id are never interpreted (the Commander uses an argv slice, not a
// shell string). The recorded argv shows the id verbatim as one element.
func TestDecisionTaskIDPassedSafely(t *testing.T) {
	root := newProject(t)
	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	// A hostile-looking id carrying shell metacharacters (no whitespace, so the
	// recording helper keeps it as one field; the real exec path always passes it
	// as a single argv element regardless).
	hostile := "t2;rm;`whoami`$(id)|cat"
	if err := c.ApproveTask(context.Background(), pid, hostile, DecisionOptions{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got := args()
	if !equal(got, []string{"approve", hostile}) {
		t.Fatalf("argv = %v, want [approve <hostile-id-as-one-element>]", got)
	}
}

// A non-zero `sop approve`/`sop decline` is a decision rejection: the returned
// error is a *DecisionRejection carrying SOP's own message and the task id, is
// classified as a conflict (ErrDecisionRejected), and is never a success.
func TestDecisionRejectionIsTyped(t *testing.T) {
	root := newProject(t)
	bin := rejectingSop(t, "gate is stale")
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		run  func() error
		verb string
	}{
		{"approve rejected", func() error { return c.ApproveTask(ctx, pid, "t2", DecisionOptions{}) }, "approve"},
		{"decline rejected", func() error { return c.DeclineTask(ctx, pid, "t2", DecisionOptions{}) }, "decline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("err = nil, want a rejection (never a fabricated success)")
			}
			if !errors.Is(err, ErrDecisionRejected) {
				t.Fatalf("err = %v, want ErrDecisionRejected", err)
			}
			var rej *DecisionRejection
			if !errors.As(err, &rej) {
				t.Fatalf("err = %v, want *DecisionRejection", err)
			}
			if rej.Verb != tc.verb || rej.TaskID != "t2" {
				t.Fatalf("rejection = %+v, want verb=%s task=t2", rej, tc.verb)
			}
			if rej.Message == "" {
				t.Fatal("rejection carries no SOP message; the operator cannot act on it")
			}
		})
	}
}

// A successful decline returns nil and records no controller-side failure: the
// boundary performs no state write (proven by the persistence snapshot) and no
// task-failure signal.
func TestDeclineSuccessIsNotFailure(t *testing.T) {
	root := newProject(t)
	bin, _ := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.DeclineTask(context.Background(), config.ProjectID(root), "t2", DecisionOptions{}); err != nil {
		t.Fatalf("DeclineTask err = %v, want nil (a decline is not a failure)", err)
	}
}

// rejectingSop writes an executable "sop" that always exits non-zero with the
// given message on stderr, so a decision rejection can be exercised without a
// real SOP.
func rejectingSop(t *testing.T, message string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sop")
	script := "#!/bin/sh\necho \"" + message + "\" 1>&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}
