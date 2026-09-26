// Package taskrunner orchestrates a single READY task through a local TDD
// cycle: create a branch, design/apply tests, prove RED, implement, prove
// GREEN, and run the required local suite. It composes the deterministic
// adapters (Git, agent, verification, store); those adapters report facts, and
// the runner owns workflow control.
package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sdlc/internal/agent"
	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/git"
	"github.com/imhttran/agentic-sdlc/internal/testrunner"
)

// Outcome is the terminal outcome of a task run.
type Outcome string

const (
	// LocalTestsPass: the task reached LOCAL_TESTS_PASS.
	LocalTestsPass Outcome = "LOCAL_TESTS_PASS"
	// BlockedOutcome: the task was blocked after bounded unsuccessful attempts.
	BlockedOutcome Outcome = "BLOCKED"
)

// Result reports the terminal outcome and the (durable) task state.
type Result struct {
	Task    *domain.Task
	Outcome Outcome
}

// Config is the small project-verification configuration T010 needs.
type Config struct {
	// TestCheck is the focused test command used to prove RED and GREEN.
	TestCheck testrunner.Check
	// FinalChecks is the required local suite run before LOCAL_TESTS_PASS.
	FinalChecks []testrunner.Check
	// Context is optional bounded project context included in agent requests.
	Context string
}

// Runner executes one READY task.
type Runner struct {
	store  TaskStore
	git    Git
	agent  agent.Agent
	apply  WorkApplier
	verify Verification
	config Config
}

// New builds a Runner from its ports and configuration.
func New(store TaskStore, g Git, a agent.Agent, applier WorkApplier, v Verification, cfg Config) *Runner {
	return &Runner{store: store, git: g, agent: a, apply: applier, verify: v, config: cfg}
}

// Run executes the READY task identified by taskID. It returns a Result and an
// error: a BLOCKED outcome is a determinate result (err == nil); operational
// failures (wrong initial state, adapter failure, persistence inconsistency,
// cancellation) return an error.
func (r *Runner) Run(ctx context.Context, taskID string) (Result, error) {
	task, err := r.loadReady(ctx, taskID)
	if err != nil {
		return Result{}, err
	}

	err = r.execute(ctx, task)
	if err == nil {
		return Result{Task: task, Outcome: LocalTestsPass}, nil
	}

	var blocked *blockedError
	if errors.As(err, &blocked) {
		return Result{Task: task, Outcome: BlockedOutcome}, nil
	}
	return Result{Task: task}, err
}

func (r *Runner) loadReady(ctx context.Context, taskID string) (*domain.Task, error) {
	task, err := r.store.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("load task %s: %w", taskID, err)
	}
	if task.Status != domain.READY {
		return nil, fmt.Errorf("task %s is %s, want READY", taskID, task.Status)
	}
	if strings.TrimSpace(r.config.TestCheck.Command) == "" {
		return nil, fmt.Errorf("task %s: no test check configured", taskID)
	}
	if err := r.git.ValidateRepository(ctx); err != nil {
		return nil, fmt.Errorf("validate repository: %w", err)
	}
	return task, nil
}

func (r *Runner) execute(ctx context.Context, task *domain.Task) error {
	branch, err := git.TaskBranchName(task.ID, task.Title)
	if err != nil {
		return err
	}
	if err := r.git.CreateBranch(ctx, branch); err != nil {
		// The task stays READY.
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	if err := r.transitionAndSave(task, domain.BRANCH_CREATED); err != nil {
		// Git branch creation cannot be rolled back safely.
		return fmt.Errorf("branch %s created but persisting BRANCH_CREATED failed: %w", branch, err)
	}

	testDesign, err := r.designAndApplyTests(ctx, task)
	if err != nil {
		return err
	}
	if err := r.transitionAndSave(task, domain.TESTS_WRITTEN); err != nil {
		return err
	}

	testDesign, err = r.verifyRED(ctx, task, testDesign)
	if err != nil {
		return err
	}
	return r.implementUntilGreen(ctx, task, testDesign)
}

// verifyRED runs the focused test until it deterministically FAILs (valid RED),
// applying bounded DESIGN_TESTS corrections when tests unexpectedly pass.
func (r *Runner) verifyRED(ctx context.Context, task *domain.Task, testDesign string) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return testDesign, err
		}

		result := r.verify.Run(ctx, r.config.TestCheck)
		switch result.Status {
		case testrunner.Fail:
			return testDesign, r.transitionAndSave(task, domain.RED_VERIFIED)
		case testrunner.Canceled:
			return testDesign, ctxErr(ctx)
		case testrunner.Error:
			return testDesign, fmt.Errorf("red verification infrastructure error: %s", truncate(result.Stderr))
		default: // testrunner.Pass: not valid RED
			if !task.CanRetry() {
				return testDesign, r.block(task, domain.TEST_DESIGN_FAILED)
			}
			if err := r.saveAttempt(ctx, task, domain.TESTS_WRITTEN,
				"tests passed before implementation; correcting test design",
				result.Stdout+result.Stderr, result.Duration); err != nil {
				return testDesign, err
			}
			corrected, err := r.designAndApplyTests(ctx, task)
			if err != nil {
				return testDesign, err
			}
			testDesign = corrected
		}
	}
}

