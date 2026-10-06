package recovery

import (
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

// EvidenceFrom builds recovery evidence from the failure classifier's verdict for
// a failed attempt. It is the adapter between the existing failure vocabulary and
// the recovery policy, so recovery never reimplements failure classification.
//
// class is the model class the failed attempt used; stage names where it failed
// (build, test, lint, review, provider) when known; escalations is how many
// escalations the task has already taken; attempt is the 1-based number of the
// attempt that failed; replans is how many times the task has already changed
// strategy.
func EvidenceFrom(cls failure.Classification, class model.Class, stage string, escalations, attempt, replans int) Evidence {
	return Evidence{
		Class:       class,
		Kind:        cls.Kind,
		Disposition: cls.Disposition,
		Stage:       stage,
		Escalations: escalations,
		Attempt:     attempt,
		Replans:     replans,
	}
}
