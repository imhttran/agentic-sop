package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Notification channels selected by notifications.channel.
const (
	// NotifyChannelNone disables notification delivery. It is the default: an
	// omitted `notifications` block means no channel.
	NotifyChannelNone = "none"
	// NotifyChannelFile writes events to a file sink.
	NotifyChannelFile = "file"
	// NotifyChannelWebhook posts events to an HTTP endpoint.
	NotifyChannelWebhook = "webhook"
	// NotifyChannelSlack posts events to a Slack incoming webhook. The webhook URL
	// is a secret and is read from the environment, never from the config file.
	NotifyChannelSlack = "slack"
)

// DefaultSlackURLEnv is the environment variable that holds the Slack webhook URL
// when notifications.slack.url_env is omitted.
const DefaultSlackURLEnv = "SOP_SLACK_WEBHOOK_URL"

// Bounded retry defaults for the notification delivery policy. They are finite
// so a channel can never retry without bound.
const (
	DefaultNotifyMaxAttempts    = 3
	DefaultNotifyBaseDelayMs    = 500
	DefaultNotifyMaxDelayMs     = 30000
	DefaultNotifyRequestTimeout = 10 // seconds
)

// supportedNotifyChannels is the accepted channel value set, kept here so a bad
// value fails at load time with a message naming the accepted values.
var supportedNotifyChannels = map[string]bool{
	NotifyChannelNone:    true,
	NotifyChannelFile:    true,
	NotifyChannelWebhook: true,
	NotifyChannelSlack:   true,
}

// Notifications is the optional notification configuration block.
//
// It selects a delivery channel, the channel's non-secret settings, and a
// bounded retry policy. It is additive: an omitted block leaves the channel as
// `none`, so existing behavior is unchanged. The block never carries
// credentials: strict decoding rejects an unknown key, so a secret cannot be
// configured here, and the status helpers below expose only presence booleans
// and resolved non-secret values.
type Notifications struct {
	// Channel selects the delivery channel: none, file, or webhook. An omitted
	// value defaults to none.
	Channel string `yaml:"channel"`
	// File holds the file channel's non-secret settings.
	File NotifyFile `yaml:"file"`
	// Webhook holds the webhook channel's non-secret settings.
	Webhook NotifyWebhook `yaml:"webhook"`
	// Slack holds the Slack channel's non-secret settings.
	Slack NotifySlack `yaml:"slack"`
	// Retry bounds delivery retries.
	Retry NotifyRetry `yaml:"retry"`
}

// NotifySlack is the Slack channel's non-secret settings. The webhook URL is a
// secret (its path authorizes posting), so the config names only the environment
// variable that holds it.
type NotifySlack struct {
	// URLEnv names the environment variable holding the webhook URL. Omitted, it
	// is DefaultSlackURLEnv.
	URLEnv string `yaml:"url_env"`
}

// NotifyFile is the file channel's non-secret settings.
type NotifyFile struct {
	// Path is the file to append events to.
	Path string `yaml:"path"`
}

// NotifyWebhook is the webhook channel's non-secret settings. The webhook URL
// and any credentials stay in the environment; only the endpoint URL is policy.
type NotifyWebhook struct {
	// URL is the endpoint the webhook channel posts events to.
	URL string `yaml:"url"`
}

// NotifyRetry is the bounded retry policy. Every bound is finite and must be
// positive on a parsed configuration: a non-positive value is a configuration
// error.
type NotifyRetry struct {
	// MaxAttempts bounds how many times one delivery is attempted.
	MaxAttempts int `yaml:"max_attempts"`
	// BaseDelayMs is the first retry delay in milliseconds.
	BaseDelayMs int `yaml:"base_delay_ms"`
	// MaxDelayMs is the largest retry delay in milliseconds.
	MaxDelayMs int `yaml:"max_delay_ms"`
	// RequestTimeoutSeconds is the per-request timeout in seconds.
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds"`
}

// applyNotifyDefaults fills omitted notification settings. An omitted block
// resolves to channel none; the retry bounds take their finite defaults only
// for a configured channel, so an omitted block stays byte-identical.
func (c *Config) applyNotifyDefaults() {
	if strings.TrimSpace(c.Notifications.Channel) == "" {
		c.Notifications.Channel = NotifyChannelNone
	}
	if c.Notifications.Channel == NotifyChannelNone {
		return
	}
	if c.Notifications.Channel == NotifyChannelSlack && strings.TrimSpace(c.Notifications.Slack.URLEnv) == "" {
		c.Notifications.Slack.URLEnv = DefaultSlackURLEnv
	}
	if c.Notifications.Retry.MaxAttempts == 0 {
		c.Notifications.Retry.MaxAttempts = DefaultNotifyMaxAttempts
	}
	if c.Notifications.Retry.BaseDelayMs == 0 {
		c.Notifications.Retry.BaseDelayMs = DefaultNotifyBaseDelayMs
	}
	if c.Notifications.Retry.MaxDelayMs == 0 {
		c.Notifications.Retry.MaxDelayMs = DefaultNotifyMaxDelayMs
	}
	if c.Notifications.Retry.RequestTimeoutSeconds == 0 {
		c.Notifications.Retry.RequestTimeoutSeconds = DefaultNotifyRequestTimeout
	}
}

