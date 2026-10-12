package commandprofile

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestAdapterObservabilityMetricsExist proves the connector emits ONE class-only
// observation per invocation: latency, an outcome class, and validity — never a
// prompt, payload, endpoint, or credential.
func TestAdapterObservabilityMetricsExist(t *testing.T) {
	var events []Event
	c, err := New(Options{
		Registry: testRegistry(),
		Argv:     []string{os.Args[0], "-test.run=^TestAdapterHelperProcess$", "--"},
		Provider: "julia",
		Model:    "julia-1",
		Timeout:  10 * time.Second,
		Observer: func(e Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Setenv("GO_WANT_ADAPTER_HELPER", "1")

	// One success, one provider failure.
	t.Setenv("ADAPTER_MODE", "success")
	t.Setenv("ADAPTER_CHOICE", "high")
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity"}); err != nil {
		t.Fatalf("success path: %v", err)
	}
	t.Setenv("ADAPTER_MODE", "unavailable")
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity"}); err == nil {
		t.Fatal("unavailable path should fail")
	}

	if len(events) != 2 {
		t.Fatalf("observations = %d, want 1 per invocation (2)", len(events))
	}
	allowed := map[string]bool{"ok": true, "invalid": true, "unsupported": true, "indeterminate": true, "timeout": true, "canceled": true, "provider_failure": true}
	for i, e := range events {
		if !allowed[e.Outcome] {
			t.Errorf("event %d has an unknown outcome class %q", i, e.Outcome)
		}
		if e.LatencyMS < 0 {
			t.Errorf("event %d has a negative latency", i)
		}
	}
	if events[0].Outcome != "ok" || !events[0].Valid {
		t.Errorf("success event = %+v, want ok/valid", events[0])
	}
	if events[1].Outcome != "provider_failure" || events[1].Valid {
		t.Errorf("failure event = %+v, want provider_failure/invalid", events[1])
	}
}
