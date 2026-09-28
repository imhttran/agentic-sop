package activity

import (
	"context"
	"testing"
	"time"
)

// capture records events for assertions.
type capture struct{ events []Event }

func (c *capture) Emit(e Event) { c.events = append(c.events, e) }

func TestRecorderStampsTaskIDAndTime(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := &capture{}
	r := WithClock("TASK-1", c, func() time.Time { return fixed })

	r.Emit(StageChange, "editing", "a.go")

	if len(c.events) != 1 {
		t.Fatalf("events = %d, want 1", len(c.events))
	}
	got := c.events[0]
	want := Event{TaskID: "TASK-1", Stage: StageChange, Action: "editing", Detail: "a.go", Timestamp: fixed}
	if got != want {
		t.Errorf("event = %+v, want %+v", got, want)
	}
}

func TestNilRecorderEmitsNothing(t *testing.T) {
	var r *Recorder
	// Must not panic, and must be observably disabled.
	r.Emit(StagePlan, "generating plan", "")
	if r.Enabled() {
		t.Error("nil recorder reports Enabled")
	}
}

func TestNilSinkDropsEvents(t *testing.T) {
	r := New("TASK-1", nil)
	if r.Enabled() {
		t.Error("recorder with nil sink reports Enabled")
	}
	r.Emit(StagePlan, "generating plan", "") // must not panic
}

func TestFuncSink(t *testing.T) {
	var got []Event
	r := New("T", Func(func(e Event) { got = append(got, e) }))
	r.Emit(StageValidate, "go test ./...", "")
	if len(got) != 1 || got[0].Stage != StageValidate || got[0].Action != "go test ./..." {
		t.Errorf("got = %+v", got)
	}
}

func TestMultiFansOutAndSkipsNil(t *testing.T) {
	a := &capture{}
	b := &capture{}
	r := New("T", Multi{nil, a, b})
	r.Emit(StageChange, "editing", "x.go")
	if len(a.events) != 1 || len(b.events) != 1 {
		t.Fatalf("a=%d b=%d, want 1 each", len(a.events), len(b.events))
	}
	if a.events[0] != b.events[0] {
		t.Errorf("sinks received different events: %+v vs %+v", a.events[0], b.events[0])
	}
}

func TestContextRoundTrip(t *testing.T) {
	if FromContext(context.Background()) != nil {
		t.Error("FromContext(empty) != nil")
	}
	c := &capture{}
	r := New("T", c)
	ctx := WithRecorder(context.Background(), r)
	if FromContext(ctx) != r {
		t.Error("FromContext did not return the recorder")
	}
}

func TestWithRecorderNilIsUnchanged(t *testing.T) {
	ctx := context.Background()
	if WithRecorder(ctx, nil) != ctx {
		t.Error("WithRecorder(ctx, nil) returned a different context")
	}
	if FromContext(WithRecorder(ctx, nil)) != nil {
		t.Error("nil recorder leaked into the context")
	}
}
