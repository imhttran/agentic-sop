package domain

// TaskStatus is the workflow state of a Task. The domain owns the complete
// vocabulary; orchestration code asks the state machine what is legal instead
// of comparing states by hand.
type TaskStatus string

// Happy path: PLANNED → READY → BRANCH_CREATED → TESTS_WRITTEN → RED_VERIFIED
// → IMPLEMENTING → LOCAL_TESTS_PASS → REVIEW → REVIEW_PASS. From REVIEW_PASS the
// task either completes locally (LOCAL_DONE) or continues through the remote
// lifecycle (PR_OPEN → CI_RUNNING → CI_PASS → MERGED → DONE). FIX_REQUIRED is the
// remediation state; BLOCKED is terminal. A PLANNED, READY, or BLOCKED task may be
// dispositioned NOT_REQUIRED (see Task.MarkNotRequired), which is also terminal.
const (
	PLANNED          TaskStatus = "PLANNED"
	READY            TaskStatus = "READY"
	BRANCH_CREATED   TaskStatus = "BRANCH_CREATED"
	TESTS_WRITTEN    TaskStatus = "TESTS_WRITTEN"
	RED_VERIFIED     TaskStatus = "RED_VERIFIED"
	IMPLEMENTING     TaskStatus = "IMPLEMENTING"
	LOCAL_TESTS_PASS TaskStatus = "LOCAL_TESTS_PASS"
	REVIEW           TaskStatus = "REVIEW"
	REVIEW_PASS      TaskStatus = "REVIEW_PASS"
	PR_OPEN          TaskStatus = "PR_OPEN"
	CI_RUNNING       TaskStatus = "CI_RUNNING"
	CI_PASS          TaskStatus = "CI_PASS"
	FIX_REQUIRED     TaskStatus = "FIX_REQUIRED"
	MERGED           TaskStatus = "MERGED"
	DONE             TaskStatus = "DONE"
	// LOCAL_DONE marks a task completed by a local-only run: it passed the local
	// lifecycle (through REVIEW_PASS) but no PR was opened, no CI ran, and nothing
	// was merged. It is distinct from DONE, which follows a real MERGED.
	LOCAL_DONE TaskStatus = "LOCAL_DONE"
	BLOCKED    TaskStatus = "BLOCKED"
	// NOT_REQUIRED marks a task whose conditional work upstream evidence proved
	// unnecessary. It is terminal and satisfies dependants, but it is not a
	// completion: no implementation ran and no repository change is implied.
	NOT_REQUIRED TaskStatus = "NOT_REQUIRED"
)

type BlockedReason string

const (
	NO_REASON               BlockedReason = ""
	DEPENDENCIES_INCOMPLETE BlockedReason = "DEPENDENCIES_INCOMPLETE"
	RETRIES_EXHAUSTED       BlockedReason = "RETRIES_EXHAUSTED"
	CI_FAILURE_UNACTIONABLE BlockedReason = "CI_FAILURE_UNACTIONABLE"
	TEST_DESIGN_FAILED      BlockedReason = "TEST_DESIGN_FAILED"
	IMPLEMENTATION_TIMEOUT  BlockedReason = "IMPLEMENTATION_TIMEOUT"
	REVIEW_UNRESOLVED       BlockedReason = "REVIEW_UNRESOLVED"
	// CONTINUATION_EXHAUSTED marks a task that kept needing another bounded work
	// slice (CONTINUE) without ever producing a repository change, until its
	// continuation budget was spent. It is a bounded automation failure — the agent
	// was stuck, not a human decision — so the task is terminally stuck and is not
	// automatically recovered again.
	CONTINUATION_EXHAUSTED BlockedReason = "CONTINUATION_EXHAUSTED"
	// NO_PROGRESS: the execution agent made no governed repository progress within
	// its bounded allowance. It is a blocked-for-operator state, not a human approval.
	NO_PROGRESS BlockedReason = "NO_PROGRESS"
	// VALIDATION_NOT_CONFIGURED marks a task that changed the repository but has no
	// validation configured to verify the change. SOP fails closed rather than treat
	// a vacuous empty suite as a pass. It is a terminal operator-intervention /
	// configuration state, not a human approval: the operator configures validation
	// (for example via `sop init`) and re-runs. Like NO_PROGRESS, it is not
	// auto-continued or auto-fixed under unchanged configuration.
	VALIDATION_NOT_CONFIGURED BlockedReason = "VALIDATION_NOT_CONFIGURED"
)
