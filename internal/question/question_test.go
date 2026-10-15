package question

import (
	"context"
	"errors"
	"testing"
)

func TestCreateQuestionPendingAndIndependentAnswer(t *testing.T) {
	s := NewService(NewMemoryStore(), nil)
	q, err := s.CreateQuestion(&Question{Identity: "task-1", Kind: "blocking", Body: "which DB?", SubmittedBy: "executor"})
	if err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}
	if q.State != StatePending {
		t.Fatalf("state = %q, want PENDING", q.State)
	}
	if q.ID == "" {
		t.Fatal("question id must be assigned")
	}
	a, err := s.CreateAnswer(&Answer{QuestionID: q.ID, Body: "postgres", AnsweredBy: "planner"})
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if a.ID == q.ID {
		t.Fatal("answer identity must be independent of question identity")
	}
	got, answers, err := s.ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("ReadQuestion: %v", err)
	}
	if got.ID != q.ID {
		t.Fatalf("read question id = %q, want %q", got.ID, q.ID)
	}
	if len(answers) != 1 || answers[0].ID != a.ID {
		t.Fatalf("answers = %+v, want the independent answer %q", answers, a.ID)
	}
}

func TestDuplicateSubmissionDeterministic(t *testing.T) {
	s := NewService(NewMemoryStore(), nil)
	first, err := s.CreateQuestion(&Question{Identity: "task-1", Kind: "blocking", Body: "q"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	dup, err := s.CreateQuestion(&Question{Identity: "task-1", Kind: "blocking", Body: "q again"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
	if dup.ID != first.ID {
		t.Fatalf("duplicate returned %q, want existing %q", dup.ID, first.ID)
	}
	list, err := s.ListQuestions(StatePending)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("stored questions = %d, want exactly 1", len(list))
	}
}

func TestStaleSubmissionRejected(t *testing.T) {
	s := NewService(NewMemoryStore(), nil)
	base, err := s.CreateQuestion(&Question{Identity: "task-1", Kind: "blocking", Body: "q", Revision: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stale, err := s.CreateQuestion(&Question{Identity: "task-1", Kind: "blocking", Body: "newer", Revision: 6})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	if stale.ID != base.ID || stale.Revision != 5 {
		t.Fatalf("stale submission must not replace current record: got %+v", stale)
	}
}

func TestListUnknownStateFailsClosed(t *testing.T) {
	s := NewService(NewMemoryStore(), nil)
	if _, err := s.CreateQuestion(&Question{Identity: "t", Kind: "k", Body: "q"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.ListQuestions(State("WAITING_FOR_PLANNER")); !errors.Is(err, ErrUnknownState) {
		t.Fatalf("err = %v, want ErrUnknownState", err)
	}
	if _, err := ParseState("bogus"); !errors.Is(err, ErrUnknownState) {
		t.Fatalf("ParseState err = %v, want ErrUnknownState", err)
	}
}

// TestApplyBeforeDispatch proves the APPLIED record is durably persisted before
// continuation is dispatched: the observer reads the record from the store
// inside the dispatch callback.
func TestApplyBeforeDispatch(t *testing.T) {
	store := NewMemoryStore()
	var observed State
	var dispatched bool
	svc := &Service{
		store: store,
		dispatcher: DispatchFunc(func(_ context.Context, id string) error {
			dispatched = true
			q, err := store.GetQuestion(id)
			if err != nil {
				t.Fatalf("read during dispatch: %v", err)
			}
			observed = q.State
			return nil
		}),
	}
	q, err := svc.CreateQuestion(&Question{Identity: "t", Kind: "k", Body: "q"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateAnswer(&Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	// Move to ANSWERED to test the ANSWERED->APPLIED transition ordering.
	q.State = StateAnswered
	if err := store.UpdateQuestion(q); err != nil {
		t.Fatalf("update: %v", err)
	}
	applied, err := svc.ApplyAnswer(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("ApplyAnswer: %v", err)
	}
	if !dispatched {
		t.Fatal("continuation was not dispatched")
	}
	if observed != StateApplied {
		t.Fatalf("dispatch observed state %q, want APPLIED (record must precede dispatch)", observed)
	}
	if applied.State != StateApplied {
		t.Fatalf("applied state = %q, want APPLIED", applied.State)
	}
}

func TestApplyIdempotentNoDoubleDispatch(t *testing.T) {
	store := NewMemoryStore()
	dispatches := 0
	svc := &Service{store: store, dispatcher: DispatchFunc(func(_ context.Context, _ string) error {
		dispatches++
		return nil
	})}
	q, _ := svc.CreateQuestion(&Question{Identity: "t", Kind: "k", Body: "q"})
	_, _ = svc.CreateAnswer(&Answer{QuestionID: q.ID, Body: "a"})
	q.State = StateAnswered
	_ = store.UpdateQuestion(q)
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if dispatches != 1 {
		t.Fatalf("dispatches = %d, want 1 (no duplicate continuation dispatch)", dispatches)
	}
}

func TestRejectAndSupersede(t *testing.T) {
	s := NewService(NewMemoryStore(), nil)
	q, _ := s.CreateQuestion(&Question{Identity: "t", Kind: "k", Body: "q"})
	rej, err := s.RejectQuestion(q.ID, "not needed")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rej.State != StateRejected || rej.Reason != "not needed" {
		t.Fatalf("reject result = %+v", rej)
	}
	if _, err := s.RejectQuestion(q.ID, "again"); !errors.Is(err, ErrTerminal) {
		t.Fatalf("second reject err = %v, want ErrTerminal", err)
	}

	other, _ := s.CreateQuestion(&Question{Identity: "t2", Kind: "k", Body: "q2"})
	sup, err := s.SupersedeQuestion(other.ID, q.ID, "replaced")
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if sup.State != StateSuperseded || sup.SupersededBy != q.ID {
		t.Fatalf("supersede result = %+v", sup)
	}
}

// TestStatesSeparateFromRunStage asserts no question state aliases a run stage.
func TestStatesSeparateFromRunStage(t *testing.T) {
	runStage := "WAITING_FOR_PLANNER"
	for _, st := range AllStates() {
		if string(st) == runStage {
			t.Fatalf("question state %q aliases the run stage", st)
		}
	}
}
