package question

import (
	"context"
	"errors"
	"testing"
)

func okBinding(_ context.Context, _ *Question) (string, error) { return "", nil }

func TestSubmitAnswerAcceptedRecordsAnsweredOnly(t *testing.T) {
	st := NewMemoryStore()
	svc := NewService(st, nil)
	q, err := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "which DB?", Revision: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := svc.SubmitAnswer(context.Background(), SubmitInput{
		QuestionID: q.ID, Body: "postgres", AnsweredBy: "planner", IdempotencyKey: "k1", ExpectedRevision: 1,
	}, okBinding)
	if err != nil {
		t.Fatalf("SubmitAnswer: %v", err)
	}
	if res.Outcome != OutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED (%s)", res.Outcome, res.Reason)
	}
	if res.Question == nil || res.Question.State != StateAnswered {
		t.Fatalf("question state = %+v, want ANSWERED", res.Question)
	}
	if res.Answer == nil || res.Answer.ID == "" {
		t.Fatalf("answer = %+v, want a recorded answer", res.Answer)
	}

	// The answer is recorded independently and the question advanced to ANSWERED.
	_, answers, err := svc.ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(answers))
	}
	// An audit outcome was recorded.
	outcomes, err := st.Outcomes(q.ID)
	if err != nil {
		t.Fatalf("outcomes: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Kind != OutcomeAccepted {
		t.Fatalf("outcomes = %+v, want one ACCEPTED", outcomes)
	}
}

func TestSubmitAnswerDuplicateAndConflict(t *testing.T) {
	st := NewMemoryStore()
	svc := NewService(st, nil)
	q, _ := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "q", Revision: 1})
	first, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a", IdempotencyKey: "k1"}, okBinding)
	if err != nil || first.Outcome != OutcomeAccepted {
		t.Fatalf("first = %+v err=%v", first, err)
	}

	dup, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a", IdempotencyKey: "k1"}, okBinding)
	if err != nil {
		t.Fatalf("dup: %v", err)
	}
	if dup.Outcome != OutcomeDuplicate || dup.Answer == nil || dup.Answer.ID != first.Answer.ID {
		t.Fatalf("dup = %+v, want DUPLICATE returning the existing answer", dup)
	}

	conflict, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "b", IdempotencyKey: "k2"}, okBinding)
	if err != nil {
		t.Fatalf("conflict: %v", err)
	}
	if conflict.Outcome != OutcomeRejected {
		t.Fatalf("conflict = %+v, want REJECTED", conflict)
	}
	// Only the first answer is stored; the conflict was not appended.
	_, answers, _ := svc.ReadQuestion(q.ID)
	if len(answers) != 1 {
		t.Fatalf("answers = %d, want 1 (conflict must not append)", len(answers))
	}
}

func TestSubmitAnswerStaleBindingPreservesRecord(t *testing.T) {
	st := NewMemoryStore()
	svc := NewService(st, nil)
	q, _ := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "q", Revision: 1, PlanRevision: "p@old"})
	stale := func(_ context.Context, _ *Question) (string, error) { return "plan revision changed", nil }

	res, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a", IdempotencyKey: "k1"}, stale)
	if err != nil {
		t.Fatalf("SubmitAnswer: %v", err)
	}
	if res.Outcome != OutcomeStale {
		t.Fatalf("outcome = %q, want STALE", res.Outcome)
	}
	// The question remains PENDING and no answer was written.
	got, answers, _ := svc.ReadQuestion(q.ID)
	if got.State != StatePending {
		t.Fatalf("state = %q, want PENDING (stale must not advance)", got.State)
	}
	if len(answers) != 0 {
		t.Fatalf("answers = %d, want 0 (stale must not record an answer)", len(answers))
	}
	outcomes, _ := st.Outcomes(q.ID)
	if len(outcomes) != 1 || outcomes[0].Kind != OutcomeStale {
		t.Fatalf("outcomes = %+v, want one STALE", outcomes)
	}
}

func TestSubmitAnswerExpectedRevisionStale(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	q, _ := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "q", Revision: 3})
	res, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a", ExpectedRevision: 2}, okBinding)
	if err != nil {
		t.Fatalf("SubmitAnswer: %v", err)
	}
	if res.Outcome != OutcomeStale {
		t.Fatalf("outcome = %q, want STALE", res.Outcome)
	}
}

func TestSubmitAnswerUnknownQuestionAndEmptyBody(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	unknown, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: "nope", Body: "a"}, okBinding)
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}
	if unknown.Outcome != OutcomeStale {
		t.Fatalf("unknown outcome = %q, want STALE", unknown.Outcome)
	}

	q, _ := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "q"})
	if _, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "   "}, okBinding); err != nil {
		t.Fatalf("empty body: %v", err)
	}
	got, _, _ := svc.ReadQuestion(q.ID)
	if got.State != StatePending {
		t.Fatalf("state = %q, want PENDING (empty body must be rejected)", got.State)
	}
}

func TestSubmitAnswerMissingQuestionIDIsError(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	if _, err := svc.SubmitAnswer(context.Background(), SubmitInput{Body: "a"}, okBinding); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("err = %v, want ErrInvalidRecord", err)
	}
}
