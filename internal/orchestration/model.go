// Package orchestration defines a provider/model-neutral domain model for
// orchestrating work across the SOP lifecycle:
//
//	Task -> Orchestrator -> Workers -> Integrator -> Reviewer -> Verification
//
// plus the deterministic SINGLE / SEQUENTIAL / PARALLEL execution decision.
//
// # SINGLE-LIFECYCLE-AUTHORITY INVARIANT
//
// Exactly one authoritative SOP lifecycle lives in internal/domain. The
// orchestration package observes lifecycle state and reads domain values
// (Task, TaskStatus, BlockedReason, ExecutionMode, dependency resolution), but
// it must NEVER define states, transitions, completion predicates, or any other
// lifecycle vocabulary, and it must never mutate a domain.Task. Every state
// change is delegated to domain.Task (Transition/Block/Requeue/...).
//
// The model depends on no specific provider or model: it references only
// agent.Capability values (plain deterministic strings) and never a concrete
// provider implementation (openai/ollama/mlx/openaicompatible) or transport.
package orchestration

import (
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// Role identifies one of the canonical orchestration roles. It is a plain
// provider/model-neutral label and defines no lifecycle state.
type Role string

const (
	RoleOrchestrator Role = "orchestrator"
	RoleWorker       Role = "worker"
	RoleIntegrator   Role = "integrator"
	RoleReviewer     Role = "reviewer"
	RoleVerification Role = "verification"
)

// WorkUnit is one unit of independent work the orchestrator may delegate: a
// single capability to be performed against a single domain task. It carries
// only domain/agent values (task ID, capability) and holds no provider, model,
// transport, or filesystem handle.
type WorkUnit struct {
	// TaskID is the domain.Task this unit works on.
	TaskID string
	// Capability is the deterministic work requested for the task.
	Capability agent.Capability
}

// Orchestrator selects a set of WorkUnits for a requested capability and
// delegates them. It is the head of the model; it never mutates lifecycle state
// itself.
type Orchestrator struct {
	// Role is the orchestration role identity. It is a plain label, not a
	// provider or model.
	Role Role
}

// Worker performs a single capability on a single task. It is an interface so
// that any provider-neutral agent.Agent may back it without the orchestration
// model depending on a provider.
type Worker interface {
	// Work returns the unit this worker is responsible for.
	Work() WorkUnit
}

// Integrator combines the results of multiple workers into a single coherent
// view (for example a merged diff or a consolidated report). It carries no
// lifecycle authority.
type Integrator struct {
	Role Role
	// Unit is the integration target, expressed as domain/agent values only.
	Unit WorkUnit
}

// Reviewer performs a post-integration review step. It is distinct from
// Verification: a reviewer exercises judgement over the integrated result,
// while verification runs deterministic checks.
type Reviewer struct {
	Role Role
	Unit WorkUnit
}

// Verification runs the deterministic post-step that proves the integrated
// result meets the caller-owned contract. It observes domain state; it does not
// mutate it.
type Verification struct {
	Role Role
	Unit WorkUnit
	// AcceptanceCriteria and ValidationCommands are the caller-owned contract the
	// verification step checks. They are task data, never model output.
	AcceptanceCriteria []string
	ValidationCommands []string
}

// Model is the canonical orchestration domain model. It is built entirely from
// domain/agent values and holds no provider, model, transport, or filesystem
// handle.
type Model struct {
	Orchestrator Orchestrator
	Workers      []WorkUnit
	Integrator   Integrator
	Reviewer     Reviewer
	Verification Verification
}

// TaskView is a read-only, provider-neutral projection of a domain task. It lets
// the orchestration model reason about task metadata without depending on the
// mutable domain.Task. All lifecycle queries delegate to the underlying
// domain.Task so orchestration never re-implements a state predicate.
type TaskView struct {
	// ID, DependencyIDs, and ExecutionMode are read from the domain task.
	ID            string
	DependencyIDs []string
	ExecutionMode domain.ExecutionMode
	// task is the authoritative domain task. Lifecycle questions are delegated to
	// it; orchestration never assigns to it.
	task *domain.Task
}

// NewTaskView projects a domain task into a read-only orchestration view. It
// keeps a reference to the authoritative task so state queries delegate to
// internal/domain.
func NewTaskView(task *domain.Task) TaskView {
	if task == nil {
		return TaskView{}
	}
	deps := make([]string, len(task.DependencyIDs))
	copy(deps, task.DependencyIDs)
	return TaskView{
		ID:            task.ID,
		DependencyIDs: deps,
		ExecutionMode: task.ExecutionMode,
		task:          task,
	}
}

// Status delegates to the authoritative domain lifecycle vocabulary. It never
// defines its own status type.
func (v TaskView) Status() domain.TaskStatus {
	if v.task == nil {
		return ""
	}
	return v.task.Status
}

// IsSatisfied delegates to the domain's single definition of "complete".
func (v TaskView) IsSatisfied() bool {
	if v.task == nil {
		return false
	}
	return v.task.IsSatisfied()
}

// IsRunnable delegates to the domain's single definition of "may be selected".
func (v TaskView) IsRunnable() bool {
	if v.task == nil {
		return false
	}
	return v.task.IsRunnable()
}

// ResolveDependencies delegates dependency resolution to the domain.
func (v TaskView) ResolveDependencies(tasks map[string]*domain.Task) (unmet []domain.Dependency, satisfied bool) {
	if v.task == nil {
		return []domain.Dependency{}, len(v.DependencyIDs) == 0
	}
	return v.task.ResolveDependencies(tasks)
}

// NewModel assembles the canonical orchestration model from domain/agent
// values. It performs no I/O, consults no provider, and mutates no lifecycle
// state.
func NewModel(units []WorkUnit, acceptance []string, commands []string) Model {
	workers := make([]WorkUnit, len(units))
	copy(workers, units)

	var head WorkUnit
	if len(workers) > 0 {
		head = workers[0]
	}
	return Model{
		Orchestrator: Orchestrator{Role: RoleOrchestrator},
		Workers:      workers,
		Integrator:   Integrator{Role: RoleIntegrator, Unit: head},
		Reviewer:     Reviewer{Role: RoleReviewer, Unit: head},
		Verification: Verification{
			Role:               RoleVerification,
			Unit:               head,
			AcceptanceCriteria: append([]string(nil), acceptance...),
			ValidationCommands: append([]string(nil), commands...),
		},
	}
}
