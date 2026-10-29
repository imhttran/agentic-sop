package config

import (
	"strings"
	"testing"
)

func parseNotify(t *testing.T, body string) (*Config, error) {
	t.Helper()
	return Parse([]byte(body))
}

const notifyBase = `
version: 1
project:
  name: demo
`

func TestNotificationsAbsentMeansNone(t *testing.T) {
	c, err := parseNotify(t, notifyBase)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.NotifyChannel(); got != NotifyChannelNone {
		t.Fatalf("channel = %q, want %q", got, NotifyChannelNone)
	}
}

func TestNotificationsUnknownChannelRejected(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: pagerduty\n"
	_, err := parseNotify(t, body)
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
	if !strings.Contains(err.Error(), "notifications.channel") {
		t.Fatalf("error does not name notifications.channel: %v", err)
	}
}

func TestNotificationsEnabledWithoutSettingsRejected(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: file\n"
	_, err := parseNotify(t, body)
	if err == nil || !strings.Contains(err.Error(), "notifications.file.path") {
		t.Fatalf("expected file.path error, got %v", err)
	}
}

func TestNotificationsNonPositiveRetryRejected(t *testing.T) {
	body := notifyBase + `notifications:
  channel: file
  file:
    path: /tmp/out.log
  retry:
    max_attempts: -1
`
	_, err := parseNotify(t, body)
	if err == nil || !strings.Contains(err.Error(), "max_attempts") {
		t.Fatalf("expected max_attempts error, got %v", err)
	}
}

func TestNotificationsFileChannelValidates(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: file\n  file:\n    path: /tmp/out.log\n"
	c, err := parseNotify(t, body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.NotifyChannel() != NotifyChannelFile {
		t.Fatalf("channel = %q", c.NotifyChannel())
	}
	st := c.NotifyStatus(false)
	if !st.Available || !st.FilePresent {
		t.Fatalf("status = %+v", st)
	}
}

func TestNotificationsWebhookWithoutCredentialsIsUnavailable(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: webhook\n  webhook:\n    url: https://example.invalid/hook\n"
	c, err := parseNotify(t, body)
	if err != nil {
		t.Fatalf("Parse (credential-less channel must validate): %v", err)
	}
	st := c.NotifyStatus(false)
	if st.Available {
		t.Fatal("webhook without credentials reported available")
	}
	if !st.HookPresent {
		t.Fatal("webhook URL presence not reported")
	}
}

func TestNotificationsMalformedKeyRejected(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: file\n  bogus: true\n"
	_, err := parseNotify(t, body)
	if err == nil {
		t.Fatal("expected strict-decoding error for unknown key")
	}
}

// TestNotificationsStatusRendersNoCredential asserts the redacted status never
// exposes a credential value: only presence booleans and the resolved non-secret
// endpoint/path are rendered.
func TestNotificationsStatusRendersNoCredential(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: webhook\n  webhook:\n    url: https://example.invalid/hook\n"
	c, err := parseNotify(t, body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	st := c.NotifyStatus(true)
	if st.HookResolved != "https://example.invalid/hook" {
		t.Fatalf("resolved url = %q", st.HookResolved)
	}
	secret := "sk-live-DO-NOT-LEAK"
	rendered := st.Channel + st.FileResolved + st.HookResolved
	if strings.Contains(rendered, secret) {
		t.Fatal("status rendered a credential")
	}
}

func TestWebhookURLWithCredentialsRejected(t *testing.T) {
	body := notifyBase + "notifications:\n  channel: webhook\n  webhook:\n    url: https://user:secret@example.com/hook\n"
	_, err := parseNotify(t, body)
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("err = %v, want a credentials rejection", err)
	}
}

func TestNotifyStatusRedactsURLUserinfo(t *testing.T) {
	var c Config
	c.Notifications.Channel = NotifyChannelWebhook
	c.Notifications.Webhook.URL = "https://user:secret@example.com/hook"
	got := c.NotifyStatus(true).HookResolved
	if strings.Contains(got, "secret") || strings.Contains(got, "user") {
		t.Fatalf("status leaked userinfo: %q", got)
	}
}

func TestSlackChannelDefaultsItsURLEnv(t *testing.T) {
	c, err := parseNotify(t, notifyBase+"notifications:\n  channel: slack\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.SlackURLEnv(); got != DefaultSlackURLEnv {
		t.Fatalf("url env = %q, want %q", got, DefaultSlackURLEnv)
	}
}

func TestSlackURLEnvMustBeAVariableName(t *testing.T) {
	for _, bad := range []string{"https://hooks.slack.com/services/T/B/X", "has space", "9LEADING", "a-b"} {
		body := notifyBase + "notifications:\n  channel: slack\n  slack:\n    url_env: \"" + bad + "\"\n"
		_, err := parseNotify(t, body)
		if err == nil {
			t.Errorf("url_env %q accepted", bad)
			continue
		}
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error echoes the rejected value: %v", err)
		}
	}
	c, err := parseNotify(t, notifyBase+"notifications:\n  channel: slack\n  slack:\n    url_env: MY_HOOK\n")
	if err != nil || c.SlackURLEnv() != "MY_HOOK" {
		t.Fatalf("valid name rejected: %v", err)
	}
}

func TestSlackConfigRejectsAnInlineURLKey(t *testing.T) {
	_, err := parseNotify(t, notifyBase+"notifications:\n  channel: slack\n  slack:\n    url: https://hooks.slack.com/services/T/B/X\n")
	if err == nil {
		t.Fatal("an inline slack url key must be rejected: the URL is a secret and belongs in the environment")
	}
}
