package runtrace

// This file is the ORCH-009 orchestration-graph PRODUCER. The runtrace package
// stays observation-only: ComposeOrchestrationEvents derives the ordered
// orchestration event list from evidence the orchestration layer already owns
// (assignments, worker selections, model-class decisions, provider/model
// resolution, worker start/complete, result accept/reject, integration
// start/complete, verification results, and termination) and returns it for the
// harness to place on runtrace.Inputs.OrchestrationEvents before Build/Write.
//
// Nothing here reads the composed events back to drive a routing, lifecycle,
// retry, progress, approval, or termination decision. It is a pure, deterministic
// function of its input: no clock (timestamps come from the descriptor), no
// randomness, no I/O, and no live provider.
//
// REFERENCE-ONLY PAYLOAD DISCIPLINE
//
// Every composed event carries identifiers, enumerated labels, counts, and bounded
// short text only. Composition never receives credential material or raw
// prompt/completion bodies: the descriptor fields it consumes are IDs and labels.
// Values that flow into identifier-guarded positions are still run through the
// sanitizer, so a careless caller cannot persist a secret.

import (
	"sort"
	"time"
)

// WorkerRun captures the observable lifecycle of one worker assignment within a
// run: its selection, the model-class decision, the resolved executing target,
// start/complete, and the accept/reject verdict. Every field is an observation
// the orchestration layer already holds; none of them is a decision the trace
// makes.
type WorkerRun struct {
	// AssignmentID and TaskID are the caller-owned correlation IDs.
	AssignmentID string
	TaskID       string
	// WorkerID identifies the selected worker.
	WorkerID string
	// ModelClass is the routing DECISION class (never rewritten by a fallback).
	ModelClass string
	// RoutingSource records what selected the class.
	RoutingSource string
	// Provider, Model, and Locality are the resolved EXECUTING target.
	Provider string
	Model    string
	Locality string
	// Started, Completed, and Timestamp order the worker lifecycle observations.
	Started   time.Time
	Completed time.Time
	// Accepted reports whether the harness accepted the worker's result claim.
	Accepted bool
	// AcceptedReason and RejectedReason are bounded, single-line explanations. They
	// MUST NOT carry secrets or raw prompt/completion content.
	AcceptedReason string
	RejectedReason string
}

// IntegrationRun captures one INTEGRATE stage pass observed for a run.
type IntegrationRun struct {
	// TaskID is the integration target.
	TaskID string
	// Inputs are the worker-result references (assignment IDs) this pass consumed.
	Inputs []string
	// Output is the integration-result reference produced (never a verdict).
	Output string
	// Started and Completed order the pass.
	Started   time.Time
	Completed time.Time
	// Reason is a bounded single-line explanation of the integration outcome.
	Reason string
}

// VerificationRun captures one deterministic verification check observed for a
// run.
type VerificationRun struct {
	// TaskID is the verification target, when any.
	TaskID string
	// Command names the deterministic check performed (a reference, never output).
	Command string
	// Status is the enumerated check status label (pass, fail, skipped).
	Status string
	// Timestamp is the observation time.
	Timestamp time.Time
}

// TerminationRun captures the run's end-state observation.
type TerminationRun struct {
	// TaskID is the terminating task, when any.
	TaskID string
	// Stage, Kind, Disposition, and Action are enumerated taxonomy labels copied
	// from the existing termination taxonomy; they are labels, never verdicts.
	Stage       string
	Kind        string
	Disposition string
	Action      string
	// Reason is a bounded single-line explanation.
	Reason string
	// Timestamp is the observation time.
	Timestamp time.Time
}

// OrchestrationRun is the observation-only descriptor composed by the harness
// from an orchestration run. It is the input to ComposeOrchestrationEvents. Every
// field is evidence the harness already owns; composing it makes no decision.
type OrchestrationRun struct {
	// RunID and TaskID identify the run.
	RunID  string
	TaskID string
	// Workers are the worker lifecycles, in caller order.
	Workers []WorkerRun
	// Integration is the INTEGRATE pass (at most one per composed run is typical;
	// multiple passes are preserved in order).
	Integration []IntegrationRun
	// Verification are the deterministic check observations, in order.
	Verification []VerificationRun
	// Termination is the end-state observation.
	Termination TerminationRun
}

