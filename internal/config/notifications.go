package config

import (
	"errors"
	"fmt"
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
)

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
	// Retry bounds delivery retries.
	Retry NotifyRetry `yaml:"retry"`
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
		st.HookResolved = u
		st.Available = st.HookPresent && credentialsPresent
	}
	return st
}
