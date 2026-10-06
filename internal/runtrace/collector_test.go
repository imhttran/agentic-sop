package runtrace

import (
	"context"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
)

func TestCollectorMapsEventsToIterations(t *testing.T) {
	c := NewCollector(func() time.Time { return time.Unix(0, 0).UTC() })
	ts := time.Unix(1000, 0).UTC()
	c.Emit(activity.Event{Stage: activity.StageStart, Action: "Add widget", Timestamp: ts})
	c.Emit(activity.Event{Stage: activity.StageDiscover, Action: "reading", Detail: "internal/x.go", Timestamp: ts})
	c.Emit(activity.Event{Stage: activity.StageChange, Action: "editing", Detail: "internal/x.go", Timestamp: ts})
	c.Emit(activity.Event{Stage: activity.StageValidate, Action: "go test ./...", Timestamp: ts})

	its := c.Iterations()
	if len(its) != 4 {
		t.Fatalf("iterations = %d, want 4", len(its))
	}
	for i, it := range its {
		if it.Sequence != i+1 {
			t.Errorf("iteration %d sequence = %d, want %d", i, it.Sequence, i+1)
		}
	}
	if its[1].RepositoryMutation {
		t.Errorf("a DISCOVER read must not be a repository mutation")
	}
	if !its[2].RepositoryMutation {
		t.Errorf("a CHANGE event must be a repository mutation")
	}
	if got := its[2].ChangedFiles; len(got) != 1 || got[0] != "internal/x.go" {
		t.Errorf("change changed files = %v, want [internal/x.go]", got)
	}
	if c.Mutations() != 1 {
		t.Errorf("mutations = %d, want 1", c.Mutations())
	}
}

func TestCollectorNarrationAndDenialAreNotProgress(t *testing.T) {
	c := NewCollector(time.Now)
	c.Emit(activity.Event{Stage: activity.StageImplement, Action: "thinking"})
	c.Emit(activity.Event{Stage: activity.StageFinalize, Action: "preparing outcome"})
	if c.Mutations() != 0 {
		t.Errorf("narration must not be a mutation, got %d", c.Mutations())
	}
	for _, it := range c.Iterations() {
		if it.RepositoryMutation {
			t.Errorf("iteration %q must not be a mutation", it.Phase)
		}
	}
}

func TestCollectorIsBounded(t *testing.T) {
	c := NewCollector(time.Now)
	for i := 0; i < maxIterations+100; i++ {
		c.Emit(activity.Event{Stage: activity.StageDiscover, Action: "reading"})
	}
	if got := len(c.Iterations()); got != maxIterations {
		t.Errorf("iterations = %d, want capped at %d", got, maxIterations)
	}
}

func TestCollectorContextAndNilSafety(t *testing.T) {
	c := NewCollector(time.Now)
	ctx := WithCollector(context.Background(), c)
	if CollectorFromContext(ctx) != c {
		t.Errorf("CollectorFromContext did not return the attached collector")
	}
	if CollectorFromContext(context.Background()) != nil {
		t.Errorf("CollectorFromContext returned a collector for a bare context")
	}
	var nilC *Collector
	nilC.Emit(activity.Event{Stage: activity.StageChange}) // must not panic
	if nilC.Iterations() != nil || nilC.Mutations() != 0 {
		t.Errorf("nil collector must be a safe no-op")
	}
	if WithCollector(context.Background(), nil) == nil {
		t.Errorf("WithCollector(nil) must return a usable context")
	}
}
