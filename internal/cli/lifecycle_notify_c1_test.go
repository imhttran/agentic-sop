package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEmitTransitionAppliesC1InventoryToNotification checks that an enabled C1
// inventory value is removed from the emitted event's descriptive text, and that
// an enabled policy whose inventory cannot be loaded emits nothing.
func TestEmitTransitionAppliesC1InventoryToNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 0600 inventory is Unix-only")
	}
	const value = "alpha-value-123"
	project := t.TempDir()
	inventory := filepath.Join(t.TempDir(), "sensitive-values.txt")
	if err := os.WriteFile(inventory, []byte(value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCLIRedactionConfig(t, project, inventory)

	sink := NewMemoryEventSink()
	prev := setLifecycleSink(sink)
	defer setLifecycleSink(prev)

	req := EmitRequest{
		Project:    "p",
		TaskID:     "t",
		Attempt:    1,
		Transition: TransitionFailed,
		Reason:     "the step printed " + value,
		NextAction: "rerun the task",
	}
	if _, _, err := emitTransition(project, req); err != nil {
		t.Fatalf("emitTransition: %v", err)
	}
	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if strings.Contains(events[0].Reason, value) {
		t.Fatalf("C1 inventory value reached the event: %q", events[0].Reason)
	}

	// An enabled policy with a missing inventory must fail closed: no event.
	writeCLIRedactionConfig(t, project, filepath.Join(t.TempDir(), "missing.txt"))
	if _, _, err := emitTransition(project, req); err == nil {
		t.Fatal("emitTransition with an unloadable inventory returned no error")
	}
	if sink.Len() != 1 {
		t.Fatalf("unloadable inventory emitted an event: %d events", sink.Len())
	}
}
