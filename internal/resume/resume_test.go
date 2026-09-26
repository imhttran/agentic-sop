package resume

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/github"
)

func task(status domain.TaskStatus) *domain.Task {
	return &domain.Task{ID: "T1", Title: "T1", Status: status, MaxAttempts: 1}
}

func taskID(id string, status domain.TaskStatus) *domain.Task {
	return &domain.Task{ID: id, Title: id, Status: status, MaxAttempts: 1}
}

func pr() *github.PullRequest { return &github.PullRequest{Number: 1} }

func TestResolveActionsForEveryStatus(t *testing.T) {
	cases := []struct {
		status domain.TaskStatus
		obs    Observation
		action Action
	}{
		{domain.PLANNED, Observation{}, CreateBranch},
		{domain.READY, Observation{}, CreateBranch},
		{domain.BRANCH_CREATED, Observation{BranchExists: true}, WriteTests},
		{domain.TESTS_WRITTEN, Observation{BranchExists: true}, VerifyRed},
		{domain.RED_VERIFIED, Observation{BranchExists: true}, Implement},
		{domain.IMPLEMENTING, Observation{BranchExists: true}, Implement},
		{domain.FIX_REQUIRED, Observation{BranchExists: true}, Implement},
		{domain.LOCAL_TESTS_PASS, Observation{BranchExists: true}, Review},
		{domain.REVIEW, Observation{BranchExists: true}, Review},
		{domain.REVIEW_PASS, Observation{BranchExists: true}, OpenPR},
		{domain.PR_OPEN, Observation{BranchExists: true, PR: pr()}, PollCI},
		{domain.CI_RUNNING, Observation{BranchExists: true, PR: pr()}, PollCI},
		{domain.CI_PASS, Observation{BranchExists: true, PR: pr()}, Merge},
		{domain.MERGED, Observation{}, Finish},
		{domain.DONE, Observation{}, None},
		{domain.BLOCKED, Observation{}, None},
	}

	for _, tc := range cases {
		got, err := resolve(task(tc.status), tc.obs)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.status, err)
			continue
		}
		if got.Action != tc.action {
			t.Errorf("%s: action = %s, want %s", tc.status, got.Action, tc.action)
		}
		if got.Recovered {
			t.Errorf("%s: unexpectedly reported recovery", tc.status)
		}
	}
}

func TestRecoverExistingBranch(t *testing.T) {
	for _, status := range []domain.TaskStatus{domain.READY, domain.PLANNED} {
		got, err := resolve(task(status), Observation{BranchExists: true})
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if !got.Recovered || got.Action != WriteTests {
			t.Errorf("%s: decision = %+v, want recovered WRITE_TESTS", status, got)
		}
		if want := domain.BRANCH_CREATED; got.path[len(got.path)-1] != want {
			t.Errorf("%s: recovery target = %s, want %s", status, got.path[len(got.path)-1], want)
		}
	}
}

func TestRecoverExistingPR(t *testing.T) {
	got, err := resolve(task(domain.REVIEW_PASS), Observation{BranchExists: true, PR: pr()})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !got.Recovered || got.Action != PollCI {
		t.Errorf("decision = %+v, want recovered POLL_CI", got)
	}
}

func TestConsistencyErrors(t *testing.T) {
	if _, err := resolve(task(domain.BRANCH_CREATED), Observation{}); err == nil {
		t.Error("expected error for BRANCH_CREATED without a branch")
	}
	if _, err := resolve(task(domain.PR_OPEN), Observation{BranchExists: true}); err == nil {
		t.Error("expected error for PR_OPEN without a pull request")
	}
}

type fakeStore struct {
	tasks   map[string]*domain.Task
	order   []string
	saveErr error
}

func newStore(tasks ...*domain.Task) *fakeStore {
	s := &fakeStore{tasks: make(map[string]*domain.Task)}
	for _, t := range tasks {
		cp := *t
		s.tasks[t.ID] = &cp
		s.order = append(s.order, t.ID)
	}
	return s
}

func (s *fakeStore) List() ([]*domain.Task, error) {
	out := make([]*domain.Task, 0, len(s.order))
	for _, id := range s.order {
		cp := *s.tasks[id]
		out = append(out, &cp)
	}
	return out, nil
}

func (s *fakeStore) Get(id string) (*domain.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *t
	return &cp, nil
}

func (s *fakeStore) Save(task *domain.Task) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	cp := *task
	s.tasks[task.ID] = &cp
	return nil
}

type fakeObserver struct {
	obs Observation
	err error
}

func (o *fakeObserver) Observe(context.Context, *domain.Task) (Observation, error) {
	return o.obs, o.err
}

func TestResolvePersistsRecovery(t *testing.T) {
	store := newStore(task(domain.READY))
	observer := &fakeObserver{obs: Observation{BranchExists: true}}

	decision, err := New(store, observer).Resolve(context.Background(), "T1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !decision.Recovered || decision.Action != WriteTests {
		t.Errorf("decision = %+v, want recovered WRITE_TESTS", decision)
	}
	if got := store.tasks["T1"].Status; got != domain.BRANCH_CREATED {
		t.Errorf("persisted status = %s, want BRANCH_CREATED", got)
	}
}

func TestRecoverySaveFailureIsAtomic(t *testing.T) {
	store := newStore(task(domain.READY))
	store.saveErr = errors.New("disk full")
	observer := &fakeObserver{obs: Observation{BranchExists: true}}

	if _, err := New(store, observer).Resolve(context.Background(), "T1"); err == nil {
		t.Fatal("expected save error to propagate")
	}
	if got := store.tasks["T1"].Status; got != domain.READY {
		t.Errorf("status = %s, want READY (unchanged)", got)
	}
}

func TestObserverErrorPropagates(t *testing.T) {
	store := newStore(task(domain.READY))
	observer := &fakeObserver{err: errors.New("git failed")}
	if _, err := New(store, observer).Resolve(context.Background(), "T1"); err == nil {
		t.Error("expected observer error to propagate")
	}
}

func TestResolveActive(t *testing.T) {
	observer := &fakeObserver{obs: Observation{BranchExists: true}}

	// Nothing in flight.
	empty, err := New(newStore(), observer).ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("ResolveActive(empty) failed: %v", err)
	}
	if empty.Action != None || empty.TaskID != "" {
		t.Errorf("empty decision = %+v, want NONE", empty)
	}

	// Exactly one in-flight task.
	one, err := New(newStore(taskID("T1", domain.PLANNED), taskID("T2", domain.IMPLEMENTING)), observer).ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("ResolveActive(one) failed: %v", err)
	}
	if one.TaskID != "T2" {
		t.Errorf("task = %q, want T2", one.TaskID)
	}

	// Several in-flight tasks require an explicit id.
	many := newStore(
		&domain.Task{ID: "T1", Status: domain.IMPLEMENTING},
		&domain.Task{ID: "T2", Status: domain.CI_RUNNING},
	)
	if _, err := New(many, observer).ResolveActive(context.Background()); err == nil {
		t.Error("expected an error when several tasks are in flight")
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newStore(task(domain.READY))
	if _, err := New(store, &fakeObserver{}).Resolve(ctx, "T1"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if _, err := New(store, &fakeObserver{}).ResolveActive(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
