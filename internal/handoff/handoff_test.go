package handoff

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func doneTask(id string) *domain.Task {
	return &domain.Task{ID: id, Title: id, Status: domain.DONE, MaxAttempts: 1}
}

func fixedNow() time.Time { return time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC) }

type fakeStore struct {
	saved map[string]Record
	order []string
	err   error
}

func (s *fakeStore) SaveHandoff(record Record) error {
	if s.err != nil {
		return s.err
	}
	if s.saved == nil {
		s.saved = map[string]Record{}
	}
	if _, ok := s.saved[record.TaskID]; !ok {
		s.order = append(s.order, record.TaskID)
	}
	s.saved[record.TaskID] = record
	return nil
}

type fakeCompressor struct {
	result CompressedContext
	err    error
	calls  int
	last   ContextBundle
}

func (c *fakeCompressor) Compress(_ context.Context, input ContextBundle) (CompressedContext, error) {
	c.calls++
	c.last = input
	if c.err != nil {
		return CompressedContext{}, c.err
	}
	return c.result, nil
}

// --- capsule ---

func TestBuildDeterministicCapsule(t *testing.T) {
	capsule, err := NewBuilder().Build(doneTask("T010"), Facts{
		Summary:      "  added task runner  ",
		Changes:      []string{" added task runner ", "added task runner", "", "added RED/GREEN"},
		Decisions:    []string{"only FAIL establishes RED"},
		Files:        []string{"internal/taskrunner/runner.go"},
		Verification: []VerificationSummary{{Check: "unit", Status: "PASS"}, {Check: " unit ", Status: "PASS"}},
		CarryForward: []string{"process-tree cancellation"},
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if capsule.TaskID != "T010" || capsule.Result != domain.DONE {
		t.Errorf("task/result = %s/%s, want T010/DONE", capsule.TaskID, capsule.Result)
	}
	if capsule.Summary != "added task runner" {
		t.Errorf("summary = %q, want trimmed", capsule.Summary)
	}
	if !reflect.DeepEqual(capsule.Changes, []string{"added task runner", "added RED/GREEN"}) {
		t.Errorf("changes = %v, want trimmed and de-duplicated", capsule.Changes)
	}
	if !reflect.DeepEqual(capsule.Verification, []VerificationSummary{{Check: "unit", Status: "PASS"}}) {
		t.Errorf("verification = %v, want normalized single entry", capsule.Verification)
	}
}

func TestBuildBoundsLists(t *testing.T) {
	var many []string
	for i := 0; i < maxItems+5; i++ {
		many = append(many, fmt.Sprintf("item-%02d", i))
	}
	capsule, err := NewBuilder().Build(doneTask("T1"), Facts{Changes: many})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(capsule.Changes) != maxItems {
		t.Errorf("changes len = %d, want %d", len(capsule.Changes), maxItems)
	}
}

func TestBuildRequiresCompletedTask(t *testing.T) {
	if _, err := NewBuilder().Build(&domain.Task{ID: "T1", Status: domain.IMPLEMENTING}, Facts{}); err == nil {
		t.Error("expected error for a non-DONE task")
	}
	if _, err := NewBuilder().Build(nil, Facts{}); err == nil {
		t.Error("expected error for a nil task")
	}
}

// --- compressor ---

func TestNoOpCompressorIsIdentity(t *testing.T) {
	in := ContextBundle{Artifacts: []Artifact{
		{Kind: ArtifactDiff, Name: "a.diff", Content: "diff body"},
		{Kind: ArtifactTestLog, Content: "log body"},
	}}
	out, err := NoOpCompressor{}.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	want := "DIFF a.diff\ndiff body\n\nTEST_LOG\nlog body"
	if out.Content != want {
		t.Errorf("content = %q, want %q", out.Content, want)
	}
	if len(out.References) != 0 {
		t.Errorf("references = %v, want none", out.References)
	}
}

// --- manager ---

func newManager(store Store, compressor Compressor, maxBytes int) *Manager {
	m := New(store, compressor, maxBytes)
	m.now = fixedNow
	m.Env = nil
	return m
}

func artifact(size int) Artifact {
	return Artifact{Kind: ArtifactCILog, Content: strings.Repeat("x", size)}
}

func TestManagerUnderBudgetDoesNotCompress(t *testing.T) {
	store := &fakeStore{}
	compressor := &fakeCompressor{}
	manager := newManager(store, compressor, 1000)

	record, err := manager.Complete(context.Background(), doneTask("T1"), Facts{Summary: "done"}, []Artifact{artifact(10)})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if record.Status != StatusNotRequested {
		t.Errorf("status = %s, want NOT_REQUESTED", record.Status)
	}
	if compressor.calls != 0 {
		t.Errorf("compressor calls = %d, want 0", compressor.calls)
	}
	if store.saved["T1"].Capsule.Summary != "done" {
		t.Errorf("capsule not persisted: %+v", store.saved["T1"])
	}
	if !record.CreatedAt.Equal(fixedNow()) {
		t.Errorf("created_at = %v, want %v", record.CreatedAt, fixedNow())
	}
}

func TestManagerOverBudgetCompresses(t *testing.T) {
	store := &fakeStore{}
	compressor := &fakeCompressor{result: CompressedContext{
		Content:    "small",
		References: []Reference{{Kind: ArtifactCILog, Locator: "run/1"}},
	}}
	manager := newManager(store, compressor, 10)

	record, err := manager.Complete(context.Background(), doneTask("T1"), Facts{}, []Artifact{artifact(100)})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if record.Status != StatusCompressed || record.Content != "small" {
		t.Errorf("record = %+v, want COMPRESSED/small", record)
	}
	if len(record.References) != 1 {
		t.Errorf("references = %v, want one", record.References)
	}
	if compressor.calls != 1 {
		t.Errorf("compressor calls = %d, want 1", compressor.calls)
	}
}

func TestManagerCompressionFailureKeepsCapsule(t *testing.T) {
	store := &fakeStore{}
	compressor := &fakeCompressor{err: errors.New("provider down")}
	manager := newManager(store, compressor, 10)

	record, err := manager.Complete(context.Background(), doneTask("T1"), Facts{Summary: "keep me"}, []Artifact{artifact(100)})
	if err != nil {
		t.Fatalf("compression failure must not error: %v", err)
	}
	if record.Status != StatusFailed {
		t.Errorf("status = %s, want FAILED", record.Status)
	}
	if record.Capsule.Summary != "keep me" {
		t.Errorf("capsule lost on failure: %+v", record.Capsule)
	}
	if record.CompressionError == "" {
		t.Error("compression error should be recorded")
	}
	if _, ok := store.saved["T1"]; !ok {
		t.Error("failed handoff should still be persisted")
	}
}

func TestManagerBudgetDisabledNeverCompresses(t *testing.T) {
	compressors := []Compressor{&fakeCompressor{}, NoOpCompressor{}}
	for _, compressor := range compressors {
		store := &fakeStore{}
		record, err := newManager(store, compressor, 0).Complete(context.Background(), doneTask("T1"), Facts{}, []Artifact{artifact(10000)})
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}
		if record.Status != StatusNotRequested {
			t.Errorf("status = %s, want NOT_REQUESTED", record.Status)
		}
	}
}

func TestManagerBuildErrorDoesNotPersist(t *testing.T) {
	store := &fakeStore{}
	_, err := newManager(store, NoOpCompressor{}, 10).Complete(context.Background(), &domain.Task{ID: "T1", Status: domain.IMPLEMENTING}, Facts{}, nil)
	if err == nil {
		t.Fatal("expected error for a non-DONE task")
	}
	if len(store.saved) != 0 {
		t.Errorf("nothing should be persisted on build error, got %v", store.saved)
	}
}

func TestManagerSaveErrorPropagates(t *testing.T) {
	store := &fakeStore{err: errors.New("disk full")}
	if _, err := newManager(store, NoOpCompressor{}, 10).Complete(context.Background(), doneTask("T1"), Facts{}, nil); err == nil {
		t.Error("expected save error to propagate")
	}
}

func TestManagerCancellation(t *testing.T) {
	// Cancelled before any work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &fakeStore{}
	if _, err := newManager(store, NoOpCompressor{}, 10).Complete(ctx, doneTask("T1"), Facts{}, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}

	// Cancelled during compression: no record is persisted.
	compressor := &fakeCompressor{err: context.Canceled}
	store2 := &fakeStore{}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := newManager(store2, compressor, 10).Complete(ctx2, doneTask("T1"), Facts{}, []Artifact{artifact(100)}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if len(store2.saved) != 0 {
		t.Error("cancelled completion must not persist")
	}
}

func TestManagerIdempotentCompletion(t *testing.T) {
	store := &fakeStore{}
	manager := newManager(store, NoOpCompressor{}, 10)
	for i := 0; i < 2; i++ {
		if _, err := manager.Complete(context.Background(), doneTask("T1"), Facts{Summary: "once"}, nil); err != nil {
			t.Fatalf("Complete failed: %v", err)
		}
	}
	if len(store.order) != 1 || len(store.saved) != 1 {
		t.Errorf("expected one logical handoff, got order=%v saved=%d", store.order, len(store.saved))
	}
}

// --- selection ---

func TestSelectDirectDependenciesOnly(t *testing.T) {
	task := &domain.Task{ID: "T3", DependencyIDs: []string{"T1", "T2"}}
	available := []Record{{TaskID: "T1"}, {TaskID: "T2"}, {TaskID: "T9"}}

	selected := Select(task, available)
	if len(selected) != 2 || selected[0].TaskID != "T1" || selected[1].TaskID != "T2" {
		t.Errorf("selected = %v, want [T1 T2]", selected)
	}
	if Select(nil, available) != nil {
		t.Error("Select(nil) should return nil")
	}
}

// --- redaction ---

func TestRedactExcludesSecrets(t *testing.T) {
	env := []string{
		"OPENAI_API_KEY=sk-secret-value",
		"HOME=/Users/example",
		"TINY=abc",
		"MALFORMED",
	}
	content := "key=sk-secret-value home=/Users/example tiny=abc"

	got := Redact(content, env)
	if strings.Contains(got, "sk-secret-value") {
		t.Errorf("secret leaked: %q", got)
	}
	if !strings.Contains(got, "/Users/example") {
		t.Errorf("non-secret value should be preserved: %q", got)
	}
	if !strings.Contains(got, "tiny=abc") {
		t.Errorf("short value should be preserved: %q", got)
	}
}

func TestManagerRedactsBeforePersisting(t *testing.T) {
	store := &fakeStore{}
	manager := newManager(store, NoOpCompressor{}, 1) // over budget: exercise compression too
	manager.Env = []string{"DEPLOY_TOKEN=supersecret"}

	record, err := manager.Complete(context.Background(), doneTask("T1"),
		Facts{Summary: "token supersecret"},
		[]Artifact{{Kind: ArtifactCILog, Content: "log supersecret"}})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if strings.Contains(record.Capsule.Summary, "supersecret") {
		t.Errorf("secret leaked into capsule: %q", record.Capsule.Summary)
	}
	if strings.Contains(store.saved["T1"].Capsule.Summary, "supersecret") {
		t.Errorf("secret leaked into persisted capsule: %q", store.saved["T1"].Capsule.Summary)
	}
	if strings.Contains(record.Content, "supersecret") {
		t.Errorf("secret leaked into compressed content: %q", record.Content)
	}
}

func TestRedactAllReturnsInputWhenNoEnv(t *testing.T) {
	artifacts := []Artifact{{Kind: ArtifactDiff, Content: "unchanged"}}
	if got := RedactAll(artifacts, nil); !reflect.DeepEqual(got, artifacts) {
		t.Errorf("RedactAll with no env = %v, want unchanged", got)
	}
}

func TestManagerBoundsAndRedactsCompressionError(t *testing.T) {
	store := &fakeStore{}
	compressor := &fakeCompressor{err: errors.New("token supersecret " + strings.Repeat("z", 1000))}
	manager := newManager(store, compressor, 1)
	manager.Env = []string{"DEPLOY_TOKEN=supersecret"}

	record, err := manager.Complete(context.Background(), doneTask("T1"), Facts{}, []Artifact{artifact(100)})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if strings.Contains(record.CompressionError, "supersecret") {
		t.Errorf("secret leaked into compression error: %q", record.CompressionError)
	}
	if len(record.CompressionError) > maxErrorLen {
		t.Errorf("compression error not bounded: len=%d", len(record.CompressionError))
	}
}
