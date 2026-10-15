package question

import (
	"path/filepath"
	"sort"
	"testing"
)

func idsOf(qs []*Question) []string {
	ids := make([]string, 0, len(qs))
	for _, q := range qs {
		ids = append(ids, q.ID)
	}
	sort.Strings(ids)
	return ids
}

// TestFileStoreReopenPreservesRecordsAndNoIDCollision proves persistence survives a
// reopen and that a new record after reload cannot reuse an existing id (the
// collision the review flagged).
func TestFileStoreReopenPreservesRecordsAndNoIDCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	svc := NewService(fs, nil)
	q1, err := svc.CreateQuestion(&Question{Identity: "T1", Kind: "blocking", Body: "q1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a1, err := svc.CreateAnswer(&Answer{QuestionID: q1.ID, Body: "a1"})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}

	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	svc2 := NewService(fs2, nil)
	q1b, answers, err := svc2.ReadQuestion(q1.ID)
	if err != nil || q1b.ID != q1.ID {
		t.Fatalf("reload question: err=%v q=%+v", err, q1b)
	}
	if len(answers) != 1 || answers[0].ID != a1.ID {
		t.Fatalf("reload answers = %+v, want %q", answers, a1.ID)
	}

	q2, err := svc2.CreateQuestion(&Question{Identity: "T2", Kind: "blocking", Body: "q2"})
	if err != nil {
		t.Fatalf("create after reload: %v", err)
	}
	if q2.ID == q1.ID {
		t.Fatalf("id collision after reload: %q", q2.ID)
	}
	if _, _, err := svc2.ReadQuestion(q1.ID); err != nil {
		t.Fatalf("prior record lost after new write: %v", err)
	}
}

// TestFileStoreConcurrentStoresNoLostUpdate proves two stores over the same path (as
// the CLI and an MCP server would be) serialize their read-modify-write and never
// lose each other's records.
func TestFileStoreConcurrentStoresNoLostUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fsA, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open A: %v", err)
	}
	fsB, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	svcA := NewService(fsA, nil)
	svcB := NewService(fsB, nil)

	if _, err := svcA.CreateQuestion(&Question{Identity: "A", Kind: "k", Body: "a"}); err != nil {
		t.Fatalf("A create: %v", err)
	}
	if _, err := svcB.CreateQuestion(&Question{Identity: "B", Kind: "k", Body: "b"}); err != nil {
		t.Fatalf("B create: %v", err)
	}
	if _, err := svcA.CreateQuestion(&Question{Identity: "C", Kind: "k", Body: "c"}); err != nil {
		t.Fatalf("A create 2: %v", err)
	}

	fsC, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open C: %v", err)
	}
	all, err := NewService(fsC, nil).ListQuestions(StatePending)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("questions = %v, want 3 (a lost update dropped a record)", idsOf(all))
	}
}

// TestFileStoreAnswerVisibleAcrossStores proves an answer written by one store is
// observed by another that shares the path.
func TestFileStoreAnswerVisibleAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fsA, _ := OpenFileStore(path)
	q, err := NewService(fsA, nil).CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := NewService(fsA, nil).CreateAnswer(&Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatalf("answer: %v", err)
	}

	fsB, _ := OpenFileStore(path)
	_, answers, err := NewService(fsB, nil).ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("read B: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("answers seen by B = %d, want 1", len(answers))
	}
}
