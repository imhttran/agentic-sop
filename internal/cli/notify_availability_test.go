package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// notificationTestConfig returns a configuration for the given channel.
func notificationTestConfig(t *testing.T, channel, webhookURL, filePath string) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Notifications.Channel = channel
	cfg.Notifications.Webhook.URL = webhookURL
	cfg.Notifications.File.Path = filePath
	return cfg
}

// TestNotificationAvailabilityValues asserts the three derived values and the
// distinction of unknown from them.
func TestNotificationAvailabilityValues(t *testing.T) {
	unavailable := notificationTestConfig(t, config.NotifyChannelNone, "", "")
	if got := notificationStatusDoc(unavailable, map[string]int{}, "")["availability"]; got != NotifyUnavailable {
		t.Fatalf("no channel = %v, want %s", got, NotifyUnavailable)
	}

	configured := notificationTestConfig(t, config.NotifyChannelWebhook, "https://example.invalid/hook", "")
	doc := notificationStatusDoc(configured, map[string]int{}, "")
	if got := doc["availability"]; got != NotifyConfiguredUnverified {
		t.Fatalf("configured with no delivery = %v, want %s", got, NotifyConfiguredUnverified)
	}
	if doc["active"] != false {
		t.Fatalf("configured_unverified must not be active")
	}

	active := notificationStatusDoc(configured, map[string]int{"DELIVERED": 1}, "")
	if got := active["availability"]; got != NotifyActive {
		t.Fatalf("after a confirmed delivery = %v, want %s", got, NotifyActive)
	}
	if active["active"] != true {
		t.Fatalf("active delivery must report active=true")
	}

	unknown := unknownNotificationStatusDoc(configured)
	if got := unknown["availability"]; got != NotifyUnknown {
		t.Fatalf("unreadable delivery state = %v, want %s", got, NotifyUnknown)
	}
	for _, other := range []string{NotifyUnavailable, NotifyConfiguredUnverified, NotifyActive} {
		if unknown["availability"] == other {
			t.Fatalf("unknown must be distinguishable from %s", other)
		}
	}
	if unknown["active"] != false {
		t.Fatalf("unknown must never report active=true")
	}
}

// TestNotificationStatusDocNeverLeaksCredentials asserts no configured webhook
// URL or file path appears in any availability document.
func TestNotificationStatusDocNeverLeaksCredentials(t *testing.T) {
	const secretURL = "https://hooks.example.invalid/T000/B000/SEKRET"
	const secretPath = "/tmp/agentic-sop-sekret/notifications.jsonl"
	for _, cfg := range []config.Config{
		notificationTestConfig(t, config.NotifyChannelWebhook, secretURL, ""),
		notificationTestConfig(t, config.NotifyChannelFile, "", secretPath),
	} {
		docs := []map[string]any{
			notificationStatusDoc(cfg, map[string]int{}, ""),
			notificationStatusDoc(cfg, map[string]int{"DELIVERED": 1}, ""),
			unknownNotificationStatusDoc(cfg),
		}
		for _, doc := range docs {
			data, err := marshalWorkDoc(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(data, secretURL) || strings.Contains(data, secretPath) {
				t.Fatalf("credential leaked in status document: %s", data)
			}
		}
	}
}
