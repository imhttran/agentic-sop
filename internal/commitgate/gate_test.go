package commitgate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeCommitter struct {
	messages []string
	err      error
}

func (f *fakeCommitter) Commit(_ context.Context, message string) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	return nil
}

func ready() Preconditions {
	return Preconditions{TestsPassed: true, ReviewPassed: true, DocsUpdated: true}
}

func TestCommitWhenAllPreconditionsHold(t *testing.T) {
	c := &fakeCommitter{}
	if err := New(c).Commit(context.Background(), "T013", "Commit gate", ready()); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if len(c.messages) != 1 || c.messages[0] != "task(T013): implement Commit gate" {
		t.Errorf("messages = %v, want [task(T013): implement Commit gate]", c.messages)
	}
}

func TestCommitBlockedWhenPreconditionUnmet(t *testing.T) {
	cases := map[string]Preconditions{
		"tests":  {ReviewPassed: true, DocsUpdated: true},
		"review": {TestsPassed: true, DocsUpdated: true},
		"docs":   {TestsPassed: true, ReviewPassed: true},
	}
	for name, pre := range cases {
		t.Run(name, func(t *testing.T) {
			c := &fakeCommitter{}
			err := New(c).Commit(context.Background(), "T013", "Commit gate", pre)
			if err == nil {
				t.Fatal("expected a commit-blocked error")
			}
			if len(c.messages) != 0 {
				t.Errorf("commit happened despite an unmet precondition: %v", c.messages)
			}
		})
	}
}

func TestCommitRejectsBlankIdentity(t *testing.T) {
	c := &fakeCommitter{}
	if err := New(c).Commit(context.Background(), "", "title", ready()); err == nil {
		t.Error("expected error for blank task id")
	}
	if err := New(c).Commit(context.Background(), "T013", "  ", ready()); err == nil {
		t.Error("expected error for blank title")
	}
	if len(c.messages) != 0 {
		t.Errorf("no commit expected, got %v", c.messages)
	}
}

func TestCommitPropagatesCommitterError(t *testing.T) {
	c := &fakeCommitter{err: errors.New("git failed")}
	err := New(c).Commit(context.Background(), "T013", "Commit gate", ready())
	if err == nil || !strings.Contains(err.Error(), "git failed") {
		t.Errorf("err = %v, want the committer error", err)
	}
}

func TestMessageFormat(t *testing.T) {
	if got := Message("T009", "Add thing"); got != "task(T009): implement Add thing" {
		t.Errorf("Message = %q", got)
	}
	if got := Message(" T009 ", " Add thing "); got != "task(T009): implement Add thing" {
		t.Errorf("Message with padding = %q", got)
	}
}
