package runtrace

// This file records the ORCH-007 INTEGRATE stage in the run trace.
//
// The recording is observation-only, like the rest of runtrace: it composes
// evidence SOP already owns (the worker-result claims consumed by integration and
// the integrated view produced) into an ordered list of stage records, so
// INTEGRATE is distinguishable in the trace from the WORK/RESULTS stages that
// produced its inputs. Nothing here is read back to drive a routing, lifecycle,
// retry, progress, approval, or termination decision.

// StageRecord is one explicitly recorded orchestration stage traversal. It makes
// the WORK -> RESULTS -> INTEGRATE -> VERIFY pipeline observable in the trace and
// keeps INTEGRATE distinct from worker execution: its Inputs are the worker-result
// references consumed and its Output is the integration-result reference produced.
type StageRecord struct {
	// Sequence orders the records within one run.
	Sequence int `json:"sequence"`
	// Stage is the recorded orchestration stage label (WORK, RESULTS, INTEGRATE,
	// VERIFY). It is a plain label, not a lifecycle state.
	Stage string `json:"stage"`
	// TaskID is the integration target, when known.
	TaskID string `json:"task_id,omitempty"`
	// Inputs are references to the worker results this stage consumed (assignment
	// IDs). For INTEGRATE they are the worker claims integrated.
	Inputs []string `json:"inputs,omitempty"`
	// Output is a reference to the artifact this stage produced. For INTEGRATE it
	// is the integration-result reference. It is never a verdict.
	Output string `json:"output,omitempty"`
	// NextStage names the stage the run proceeds to (INTEGRATE always proceeds to
	// VERIFY). It records ordering only and grants no authority.
	NextStage string `json:"next_stage,omitempty"`
}
