package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// LLM08 lifecycle-producer checks for the notification gap. Every production
// producer funnels through emitTransition*, so these tests drive that seam with a
// synthetic canary in a secret-named environment variable and prove the value is
// absent from the in-memory event, the durable outbox row, and the delivered
// payload. They use a local file transport only and send no real message.

const (
	llm08EnvSecretName  = "SOP_LLM08_NOTIFY_TOKEN"
	llm08EnvSecretValue = "llm08env4f9a1c2b7d3e8a6c"
)

// TestLLM08LifecycleEmissionsRedactExactEnvSecret proves completed, failed, and
// waiting_for_human emissions all redact an exact environment-secret value.
func TestLLM08LifecycleEmissionsRedactExactEnvSecret(t *testing.T) {
	t.Setenv(llm08EnvSecretName, llm08EnvSecretValue)

	sink := NewMemoryEventSink()
	prev := setLifecycleSink(sink)
	defer setLifecycleSink(prev)

	dir := t.TempDir()
	if _, _, err := emitTransitionCompleted(dir, "T1", "job-1", 1,
		"completed while holding "+llm08EnvSecretValue,
		"rotate "+llm08EnvSecretValue+" then merge"); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if _, _, err := emitTransitionFailed(dir, "T1", "job-1", 1, false,
		"validation failed with "+llm08EnvSecretValue,
		"rotate "+llm08EnvSecretValue+" and retry"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	appr := &domain.ApprovalRequest{
		ID:     "APR-1",
		TaskID: "T1",
		Status: domain.ApprovalPending,
		Reason: "the gate needs " + llm08EnvSecretValue,
	}
	if _, _, err := emitTransitionApproval(dir, "T1", 1, appr); err != nil {
		t.Fatalf("approval: %v", err)
	}

	if sink.Len() != 3 {
		t.Fatalf("emitted %d events, want 3", sink.Len())
	}
	for _, e := range sink.Events() {
		for field, v := range map[string]string{
			"reason":             e.Reason,
			"next_action":        e.NextAction,
			"outcome":            e.Outcome,
			"resolution_command": e.ResolutionCommand,
		} {
			if strings.Contains(v, llm08EnvSecretValue) {
				t.Fatalf("%s event %s leaked the environment secret: %q", e.Kind, field, v)
			}
		}
	}
}

// TestLLM08PersistedOutboxAndDeliveredPayloadOmitEnvSecret proves the value never
// reaches the durable outbox row or the file channel payload, while ordinary prose
// in the same fields survives.
func TestLLM08PersistedOutboxAndDeliveredPayloadOmitEnvSecret(t *testing.T) {
	t.Setenv(llm08EnvSecretName, llm08EnvSecretValue)

	sinkPath := filepath.Join(t.TempDir(), "events.jsonl")
	dir := notifyFixture(t, config.NotifyChannelFile, sinkPath)

	prev := setLifecycleSink(nil)
	defer setLifecycleSink(prev)

	event, recorded, err := emitTransitionFailed(dir, "T1", "job-1", 1, false,
		"validation failed with "+llm08EnvSecretValue,
		"rotate "+llm08EnvSecretValue+" and retry")
	if err != nil {
		t.Fatalf("emit failed event: %v", err)
	}
	if !recorded {
		t.Fatal("the failed event was not recorded")
	}

	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	stored, err := st.GetNotificationEvent(event.ID)
	st.Close()
	if err != nil {
		t.Fatalf("read stored event: %v", err)
	}
	for field, v := range map[string]string{
		"reason":             stored.Reason,
		"next_action":        stored.NextAction,
		"outcome":            stored.Outcome,
		"resolution_command": stored.ResolutionCommand,
	} {
		if strings.Contains(v, llm08EnvSecretValue) {
			t.Fatalf("persisted outbox %s leaked the environment secret: %q", field, v)
		}
	}

	var out, errb bytes.Buffer
	if code := runNotify([]string{"drain"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("drain exit = %d; stderr=%s", code, errb.String())
	}
	payload, err := os.ReadFile(sinkPath)
	if err != nil {
		t.Fatalf("read delivered payload: %v", err)
	}
	if strings.Contains(string(payload), llm08EnvSecretValue) {
		t.Fatalf("delivered payload leaked the environment secret: %s", payload)
	}
	if !strings.Contains(string(payload), "validation failed with") {
		t.Fatalf("delivered payload lost ordinary prose: %s", payload)
	}
	if !strings.Contains(string(payload), "[REDACTED]") {
		t.Fatalf("delivered payload is not marked redacted: %s", payload)
	}
}
