package config

import "testing"

// TestNotifyChannelEnvEnablesOnlyWhenProjectIsNone proves the environment can switch
// on a channel the project left at none, and cannot override an explicit project channel.
func TestNotifyChannelEnvEnablesOnlyWhenProjectIsNone(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	const base = "project:\n  name: t\nnotifications:\n  channel: none\n  webhook:\n    payload: slack\n"
	withURL := map[string]string{NotifyChannelEnv: "webhook", NotifyWebhookURLEnv: "https://hooks.example.test/x"}

	c, err := parseWithEnv([]byte(base), env(withURL))
	if err != nil {
		t.Fatalf("env-enabled webhook: %v", err)
	}
	if c.NotifyChannel() != NotifyChannelWebhook {
		t.Fatalf("channel = %q, want webhook", c.NotifyChannel())
	}

	c, err = parseWithEnv([]byte(base), env(nil))
	if err != nil {
		t.Fatalf("committed config without env: %v", err)
	}
	if c.NotifyChannel() != NotifyChannelNone {
		t.Fatalf("channel = %q, want none", c.NotifyChannel())
	}

	explicit := "project:\n  name: t\nnotifications:\n  channel: file\n  file:\n    path: /tmp/n.jsonl\n"
	c, err = parseWithEnv([]byte(explicit), env(withURL))
	if err != nil {
		t.Fatalf("explicit file channel: %v", err)
	}
	if c.NotifyChannel() != NotifyChannelFile {
		t.Fatalf("explicit project channel overridden to %q", c.NotifyChannel())
	}
}
