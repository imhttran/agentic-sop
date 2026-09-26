// Package commitgate decides whether a task commit is allowed and, if so,
// creates it through a committer port. It owns the commit policy only; the Git
// adapter performs the actual commit and never force-commits or bypasses hooks.
package commitgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Preconditions are the conditions that must hold before a task commit.
type Preconditions struct {
	TestsPassed  bool
	ReviewPassed bool
	DocsUpdated  bool
}

func (p Preconditions) unmet() []string {
	var unmet []string
	if !p.TestsPassed {
		unmet = append(unmet, "required tests have not passed")
	}
	if !p.ReviewPassed {
		unmet = append(unmet, "required review has not passed")
	}
	if !p.DocsUpdated {
		unmet = append(unmet, "task documentation has not been updated")
	}
	return unmet
}

// Message builds the deterministic, task-scoped commit message.
func Message(taskID, title string) string {
	return fmt.Sprintf("task(%s): implement %s", strings.TrimSpace(taskID), strings.TrimSpace(title))
}

// Committer creates the task commit.
type Committer interface {
	Commit(ctx context.Context, message string) error
}

// Gate enforces the preconditions and creates the commit.
type Gate struct {
	committer Committer
}

// New returns a Gate that commits through c.
func New(c Committer) *Gate { return &Gate{committer: c} }

// Commit creates the task commit when every required condition holds. Otherwise
// it returns an error and performs no commit.
func (g *Gate) Commit(ctx context.Context, taskID, title string, pre Preconditions) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(title) == "" {
		return errors.New("commit gate: task id and title must not be empty")
	}
	if unmet := pre.unmet(); len(unmet) > 0 {
		return fmt.Errorf("commit blocked: %s", strings.Join(unmet, "; "))
	}
	return g.committer.Commit(ctx, Message(taskID, title))
}
