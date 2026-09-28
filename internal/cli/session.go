package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
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
type runSession struct {
	validation map[string]testrunner.SuiteResult
	review     map[string]review.Report
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