// ComposeOrchestrationEvents derives the ordered orchestration-graph event list
// from an OrchestrationRun. The returned events are in causal order (assignment
// before its selection, selection before resolution, start before complete,
// complete before accept/reject, integration start before complete, verification
// before termination). It is pure and deterministic: identical descriptors yield
// byte-identical event lists.
//
// The function is the producer the runtrace package previously lacked. It makes
// the orchestration graph reconstructable from trace.json alone while preserving
// the observation-only boundary: nothing reads its output back to drive a
// decision.
func ComposeOrchestrationEvents(run OrchestrationRun) []OrchestrationEvent {
	var events []OrchestrationEvent

	add := func(e OrchestrationEvent) {
		events = append(events, sanitizeEvent(e))
	}

	// Deterministic assignment ordering: by (AssignmentID, TaskID).
	workers := append([]WorkerRun(nil), run.Workers...)
	sort.SliceStable(workers, func(i, j int) bool {
		if workers[i].AssignmentID != workers[j].AssignmentID {
			return workers[i].AssignmentID < workers[j].AssignmentID
		}
		return workers[i].TaskID < workers[j].TaskID
	})

	for _, w := range workers {
		node := assignmentNodeID(w.AssignmentID)
		workerNode := workerNodeID(w.WorkerID, w.AssignmentID)

		// 1. Assignment created.
		add(OrchestrationEvent{
			Kind:         EventAssignmentCreated,
			Timestamp:    w.Started,
			AssignmentID: w.AssignmentID,
			TaskID:       w.TaskID,
			Children:     []string{workerNode},
		})

		// 2. Worker selected (edge: assignment -> worker selection).
		add(OrchestrationEvent{
			Kind:         EventWorkerSelected,
			Timestamp:    w.Started,
			AssignmentID: w.AssignmentID,
			TaskID:       w.TaskID,
			WorkerID:     w.WorkerID,
			Parents:      []string{node},
		})

		// 3. Model class selected (routing DECISION node).
		add(OrchestrationEvent{
			Kind:          EventModelClassSelected,
			Timestamp:     w.Started,
			AssignmentID:  w.AssignmentID,
			TaskID:        w.TaskID,
			WorkerID:      w.WorkerID,
			ModelClass:    w.ModelClass,
			RoutingSource: w.RoutingSource,
			Parents:       []string{workerNode},
		})

		// 4. Provider/model resolved (EXECUTING target node, distinct from class).
		add(OrchestrationEvent{
			Kind:         EventProviderResolved,
			Timestamp:    w.Started,
			AssignmentID: w.AssignmentID,
			TaskID:       w.TaskID,
			WorkerID:     w.WorkerID,
			Provider:     w.Provider,
			Model:        w.Model,
			Locality:     w.Locality,
			Parents:      []string{workerNode},
		})

		// 5. Worker started.
		add(OrchestrationEvent{
			Kind:         EventWorkerStarted,
			Timestamp:    w.Started,
			AssignmentID: w.AssignmentID,
			TaskID:       w.TaskID,
			WorkerID:     w.WorkerID,
			Parents:      []string{workerNode},
		})

		// 6. Worker completed.
		add(OrchestrationEvent{
			Kind:         EventWorkerCompleted,
			Timestamp:    w.Completed,
			AssignmentID: w.AssignmentID,
			TaskID:       w.TaskID,
			WorkerID:     w.WorkerID,
			Status:       completedStatus(w.Accepted),
			Parents:      []string{workerNode},
		})

		// 7. Result accepted or rejected (complete -> accept/reject).
		if w.Accepted {
			add(OrchestrationEvent{
				Kind:         EventResultAccepted,
				Timestamp:    w.Completed,
				AssignmentID: w.AssignmentID,
				TaskID:       w.TaskID,
				WorkerID:     w.WorkerID,
				Status:       "accepted",
				Reason:       w.AcceptedReason,
			})
		} else {
			add(OrchestrationEvent{
				Kind:         EventResultRejected,
				Timestamp:    w.Completed,
				AssignmentID: w.AssignmentID,
				TaskID:       w.TaskID,
				WorkerID:     w.WorkerID,
				Status:       "rejected",
				Reason:       w.RejectedReason,
			})
		}
	}

	// Integration passes, in caller order.
	for _, pass := range run.Integration {
		inputs := append([]string(nil), pass.Inputs...)
		sort.Strings(inputs)
		add(OrchestrationEvent{
			Kind:      EventIntegrationStarted,
			Timestamp: pass.Started,
			TaskID:    pass.TaskID,
			Parents:   inputs,
		})
		add(OrchestrationEvent{
			Kind:      EventIntegrationCompleted,
			Timestamp: pass.Completed,
			TaskID:    pass.TaskID,
			Status:    "completed",
			Reason:    pass.Reason,
		})
	}

	// Verification results, in caller order.
	for _, v := range run.Verification {
		add(OrchestrationEvent{
			Kind:      EventVerificationResult,
			Timestamp: v.Timestamp,
			TaskID:    v.TaskID,
			Status:    v.Status,
			Reason:    v.Command,
		})
	}

	// Termination.
	add(OrchestrationEvent{
		Kind:      EventTermination,
		Timestamp: run.Termination.Timestamp,
		TaskID:    run.Termination.TaskID,
		Status:    run.Termination.Disposition,
		Reason:    run.Termination.Reason,
	})

	// Assign stable sequence numbers in emission order.
	for i := range events {
		events[i].Sequence = i + 1
	}
	return events
}

// assignmentNodeID names the assignment graph node. It is a stable reference.
func assignmentNodeID(assignmentID string) string { return "assignment:" + assignmentID }

// workerNodeID names the worker-selection graph node. It is a stable reference.
func workerNodeID(workerID, assignmentID string) string {
	if workerID == "" {
		return "worker:" + assignmentID
	}
	return "worker:" + workerID
}

// completedStatus maps a worker's accept/reject verdict to a completion label.
func completedStatus(accepted bool) string {
	if accepted {
		return "completed"
	}
	return "failed"
}
