package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/store"
)

// timeZero and timeNow are fixed instants for enqueue and claim calls.
var (
	timeZero = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	timeNow  = time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)
)

// notifyWebhookFixture seeds a project configured with a webhook channel whose
// URL is a credential-like value. It returns the project dir and the secret so a
// test can assert the secret never appears in a status response.
func notifyWebhookFixture(t *testing.T) (string, string) {
	t.Helper()
	secret := "https://hooks.example.test/secret-webhook-token-abc123"
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
		"  webhook:\n    url: " + secret + "\n" +
		"  retry:\n    max_attempts: 3\n    base_delay_ms: 1\n    max_delay_ms: 5\n    request_timeout_seconds: 5\n"
	if err := os.WriteFile(filepath.Join(stateDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir, secret
}

// notifyStatusJSON runs `sop notify status --json` and decodes the document.
func notifyStatusJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	var out, errb bytes.Buffer
	if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("status exit = %d; stderr=%s", code, errb.String())
	}
	doc := map[string]any{}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return doc
}

// recordConfirmedDelivery enqueues one event and marks its delivery DELIVERED so
// the store holds observed delivery evidence.
func recordConfirmedDelivery(t *testing.T, dir, channel string) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	event := store.NotificationEvent{ID: "evt-confirmed", Version: 1, Kind: "test", Project: "fixture"}
	if err := st.EnqueueNotification(event, []string{channel}, 3, timeZero, timeZero); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := st.ClaimNotificationDeliveries(10, timeNow)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d, want 1", len(claimed))
	}
	if err := st.RecordDeliveryOutcome(claimed[0].ClaimToken, store.DeliveryResultDelivered, "", timeNow, timeNow); err != nil {
		t.Fatalf("record delivered: %v", err)
	}
}

// TestNotifyStatusConfiguredWithoutDeliveryIsUnverified proves configuration alone
// never reports active.
func TestNotifyStatusConfiguredWithoutDeliveryIsUnverified(t *testing.T) {
	dir, secret := notifyWebhookFixture(t)
	doc := notifyStatusJSON(t, dir)
	if doc["availability"] != NotifyConfiguredUnverified {
		t.Fatalf("availability = %v, want %s", doc["availability"], NotifyConfiguredUnverified)
	}
	reason, _ := doc["reason"].(string)
	if !strings.Contains(reason, "sop notify test") {
		t.Errorf("reason = %q, want it to name sop notify test", reason)
	}
	data, _ := json.Marshal(doc)
	if strings.Contains(string(data), secret) {
		t.Errorf("status JSON leaks the configured credential: %s", string(data))
	}
	if doc["webhook_configured"] != true {
		t.Errorf("webhook_configured = %v, want true", doc["webhook_configured"])
	}
}

// TestNotifyStatusConfirmedDeliveryIsActive proves a confirmed delivery moves
// availability to active.
func TestNotifyStatusConfirmedDeliveryIsActive(t *testing.T) {
	dir, _ := notifyWebhookFixture(t)
	recordConfirmedDelivery(t, dir, config.NotifyChannelWebhook)
	doc := notifyStatusJSON(t, dir)
	if doc["availability"] != NotifyActive {
		t.Fatalf("availability = %v, want %s", doc["availability"], NotifyActive)
	}
	if doc["reason"] != "" {
		t.Errorf("reason = %v, want empty for active", doc["reason"])
	}
}

// TestNotifyStatusRemovedConfigurationIsUnavailable proves removing configuration
// reports unavailable with an actionable reason.
func TestNotifyStatusRemovedConfigurationIsUnavailable(t *testing.T) {
	dir := notifyFixture(t, config.NotifyChannelNone, "")
	doc := notifyStatusJSON(t, dir)
	if doc["availability"] != NotifyUnavailable {
		t.Fatalf("availability = %v, want %s", doc["availability"], NotifyUnavailable)
	}
	reason, _ := doc["reason"].(string)
	if strings.TrimSpace(reason) == "" {
		t.Fatal("reason is empty for unavailable; want an actionable reason")
	}
}

// TestNotifyStatusShowsCountsAndLastError proves pending/exhausted counts and the
// last delivery error reach the status document.
func TestNotifyStatusShowsCountsAndLastError(t *testing.T) {
	dir := notifyFixture(t, config.NotifyChannelFile, filepath.Join(t.TempDir(), "sink.jsonl"))
	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	event := store.NotificationEvent{ID: "evt-fail", Version: 1, Kind: "test", Project: "fixture"}
	if err := st.EnqueueNotification(event, []string{config.NotifyChannelFile}, 1, timeZero, timeZero); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := st.ClaimNotificationDeliveries(10, timeNow)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := st.RecordDeliveryOutcome(claimed[0].ClaimToken, store.DeliveryResultExhausted, "sink write failed", timeNow, timeNow); err != nil {
		t.Fatalf("record exhausted: %v", err)
	}

	doc := notifyStatusJSON(t, dir)
	if got := intField(doc, "exhausted"); got != 1 {
		t.Errorf("exhausted = %d, want 1", got)
	}
	if got, _ := doc["last_error"].(string); got != "sink write failed" {
		t.Errorf("last_error = %q, want %q", got, "sink write failed")
	}

	// MCP sop_work_status carries the same observed evidence.
	out, err := workStatus(dir, defaultWorkEnv(), "")
	if err != nil {
		t.Fatalf("workStatus: %v", err)
	}
	notif, ok := out["notifications"].(map[string]any)
	if !ok {
		t.Fatalf("work status has no notifications object: %v", out)
	}
	if got := intField(notif, "exhausted"); got != 1 {
		t.Errorf("MCP exhausted = %d, want 1", got)
	}
	if got, _ := notif["last_error"].(string); got != "sink write failed" {
		t.Errorf("MCP last_error = %q, want %q", got, "sink write failed")
	}
}

// TestNotifyStatusRepeatedReadsAreReadOnly proves repeated status reads create no
// events and mutate no delivery state.
func TestNotifyStatusRepeatedReadsAreReadOnly(t *testing.T) {
	dir, _ := notifyWebhookFixture(t)
	recordConfirmedDelivery(t, dir, config.NotifyChannelWebhook)
	path := filepath.Join(dir, config.DirName, "state.db")

	before := notifyStateSnapshot(t, path)
	for i := 0; i < 3; i++ {
		notifyStatusJSON(t, dir)
		var out, errb bytes.Buffer
		if code := runNotify([]string{"status"}, &out, &errb, notifyDeps(dir)); code != exitOK {
			t.Fatalf("text status exit = %d; stderr=%s", code, errb.String())
		}
		if _, err := workStatus(dir, defaultWorkEnv(), ""); err != nil {
			t.Fatalf("workStatus: %v", err)
		}
		if _, err := mcpStatus(dir); err != nil {
			t.Fatalf("mcpStatus: %v", err)
		}
	}
	after := notifyStateSnapshot(t, path)
	if before != after {
		t.Fatalf("repeated reads changed notification state: before=%s after=%s", before, after)
	}
}

// notifyStateSnapshot renders event count and delivery statuses for comparison.
func notifyStateSnapshot(t *testing.T, path string) string {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	events, err := st.CountNotificationEvents()
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	counts, err := st.CountNotificationDeliveriesByStatus()
	if err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	data, _ := json.Marshal(map[string]any{"events": events, "deliveries": counts})
	return string(data)
}
