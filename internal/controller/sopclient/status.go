package sopclient

// CTRL003 status model. A missing artifact is "" or StatusUnknown, never PASS.

// StatusUnknown means SOP persisted no evidence; it never implies a pass.
const StatusUnknown = "UNKNOWN"

// Aggregate validation statuses (SOP's PASS/FAIL/SKIP vocabulary).
const (
	ValidationPass    = "PASS"
	ValidationFail    = "FAIL"
	ValidationSkipped = "SKIP"
)

// Aggregate review outcomes.
const (
	ReviewNotRun = "NOT_RUN"
	ReviewPassed = "PASS"
	ReviewFailed = "FAIL"
)

// PlanGate is SOP's active plan and final gate from plan.meta.json. FinalGate
// is StatusUnknown when SOP recorded none.
type PlanGate struct {
	Recorded  bool
	Source    string
	PlanID    string
	FinalGate string
	// InProgress: an active plan whose final gate is not a completed value.
	InProgress bool
}

// StatusLevel is a status value plus whether SOP persisted evidence for it.
type StatusLevel struct {
	Level   string
	Present bool
}

// RunStatus is the current run's stage and decision.
type RunStatus struct {
	Present  bool
	Stage    string
	Decision string
	Active   bool
	// Blocked: SOP parked the run at WAITING_FOR_HUMAN.
	Blocked bool
}

// Status projects the run record; a missing run never becomes PASS.
func (r RunInfo) Status() RunStatus {
	return RunStatus{
		Present:  r.Present,
		Stage:    r.Stage,
		Decision: r.Decision,
		Active:   activeStage(r.Stage),
		Blocked:  r.Stage == StageWaitingForHuman,
	}
}

// activeStage groups SOP's in-flight stages for display.
func activeStage(stage string) bool {
	switch stage {
	case StageCreated, StagePlanning, StageImplementing, StageValidating, StageReviewing, StageFixing:
		return true
	default:
		return false
	}
}

// aggregateValidation: no checks is absent; any failure is FAIL; else PASS.
func aggregateValidation(checks []ValidationCheck) StatusLevel {
	if len(checks) == 0 {
		return StatusLevel{Level: StatusUnknown}
	}
	level := ValidationPass
	for _, c := range checks {
		switch c.Status {
		case ValidationFail:
			return StatusLevel{Level: ValidationFail, Present: true}
		case ValidationSkipped:
			if level == ValidationPass {
				level = ValidationSkipped
			}
		}
	}
	return StatusLevel{Level: level, Present: true}
}

// aggregateReview: no artifact is absent; any finding is FAIL; else PASS.
func aggregateReview(r Review) StatusLevel {
	if r.Summary == "" && len(r.Findings) == 0 {
		return StatusLevel{Level: StatusUnknown}
	}
	if len(r.Findings) > 0 {
		return StatusLevel{Level: ReviewFailed, Present: true}
	}
	return StatusLevel{Level: ReviewPassed, Present: true}
}

// aggregateQuality: a missing JEV artifact is absent, never PASS.
func aggregateQuality(j *JEV) StatusLevel {
	if j == nil {
		return StatusLevel{Level: StatusUnknown}
	}
	status := j.Status
	if status == "" {
		status = StatusUnknown
	}
	return StatusLevel{Level: status, Present: true}
}
