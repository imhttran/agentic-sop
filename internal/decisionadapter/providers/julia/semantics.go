package julia

import (
	"fmt"

	"github.com/imhttran/agentic-sop/decision"
)

// This file holds Julia-1-ONNX's native vocabulary. It is deliberately confined
// to this adapter: none of it may appear in the public decision package.
//
// Julia is a supplied-options decision model. Each native decision carries an
// ordered set of options plus a question type ("qtype"):
//
//	qtype 0  choice  — select one option
//	qtype 1  score   — expected zero-based option index
//	qtype 2  noul    — boolean (option 0 = false, option 1 = true)
//
// CHOICE and SCORE require 2..20 ordered options. BOOLEAN always uses exactly two
// internally generated options and carries none from the caller.
//
// These constraints are Julia-specific. They are enforced here, never in the
// provider-neutral decision contract, which intentionally permits other option
// shapes for other providers.

// Julia-native qtype values.
const (
	juliaQTypeChoice  = 0
	juliaQTypeScore   = 1
	juliaQTypeBoolean = 2
)

// Julia's option-count limits for CHOICE and SCORE.
const (
	juliaMinOptions = 2
	juliaMaxOptions = 20
)

// juliaBooleanOptions are Julia's fixed noul options. Index 0 is false and index
// 1 is true, so the normalized boolean probability is P(true) =
// softmax(logits)[1].
var juliaBooleanOptions = []string{"false", "true"}

// juliaQType maps a provider-neutral question type onto Julia's native qtype.
func juliaQType(t decision.QuestionType) (int, error) {
	switch t {
	case decision.QuestionChoice:
		return juliaQTypeChoice, nil
	case decision.QuestionScore:
		return juliaQTypeScore, nil
	case decision.QuestionBoolean:
		return juliaQTypeBoolean, nil
	default:
		return 0, fmt.Errorf("%w: unsupported question type %q", decision.ErrInvalidRequest, t)
	}
}

// juliaOptions returns the ordered Julia options for a question and enforces
// Julia's 2..20 option limit.
//
// CHOICE and SCORE use the question's ordered Choices; BOOLEAN uses Julia's fixed
// false/true options. The public contract has no first-class score-scale field,
// so SCORE levels ride along in Choices (existing design debt tracked in the
// backlog). A CHOICE or SCORE question with fewer than two or more than twenty
// options has no valid Julia decision to emit and is rejected with
// decision.ErrInvalidRequest.
func juliaOptions(q decision.Question) ([]string, error) {
	switch q.Type {
	case decision.QuestionBoolean:
		return append([]string(nil), juliaBooleanOptions...), nil
	case decision.QuestionChoice, decision.QuestionScore:
		n := len(q.Choices)
		if n < juliaMinOptions || n > juliaMaxOptions {
			return nil, fmt.Errorf("%w: question %q: Julia %s questions require %d-%d ordered options (got %d)",
				decision.ErrInvalidRequest, q.ID, q.Type, juliaMinOptions, juliaMaxOptions, n)
		}
		return append([]string(nil), q.Choices...), nil
	default:
		return nil, fmt.Errorf("%w: question %q: unsupported question type %q", decision.ErrInvalidRequest, q.ID, q.Type)
	}
}
