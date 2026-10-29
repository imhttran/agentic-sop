package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/store"
)

// notifyFixture seeds a project with initialized state and the given notification
// configuration.
func notifyFixture(t *testing.T, channel, filePath string) string {
	t.Helper()
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

	cfg := "version: 1\nproject:\n  name: \"fixture\"\nnotifications:\n  channel: " + channel + "\n"
	if channel == config.NotifyChannelFile {
		cfg += "  file:\n    path: " + filePath + "\n"
	}
	cfg += "  retry:\n    max_attempts: 3\n    base_delay_ms: 1\n    max_delay_ms: 5\n    request_timeout_seconds: 5\n"
	if err := os.WriteFile(filepath.Join(stateDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

func notifyDeps(dir string) deps {
	return deps{getwd: func() (string, error) { return dir, nil }}
}

// TestNotifyStatusDistinguishesConfigurationFromDelivery proves availability comes
// from observed evidence, not configuration alone.
func TestNotifyStatusDistinguishesConfigurationFromDelivery(t *testing.T) {
	dir := notifyFixture(t, config.NotifyChannelNone, "")
	var out, errb bytes.Buffer
	if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, errb.String())
	}
	doc := map[string]any{}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["availability"] != NotifyUnavailable {
		t.Errorf("availability = %v, want unavailable", doc["availability"])
	}
}

// TestNotifyTestDeliversThroughFileChannel proves the test command enqueues, drains,
// and only reports success on a confirmed delivery, recording it so status flips to
// active.
func TestNotifyTestDeliversThroughFileChannel(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "events.jsonl")
	dir := notifyFixture(t, config.NotifyChannelFile, sink)

	var out, errb bytes.Buffer
	if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("status exit = %d; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), NotifyConfiguredUnverified) {
		t.Fatalf("status before delivery = %s, want configured_unverified", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := runNotify([]string{"test"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("test exit = %d; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "delivered") {
		t.Errorf("test output = %q", out.String())
	}
	data, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	if !strings.Contains(string(data), "\"kind\"") {
		t.Errorf("sink content = %q", string(data))
	}

	out.Reset()
	if code := runNotify([]string{"status", "--json"}, &out, &errb, notifyDeps(dir)); code != exitOK {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out.String(), NotifyActive) {
		t.Errorf("status after delivery = %s, want active", out.String())
	}
}

// TestNotifyDrainWithoutChannelReportsUnavailable proves the drain refuses to
// silently succeed when no channel is configured.
func TestNotifyDrainWithoutChannelReportsUnavailable(t *testing.T) {
	dir := notifyFixture(t, config.NotifyChannelNone, "")
	var out, errb bytes.Buffer
	if code := runNotify([]string{"drain"}, &out, &errb, notifyDeps(dir)); code == exitOK {
		t.Fatal("drain without a channel must not succeed")
	}
	if !strings.Contains(errb.String(), "delivery channel is available") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// TestNotifyTestTwiceCreatesTwoEvents proves each explicit test invocation is its
// own event. A constant identity made the second run deduplicate onto the first
// delivered row, so it claimed nothing and reported "test delivery not confirmed".
func TestNotifyTestTwiceCreatesTwoEvents(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "events.jsonl")
	dir := notifyFixture(t, config.NotifyChannelFile, sink)

	for i := 1; i <= 2; i++ {
		var out, errb bytes.Buffer
		if code := runNotify([]string{"test"}, &out, &errb, notifyDeps(dir)); code != exitOK {
			t.Fatalf("test %d exit = %d; stderr=%s", i, code, errb.String())
		}
		if !strings.Contains(out.String(), "delivered") {
			t.Fatalf("test %d output = %q", i, out.String())
		}
	}

	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	counts, err := st.CountNotificationDeliveriesByStatus()
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	delivered := 0
	for _, c := range counts {
		if c.Status == "DELIVERED" {
			delivered = c.Count
		}
	}
	if delivered != 2 {
		t.Fatalf("delivered = %d, want 2 distinct test events", delivered)
	}

	// The two events must also be distinct in the channel, not one repeated id.
	data, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	ids := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var ev struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("decode sink line: %v", err)
		}
		ids[ev.ID] = true
	}
	if len(ids) != 2 {
		t.Fatalf("distinct delivered event ids = %d, want 2", len(ids))
	}
}
