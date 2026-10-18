package question

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestAtomicSubmissionConcurrentExactlyOneAccepted proves that a submission is one
// atomic transaction across competing stores: with two stores over the same path
// submitting concurrently, exactly one answer is accepted, the question reaches
// ANSWERED with a single answer, and both attempts are audited.
func TestAtomicSubmissionConcurrentExactlyOneAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	seed, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	q, err := NewService(seed, nil).CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"})
	if err != nil {
		t.Fatalf("seed question: %v", err)
	}

	const n = 2
	stores := make([]*FileStore, n)
	for i := 0; i < n; i++ {
		s, err := OpenFileStore(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		stores[i] = s
	}

	var wg sync.WaitGroup
	results := make([]SubmitResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			svc := NewService(stores[i], nil)
			results[i], errs[i] = svc.SubmitAnswer(context.Background(), SubmitInput{
				QuestionID: q.ID, Body: fmt.Sprintf("answer-%d", i), IdempotencyKey: fmt.Sprintf("k-%d", i),
			}, okBinding)
		}(i)
	}
	close(start)
	wg.Wait()

	accepted, rejected := 0, 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("submit %d: %v", i, errs[i])
		}
		switch results[i].Outcome {
		case OutcomeAccepted:
			accepted++
		case OutcomeRejected:
			rejected++
		default:
			t.Fatalf("submit %d unexpected outcome %q", i, results[i].Outcome)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d, want exactly one of each", accepted, rejected)
	}

	// Consistent state: one answer, question ANSWERED.
	check, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, answers, err := NewService(check, nil).ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.State != StateAnswered || len(answers) != 1 {
		t.Fatalf("state=%s answers=%d, want ANSWERED with exactly 1 answer", got.State, len(answers))
	}
	// Both attempts durably audited.
	outcomes, err := check.Outcomes(q.ID)
	if err != nil {
		t.Fatalf("outcomes: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2 (ACCEPTED + REJECTED)", len(outcomes))
	}
}

// TestFileStoreWritesFailClosedWithoutLock proves the fail-closed policy: when the
// advisory lock cannot be taken (as on Windows), mutation and answer submission
// return ErrLockUnsupported while reads still work.
func TestFileStoreWritesFailClosedWithoutLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	fs.lock = func(*os.File) error { return ErrLockUnsupported }

	if _, err := fs.ListQuestions(StatePending); err != nil {
		t.Fatalf("read must still work without a lock: %v", err)
	}
	svc := NewService(fs, nil)
	if _, err := svc.CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"}); !errors.Is(err, ErrLockUnsupported) {
		t.Fatalf("create err = %v, want ErrLockUnsupported", err)
	}
	if _, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: "x", Body: "y"}, okBinding); !errors.Is(err, ErrLockUnsupported) {
		t.Fatalf("submit err = %v, want ErrLockUnsupported", err)
	}
}

// TestSubmitIdempotencyKeyDifferentPayloadRejected proves a reuse of the idempotency
// key with a different payload is rejected, never treated as a successful duplicate.
func TestSubmitIdempotencyKeyDifferentPayloadRejected(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	q, _ := svc.CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"})
	if _, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a", IdempotencyKey: "k1"}, okBinding); err != nil {
		t.Fatalf("first: %v", err)
	}
	res, err := svc.SubmitAnswer(context.Background(), SubmitInput{QuestionID: q.ID, Body: "a DIFFERENT payload", IdempotencyKey: "k1"}, okBinding)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Outcome != OutcomeRejected {
		t.Fatalf("outcome = %q, want REJECTED for a reused key with a different payload", res.Outcome)
	}
}

// TestSubmitHelperProcess is not a real test: it is the subprocess entry point the
// cross-process test re-executes. It is skipped unless the helper env is set.
func TestSubmitHelperProcess(t *testing.T) {
	if os.Getenv("QUESTION_SUBMIT_HELPER") != "1" {
		t.Skip("helper process entry point")
	}
	fs, err := OpenFileStore(os.Getenv("QUESTION_STORE_PATH"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	res, err := NewService(fs, nil).SubmitAnswer(context.Background(), SubmitInput{
		QuestionID:     os.Getenv("QUESTION_ID"),
		Body:           os.Getenv("QUESTION_BODY"),
		IdempotencyKey: os.Getenv("QUESTION_KEY"),
	}, okBinding)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	fmt.Println(res.Outcome)
	os.Exit(0)
}

// TestAtomicSubmissionAcrossProcessesExactlyOneAccepted proves the submission is
// atomic across SEPARATE OS PROCESSES (not just goroutines): two independent
// processes submitting to the same durable store produce exactly one accepted
// answer, consistent state, and both audit outcomes on disk.
func TestAtomicSubmissionAcrossProcessesExactlyOneAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	seed, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	q, err := NewService(seed, nil).CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"})
	if err != nil {
		t.Fatalf("seed question: %v", err)
	}

	run := func(key, body string) (OutcomeKind, string) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSubmitHelperProcess$")
		cmd.Env = append(os.Environ(),
			"QUESTION_SUBMIT_HELPER=1",
			"QUESTION_STORE_PATH="+path,
			"QUESTION_ID="+q.ID,
			"QUESTION_KEY="+key, //gitleaks:allow: synthetic idempotency key, not a credential
			"QUESTION_BODY="+body,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("helper process %s: %v\n%s", key, err, out)
		}
		return OutcomeKind(strings.TrimSpace(string(out))), string(out)
	}

	const n = 2
	var wg sync.WaitGroup
	outs := make([]string, n)
	kinds := make([]OutcomeKind, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			kinds[i], outs[i] = run(fmt.Sprintf("k-%d", i), fmt.Sprintf("answer-%d", i))
		}(i)
	}
	close(start)
	wg.Wait()

	accepted, rejected := 0, 0
	for i := 0; i < n; i++ {
		switch kinds[i] {
		case OutcomeAccepted:
			accepted++
		case OutcomeRejected:
			rejected++
		default:
			t.Fatalf("process %d outcome = %q (raw %q), want ACCEPTED or REJECTED", i, kinds[i], outs[i])
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d, want exactly one of each across processes", accepted, rejected)
	}

	check, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, answers, err := NewService(check, nil).ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.State != StateAnswered || len(answers) != 1 {
		t.Fatalf("state=%s answers=%d, want ANSWERED with exactly 1 answer", got.State, len(answers))
	}
	outcomes, err := check.Outcomes(q.ID)
	if err != nil {
		t.Fatalf("outcomes: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2 (ACCEPTED + REJECTED) persisted", len(outcomes))
	}
}
