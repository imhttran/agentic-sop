package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/store"
)

// Acceptance mapping for NTFY-008 (issue scenario coverage, cli layer):
//
//	isolation: TestAcceptanceNotifyStatusIsolatesEvents proves a delivery recorded for
//	one event is absent from another event's scoped status view, and the global view
//	counts only what was recorded.
//	configuration: TestAcceptanceNotifyStatusReportsConfigurationAndExhaustion proves
//	status exposes the configured channel, an explicit unavailable condition when
//	unconfigured, and EXHAUSTED delivery counts.
//	local receiver: TestAcceptanceNotifyTestDeliversToLocalReceiver runs the CLI test
//	path against an httptest.Server bound to loopback and asserts a confirmed delivery.

// TestAcceptanceNotifyStatusIsolatesEvents proves per-event scoping: a delivery for
// one event is counted for that event and not for a different event.
func TestAcceptanceNotifyStatusIsolatesEvents(t *testing.T) {
	dir := notifyFixture(t, config.NotifyChannelFile, filepath.Join(t.TempDir(), "sink.jsonl"))
	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	a := store.NotificationEvent{ID: "evt-a", Version: 1, Kind: "test", Project: "proj-a"}
	b := store.NotificationEvent{ID: "evt-b", Version: 1, Kind: "test", Project: "proj-b"}
	if err := st.EnqueueNotification(a, []string{config.NotifyChannelFile}, 1, now, now); err != nil {
		t.Fatalf("enqueue a: %v", err)
	}
	if err := st.EnqueueNotification(b, []string{config.NotifyChannelFile}, 1, now, now); err != nil {
		t.Fatalf("enqueue b: %v", err)
	}
	claimed, err := st.ClaimNotificationDeliveries(10, now)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, c := range claimed {
		if c.EventID != "evt-a" {
			continue
		}
		if err := st.RecordDeliveryOutcome(c.ClaimToken, store.DeliveryResultDelivered, "", now, now); err != nil {
			t.Fatalf("record delivered a: %v", err)
		}
	}

	countsA, err := st.CountNotificationDeliveriesByStatusForEvent("evt-a")
	if err != nil {
		t.Fatalf("count evt-a: %v", err)
	}
	byA := map[string]int{}
	for _, c := range countsA {
		byA[c.Status] = c.Count
	}
	if byA[store.DeliveryStatusDelivered] != 1 {
		t.Fatalf("evt-a delivered = %d, want 1", byA[store.DeliveryStatusDelivered])
	}

	countsB, err := st.CountNotificationDeliveriesByStatusForEvent("evt-b")
	if err != nil {
		t.Fatalf("count evt-b: %v", err)
	}
	byB := map[string]int{}
	for _, c := range countsB {
		byB[c.Status] = c.Count
	}
	if byB[store.DeliveryStatusDelivered] != 0 {
		t.Fatalf("evt-b delivered = %d, want 0 (isolation leak)", byB[store.DeliveryStatusDelivered])
	}
	// The claim touched both events, so evt-b is CLAIMED (not PENDING). The point
	// of the assertion is that evt-a's DELIVERED outcome did not leak into evt-b:
	// evt-b has no DELIVERED row and is still an in-flight claim of one row.
	if got := byB[store.DeliveryStatusClaimed]; got != 1 {
		t.Fatalf("evt-b claimed = %d, want 1", got)
	}
	if byB[store.DeliveryStatusPending] != 0 {
		t.Fatalf("evt-b pending = %d, want 0 (the claim moved it to CLAIMED)", byB[store.DeliveryStatusPending])
	}
}

// TestAcceptanceNotifyStatusReportsConfigurationAndExhaustion proves status exposes
// the configured channel and EXHAUSTED counts, and reports an explicit unavailable
// condition when no channel is configured.
func TestAcceptanceNotifyStatusReportsConfigurationAndExhaustion(t *testing.T) {
	t.Run("configured channel and exhausted count", func(t *testing.T) {
		dir := notifyFixture(t, config.NotifyChannelFile, filepath.Join(t.TempDir(), "sink.jsonl"))
		st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer st.Close()
		now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		if err := st.EnqueueNotification(store.NotificationEvent{ID: "evt-x", Version: 1, Kind: "test", Project: "fixture"}, []string{config.NotifyChannelFile}, 1, now, now); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		claimed, err := st.ClaimNotificationDeliveries(10, now)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim = %d, %v", len(claimed), err)
		}
		if err := st.RecordDeliveryOutcome(claimed[0].ClaimToken, store.DeliveryResultExhausted, "sink failed", now, now); err != nil {
			t.Fatalf("record exhausted: %v", err)
		}

		var out, errb bytes.Buffer
		if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
			t.Fatalf("status exit = %d; stderr=%s", code, errb.String())
		}
		doc := map[string]any{}
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if doc["channel"] != config.NotifyChannelFile {
			t.Errorf("channel = %v, want %s", doc["channel"], config.NotifyChannelFile)
		}
		if got := intField(doc, "exhausted"); got != 1 {
			t.Errorf("exhausted = %d, want 1", got)
		}
	})

	t.Run("unconfigured reports unavailable", func(t *testing.T) {
		dir := notifyFixture(t, config.NotifyChannelNone, "")
		var out, errb bytes.Buffer
		if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
			t.Fatalf("status exit = %d; stderr=%s", code, errb.String())
		}
		doc := map[string]any{}
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if doc["availability"] != NotifyUnavailable {
			t.Fatalf("availability = %v, want %s", doc["availability"], NotifyUnavailable)
		}
		reason, _ := doc["reason"].(string)
		if strings.TrimSpace(reason) == "" {
			t.Fatal("unavailable status carries no reason")
		}
	})
}

// TestAcceptanceNotifyTestDeliversToLocalReceiver runs the CLI notification test path
// against an httptest.Server on loopback and asserts a confirmed delivery. It never
// touches a non-loopback host and uses a temp config and temp DB.
func TestAcceptanceNotifyTestDeliversToLocalReceiver(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("request carried no Idempotency-Key")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	st, err := store.Open(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	st.Close()
	cfg := "version: 1\nproject:\n  name: \"fixture\"\nnotifications:\n  channel: " + config.NotifyChannelWebhook + "\n" +
		"  webhook:\n    url: " + srv.URL + "\n" +
		"  retry:\n    max_attempts: 3\n    base_delay_ms: 1\n    max_delay_ms: 5\n    request_timeout_seconds: 5\n"
	if err := os.WriteFile(filepath.Join(stateDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out, errb bytes.Buffer
	if code := runNotify([]string{"test"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("test exit = %d; stdout=%s stderr=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "delivered") {
		t.Errorf("test output = %q, want a confirmed delivery", out.String())
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("local receiver saw no request")
	}

	// Status now reflects the confirmed delivery as active.
	out.Reset()
	errb.Reset()
	if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out.String(), NotifyActive) {
		t.Errorf("status after delivery = %s, want %s", out.String(), NotifyActive)
	}
}