// validateNotifications validates the notification block. It never returns a
// credential value: the errors name only the offending setting.
func (c *Config) validateNotifications() error {
	n := c.Notifications
	ch := strings.TrimSpace(n.Channel)
	if !supportedNotifyChannels[ch] {
		return fmt.Errorf("config: unknown notifications.channel %q (want %s)",
			n.Channel, strings.Join(sortedKeys(supportedNotifyChannels), ", "))
	}
	if ch == NotifyChannelNone {
		return nil
	}
	switch ch {
	case NotifyChannelFile:
		if strings.TrimSpace(n.File.Path) == "" {
			return errors.New("config: notifications.channel \"file\" requires notifications.file.path")
		}
	case NotifyChannelWebhook:
		if strings.TrimSpace(n.Webhook.URL) == "" {
			return errors.New("config: notifications.channel \"webhook\" requires notifications.webhook.url")
		}
		if u, err := url.Parse(strings.TrimSpace(n.Webhook.URL)); err == nil && u.User != nil {
			return errors.New("config: notifications.webhook.url must not embed credentials")
		}
	case NotifyChannelSlack:
		if !validEnvName(n.Slack.URLEnv) {
			return errors.New("config: notifications.slack.url_env must be an environment variable name")
		}
	}
	r := n.Retry
	if r.MaxAttempts < 1 {
		return fmt.Errorf("config: notifications.retry.max_attempts must be positive, got %d", r.MaxAttempts)
	}
	if r.BaseDelayMs < 1 {
		return fmt.Errorf("config: notifications.retry.base_delay_ms must be positive, got %d", r.BaseDelayMs)
	}
	if r.MaxDelayMs < 1 {
		return fmt.Errorf("config: notifications.retry.max_delay_ms must be positive, got %d", r.MaxDelayMs)
	}
	if r.MaxDelayMs < r.BaseDelayMs {
		return fmt.Errorf("config: notifications.retry.max_delay_ms (%d) must not be smaller than notifications.retry.base_delay_ms (%d)",
			r.MaxDelayMs, r.BaseDelayMs)
	}
	if r.RequestTimeoutSeconds < 1 {
		return fmt.Errorf("config: notifications.retry.request_timeout_seconds must be positive, got %d", r.RequestTimeoutSeconds)
	}
	return nil
}

// NotifyChannel returns the resolved notification channel. An omitted block
// resolves to none.
func (c Config) NotifyChannel() string {
	ch := strings.TrimSpace(c.Notifications.Channel)
	if ch == "" {
		return NotifyChannelNone
	}
	return ch
}

// NotifyRetryPolicy returns the resolved retry policy as durations and a bound.
func (c Config) NotifyRetryPolicy() (maxAttempts int, baseDelay, maxDelay, timeout time.Duration) {
	r := c.Notifications.Retry
	return r.MaxAttempts,
		time.Duration(r.BaseDelayMs) * time.Millisecond,
		time.Duration(r.MaxDelayMs) * time.Millisecond,
		time.Duration(r.RequestTimeoutSeconds) * time.Second
}

// NotifyStatus is the redacted, renderable view of the notification
// configuration. It carries only presence booleans and resolved non-secret
// values: no credential can be rendered from it.
type NotifyStatus struct {
	Channel      string
	Available    bool
	FilePresent  bool
	FileResolved string
	HookPresent  bool
	HookResolved string
}

// NotifyStatus resolves the redacted notification status. A configured channel
// with no credentials available reports Available=false (unavailable) rather
// than failing the whole configuration load. A credential is never rendered:
// only the presence booleans are exposed.
func (c Config) NotifyStatus(credentialsPresent bool) NotifyStatus {
	st := NotifyStatus{Channel: c.NotifyChannel()}
	switch st.Channel {
	case NotifyChannelFile:
		p := strings.TrimSpace(c.Notifications.File.Path)
		st.FilePresent = p != ""
		st.FileResolved = p
		st.Available = st.FilePresent
	case NotifyChannelWebhook:
		u := strings.TrimSpace(c.Notifications.Webhook.URL)
		st.HookPresent = u != ""
		st.HookResolved = redactURLUserinfo(u)
		st.Available = st.HookPresent && credentialsPresent
	}
	return st
}

// redactURLUserinfo drops any user:password part of a URL so a status view can
// never render an embedded credential, even for a configuration that predates
// the validation rule.
func redactURLUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// SlackURLEnv returns the name of the environment variable holding the Slack
// webhook URL.
func (c Config) SlackURLEnv() string {
	if name := strings.TrimSpace(c.Notifications.Slack.URLEnv); name != "" {
		return name
	}
	return DefaultSlackURLEnv
}

// validEnvName reports whether s is a plausible environment variable name. It is
// strict on purpose: a pasted URL or token must be rejected, not accepted as a name.
func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
