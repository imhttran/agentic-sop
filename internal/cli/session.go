package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
	"github.com/imhttran/agentic-sop/internal/verify"
)

// runSession holds the per-invocation caches that let a run reuse work it can
// prove is still valid. A hit is only possible when the identity of the inputs is
// byte-identical — for validation, the command set plus the working-tree change;
// for review, the task, diff, and engine — so a changed repository or changed
// configuration can never reuse a stale result. It is a performance optimization
// only: a miss is always safe, and correctness never depends on a hit.
//
// Only passing validation and clean review are cached. A failure is always
// re-run, so a transient or flaky failure is never replayed onto another task.
//
// The session is created once per `sop run` invocation and discarded when it
// ends. Automatic-recovery state does not live here: it is the scheduler's
// per-invocation guard, created per `sop run` invocation and discarded with it,
// so no recovery state leaks between separate invocations.
type runSession struct {
	validation map[string]testrunner.SuiteResult
	review     map[string]review.Report
	// scoped enables verification.scoped test selection. Only a plan drive sets
	// it, because only a drive runs the full-suite gate before completion.
	scoped bool
	// earlyJEV tallies this run's early JEV checks, so a run where every one
	// failed can say so once instead of only per task.
	// triage holds each task's early JEV triage result for this run.
	triage   map[string]earlyGateResult
	earlyJEV struct {
		ran, failed int
		detail      string
	}
}

// cachedTriage returns this run's earlier triage result for a task.
func (s *runSession) cachedTriage(taskID string) (earlyGateResult, bool) {
	if s == nil {
		return earlyGateResult{}, false
	}
	tri, ok := s.triage[taskID]
	return tri, ok
}

// cacheTriage records a task's triage result for the rest of the run. Only a
// result that actually ran is kept, so a disabled gate stays a strict no-op.
func (s *runSession) cacheTriage(taskID string, tri earlyGateResult) {
	if s == nil || !tri.Ran {
		return
	}
	if s.triage == nil {
		s.triage = map[string]earlyGateResult{}
	}
	s.triage[taskID] = tri
}

// noteEarlyJEV records one early JEV checkpoint result.
func (s *runSession) noteEarlyJEV(res earlyGateResult) {
	if s == nil || !res.Ran {
		return
	}
	s.earlyJEV.ran++
	if res.ProviderFailed {
		s.earlyJEV.failed++
		s.earlyJEV.detail = res.FailureDetail
	}
}

func newRunSession() *runSession {
	return &runSession{
		validation: map[string]testrunner.SuiteResult{},
		review:     map[string]review.Report{},
	}
}

// cachedValidation returns a prior passing validation for the identity.
func (s *runSession) cachedValidation(key string) (testrunner.SuiteResult, bool) {
	if s == nil {
		return testrunner.SuiteResult{}, false
	}
	suite, ok := s.validation[key]
	return suite, ok
}

// cacheValidation stores a passing validation result under the identity.
func (s *runSession) cacheValidation(key string, suite testrunner.SuiteResult) {
	if s != nil && suite.Passed() {
		s.validation[key] = suite
	}
}

// scope applies verification.scoped selection when this session enables it. The
// task's change set is read only when scoping can apply.
func (s *runSession) scope(cfg config.Config, v config.Validation, tier verify.Tier, rn *runpkg.Run) config.Validation {
	if s == nil || !s.scoped || len(cfg.Verification.Scoped) == 0 {
		return v
	}
	return scopeValidation(cfg, v, tier, rn.ChangedFiles())
}

// cachedReview returns a prior clean review for the identity.
func (s *runSession) cachedReview(key string) (review.Report, bool) {
	if s == nil {
		return review.Report{}, false
	}
	r, ok := s.review[key]
	return r, ok
}

// cacheReview stores a review with no findings under the identity.
func (s *runSession) cacheReview(key string, r review.Report) {
	if s != nil && len(r.Findings) == 0 {
		s.review[key] = r
	}
}

// validationIdentity hashes the validation command set and the working-tree
// change: the inputs that determine the result. Any change to either yields a
// different identity, so a stale result can never be reused.
func validationIdentity(v config.Validation, diff string) string {
	h := sha256.New()
	for _, c := range validate.Checks(v) {
		fmt.Fprintf(h, "%s\x00%s\x00", c.Category, c.Command)
	}
	h.Write([]byte{0})
	h.Write([]byte(diff))
	return hex.EncodeToString(h.Sum(nil))
}

// reviewIdentity hashes the review input and engine.
func reviewIdentity(engine, task, diff string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", engine, task)
	h.Write([]byte(diff))
	return hex.EncodeToString(h.Sum(nil))
}
