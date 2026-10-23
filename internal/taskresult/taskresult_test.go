package taskresult

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agent-sdlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPublishIsAcceptedAndDurable proves a publication is written once and read
// back with the binding it was published against.
func TestPublishIsAcceptedAndDurable(t *testing.T) {
	dir := fixture(t)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	res, err := Publish(dir, PublishInput{
		TaskID: "T001", Attempt: 2, PlanRevision: "plan-x@abc",
		Kind: KindTest, Summary: "go test ./internal/cli passed",
		Evidence:    []string{"internal/cli/result_test.go"},
		PublishedBy: "zed", Now: now,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.Outcome != OutcomeAccepted {
		t.Fatalf("outcome = %s, want ACCEPTED (%s)", res.Outcome, res.Reason)
	}

	got, err := List(dir, "T001")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("publications = %d, want 1", len(got))
	}
	p := got[0]
	if p.Attempt != 2 || p.PlanRevision != "plan-x@abc" || p.Kind != KindTest || p.PublishedBy != "zed" {
		t.Errorf("publication = %+v, want the published binding", p)
	}
	if !p.CreatedAt.Equal(now) {
		t.Errorf("created_at = %s, want %s", p.CreatedAt, now)
	}
	if _, err := os.Stat(filepath.Join(ResultsDir(dir, "T001"), p.ID+".json")); err != nil {
		t.Errorf("publication file missing: %v", err)
	}
}

// TestPublishIsIdempotent proves a re-delivered publication creates no second
// record, both by idempotency key and by identical content.
func TestPublishIsIdempotent(t *testing.T) {
	cases := []struct {
		name string
		in   PublishInput
	}{
		{"same key", PublishInput{TaskID: "T001", Attempt: 1, Kind: KindProgress, Summary: "s", IdempotencyKey: "k1", PlanRevision: "p@1"}},
		{"same content", PublishInput{TaskID: "T001", Attempt: 1, Kind: KindProgress, Summary: "s", PlanRevision: "p@1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			if res, err := Publish(dir, tc.in); err != nil || res.Outcome != OutcomeAccepted {
				t.Fatalf("first publish = %s, %v; want ACCEPTED", res.Outcome, err)
			}
			res, err := Publish(dir, tc.in)
			if err != nil {
				t.Fatalf("second publish: %v", err)
			}
			if res.Outcome != OutcomeDuplicate {
				t.Errorf("second publish = %s, want DUPLICATE", res.Outcome)
			}
			got, _ := List(dir, "T001")
			if len(got) != 1 {
				t.Errorf("publications = %d, want exactly 1", len(got))
			}
		})
	}
}

// TestPublishRefusesStalePlanRevision proves a publication made against an older
// plan revision writes nothing.
func TestPublishRefusesStalePlanRevision(t *testing.T) {
	dir := fixture(t)
	res, err := Publish(dir, PublishInput{
		TaskID: "T001", Attempt: 1, Kind: KindBlocker, Summary: "blocked",
		PlanRevision: "plan-x@new", ExpectedPlanRevision: "plan-x@old",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.Outcome != OutcomeStale {
		t.Fatalf("outcome = %s, want STALE", res.Outcome)
	}
	if got, _ := List(dir, "T001"); len(got) != 0 {
		t.Errorf("stale publication wrote %d record(s), want 0", len(got))
	}
}

// TestPublishRefusesStaleAttempt proves a publication made against an older attempt
// writes nothing.
func TestPublishRefusesStaleAttempt(t *testing.T) {
	dir := fixture(t)
	res, err := Publish(dir, PublishInput{
		TaskID: "T001", Attempt: 3, Kind: KindProgress, Summary: "s", ExpectedAttempt: 2,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.Outcome != OutcomeStale || !strings.Contains(res.Reason, "attempt changed") {
		t.Fatalf("outcome = %s reason = %q, want STALE with the attempt reason", res.Outcome, res.Reason)
	}
	if got, _ := List(dir, "T001"); len(got) != 0 {
		t.Errorf("stale publication wrote %d record(s), want 0", len(got))
	}
}

// TestPublishRejectsUnknownKindAndEmptySummary pins the fail-closed validation.
func TestPublishRejectsUnknownKindAndEmptySummary(t *testing.T) {
	dir := fixture(t)
	cases := []PublishInput{
		{TaskID: "T001", Attempt: 1, Kind: Kind("DONE"), Summary: "x"},
		{TaskID: "T001", Attempt: 1, Kind: KindProgress, Summary: "   "},
		{TaskID: "../escape", Attempt: 1, Kind: KindProgress, Summary: "x"},
	}
	for _, in := range cases {
		res, err := Publish(dir, in)
		if err == nil && res.Outcome != OutcomeRejected {
			t.Errorf("publish(%+v) = %s, want REJECTED or an error", in, res.Outcome)
		}
		if err != nil && !strings.Contains(err.Error(), "invalid publication") {
			t.Errorf("publish(%+v) error = %v, want an invalid-publication error", in, err)
		}
	}
}

// TestListIsDeterministic proves list order is stable by creation time then id.
func TestListIsDeterministic(t *testing.T) {
	dir := fixture(t)
	base := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for i, kind := range AllKinds() {
		if _, err := Publish(dir, PublishInput{
			TaskID: "T001", Attempt: 1, Kind: kind, Summary: string(kind), Now: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("publish %s: %v", kind, err)
		}
	}
	got, err := List(dir, "T001")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != len(AllKinds()) {
		t.Fatalf("publications = %d, want %d", len(got), len(AllKinds()))
	}
	for i, kind := range AllKinds() {
		if got[i].Kind != kind {
			t.Errorf("publication[%d] = %s, want %s", i, got[i].Kind, kind)
		}
	}
}