func (r *Runner) implementUntilGreen(ctx context.Context, task *domain.Task, testDesign string) error {
	if err := r.transitionAndSave(task, domain.IMPLEMENTING); err != nil {
		return err
	}

	var evidence failureEvidence
	for cycle := 0; ; cycle++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The retry budget is shared across RED correction and implementation
		// (documented V1 policy); it is owned by the domain attempt model.
		if !task.CanRetry() {
			return r.block(task, domain.RETRIES_EXHAUSTED)
		}

		if cycle == 0 {
			if err := r.applyCapability(ctx, agent.Implement, implementRequest(task, testDesign, r.config.Context)); err != nil {
				return err
			}
		} else {
			diagnosis, err := r.diagnose(ctx, task, evidence)
			if err != nil {
				return err
			}
			if err := r.applyCapability(ctx, agent.Fix, fixRequest(task, diagnosis, evidence, r.config.Context)); err != nil {
				return err
			}
		}

		focused := r.verify.Run(ctx, r.config.TestCheck)
		switch focused.Status {
		case testrunner.Canceled:
			return ctxErr(ctx)
		case testrunner.Error:
			return fmt.Errorf("focused verification infrastructure error: %s", truncate(focused.Stderr))
		case testrunner.Fail:
			if err := r.saveAttempt(ctx, task, domain.FIX_REQUIRED, "focused tests failed",
				focused.Stdout+focused.Stderr, focused.Duration); err != nil {
				return err
			}
			evidence = evidenceFrom(r.config.TestCheck, focused)
			continue
		}

		// Focused test passed: run the required final suite.
		suite := r.verify.RunAll(ctx, r.config.FinalChecks)
		switch suite.Status {
		case testrunner.Canceled:
			return ctxErr(ctx)
		case testrunner.Error:
			return fmt.Errorf("final suite infrastructure error: %s", evidenceFromSuite(suite).String())
		case testrunner.Pass:
			if err := r.saveAttempt(ctx, task, domain.LOCAL_TESTS_PASS,
				"focused and final verification passed", "", 0); err != nil {
				return err
			}
			return r.transitionAndSave(task, domain.LOCAL_TESTS_PASS)
		default: // suite FAIL
			if err := r.saveAttempt(ctx, task, domain.FIX_REQUIRED, "required local suite failed",
				suiteSummary(suite), 0); err != nil {
				return err
			}
			evidence = evidenceFromSuite(suite)
		}
	}
}

func (r *Runner) designAndApplyTests(ctx context.Context, task *domain.Task) (string, error) {
	resp, err := r.agent.Generate(ctx, designTestsRequest(task, r.config.Context))
	if err != nil {
		return "", fmt.Errorf("agent DESIGN_TESTS: %w", err)
	}
	if err := r.apply.Apply(ctx, resp); err != nil {
		return "", fmt.Errorf("apply test work: %w", err)
	}
	return resp.Content, nil
}

func (r *Runner) diagnose(ctx context.Context, task *domain.Task, evidence failureEvidence) (string, error) {
	resp, err := r.agent.Generate(ctx, diagnoseRequest(task, evidence, r.config.Context))
	if err != nil {
		return "", fmt.Errorf("agent DIAGNOSE_FAILURE: %w", err)
	}
	return resp.Content, nil
}

func (r *Runner) applyCapability(ctx context.Context, capability agent.Capability, request agent.Request) error {
	resp, err := r.agent.Generate(ctx, request)
	if err != nil {
		return fmt.Errorf("agent %s: %w", capability, err)
	}
	if err := r.apply.Apply(ctx, resp); err != nil {
		return fmt.Errorf("apply %s work: %w", capability, err)
	}
	return nil
}

// transitionAndSave stages a state transition on a copy and persists it,
// publishing the copy only after persistence succeeds.
func (r *Runner) transitionAndSave(task *domain.Task, next domain.TaskStatus) error {
	staged := clone(task)
	if err := staged.Transition(next); err != nil {
		return err
	}
	if err := r.store.Save(staged); err != nil {
		return err
	}
	*task = *staged
	return nil
}

// saveAttempt records an attempt on a copy and persists it, publishing only on
// success.
func (r *Runner) saveAttempt(ctx context.Context, task *domain.Task, status domain.TaskStatus, reason, output string, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	staged := clone(task)
	staged.AddAttempt(status, reason)
	if n := len(staged.Attempts); n > 0 {
		staged.Attempts[n-1].Output = truncate(output)
		staged.Attempts[n-1].Duration = duration
	}
	if err := r.store.Save(staged); err != nil {
		return err
	}
	*task = *staged
	return nil
}

func (r *Runner) block(task *domain.Task, reason domain.BlockedReason) error {
	staged := clone(task)
	if err := staged.Block(reason); err != nil {
		return err
	}
	if err := r.store.Save(staged); err != nil {
		return err
	}
	*task = *staged
	return &blockedError{reason: reason}
}

// blockedError signals a determinate BLOCKED outcome (not an operational error).
type blockedError struct {
	reason domain.BlockedReason
}

func (e *blockedError) Error() string { return "task blocked: " + string(e.reason) }

func ctxErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("operation canceled")
}

func clone(task *domain.Task) *domain.Task {
	c := *task
	c.DependencyIDs = append([]string{}, task.DependencyIDs...)
	c.Attempts = append([]domain.Attempt{}, task.Attempts...)
	return &c
}
