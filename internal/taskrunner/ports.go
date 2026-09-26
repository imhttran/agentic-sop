package taskrunner

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// TaskStore loads and persists authoritative task state.
type TaskStore interface {
	Get(id string) (*domain.Task, error)
	Save(task *domain.Task) error
}

// Git is the narrow slice of the Git adapter the runner needs.
type Git interface {
	ValidateRepository(ctx context.Context) error
	CreateBranch(ctx context.Context, branch string) error
}

// WorkApplier applies agent-produced work to the working tree. The generic
// agent contract is content-generation only, so applying that content is an
// explicit, controlled step owned by the caller.
type WorkApplier interface {
	Apply(ctx context.Context, work agent.Response) error
}

// Verification is the deterministic verification boundary (backed by T008).
type Verification interface {
	Run(ctx context.Context, check testrunner.Check) testrunner.Result
	RunAll(ctx context.Context, checks []testrunner.Check) testrunner.SuiteResult
}
