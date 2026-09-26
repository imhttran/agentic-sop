package domain

// TaskStatus is the workflow state of a Task. The domain owns the complete
// vocabulary; orchestration code asks the state machine what is legal instead
// of comparing states by hand.
type TaskStatus string

// Happy path: PLANNED → READY → BRANCH_CREATED → TESTS_WRITTEN → RED_VERIFIED
// → IMPLEMENTING → LOCAL_TESTS_PASS → REVIEW → REVIEW_PASS → PR_OPEN
// → CI_RUNNING → CI_PASS → MERGED → DONE. FIX_REQUIRED is the remediation
// state; BLOCKED is terminal for V1.
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
	BLOCKED          TaskStatus = "BLOCKED"
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
)
