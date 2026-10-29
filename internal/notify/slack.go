package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxSlackText bounds one message so a long reason cannot be refused by Slack.
const maxSlackText = 3000

// SlackCapabilities is the delivery guarantee of the Slack channel. A Slack
// incoming webhook does not honour an idempotency key, so a retry after an
// ambiguous attempt can post a second message.
func SlackCapabilities() Capabilities {
	return Capabilities{
		KeyHonored: false,
		Guarantee: "at-least-once attempt; Slack incoming webhooks do not honour an idempotency key, " +
			"so an event retried after an ambiguous attempt may be posted more than once",
	}
}

// SlackChannel delivers events to a Slack incoming webhook as a text message.
//
// The webhook URL carries a secret in its path, so it is never placed in an error:
// a transport failure is reduced to its cause before it is returned, because the
// standard library's URL error repeats the full URL.
type SlackChannel struct {
	// URL is the Slack incoming-webhook URL. It is a secret.
	URL string
	// Transport performs the request. A nil Transport uses http.DefaultClient.
	Transport Transport
	// Timeout bounds one attempt. A zero value means 10 seconds.
	Timeout time.Duration
}

// NewSlackChannel returns a Slack channel for url that uses transport. A nil
// transport uses http.DefaultClient.
func NewSlackChannel(url string, transport Transport) *SlackChannel {
	return &SlackChannel{URL: url, Transport: transport}
}

// Capabilities reports that the Slack channel does not honour the idempotency key.
func (c *SlackChannel) Capabilities() Capabilities { return SlackCapabilities() }

// Send posts e as a Slack message and classifies the outcome: 2xx is confirmed, a
// 4xx is a definitive refusal, and a timeout, transport error, or 5xx is ambiguous.
func (c *SlackChannel) Send(d Delivery) (Ack, error) {
	if err := d.Validate(); err != nil {
		return AckRejected, err
	}
	e := d.Event
	if c == nil || strings.TrimSpace(c.URL) == "" {
		return AckRejected, errors.New("notify: slack channel requires a webhook URL")
	}
	payload, err := json.Marshal(map[string]string{"text": SlackText(e)})
	if err != nil {
		return AckRejected, fmt.Errorf("notify: encode event %q: %w", e.ID, err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		return AckRejected, fmt.Errorf("notify: build slack request for event %q: %w", e.ID, withoutURL(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(IdempotencyHeader, d.Key())
	transport := c.Transport
	if transport == nil {
		transport = http.DefaultClient
	}
	resp, err := transport.Do(req)
	if err != nil {
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q to slack: %w", e.ID, withoutURL(err))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return AckConfirmed, nil
	case resp.StatusCode >= 500:
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q to slack: server status %d", e.ID, resp.StatusCode)
	case resp.StatusCode >= 400:
		return AckRejected, fmt.Errorf("notify: deliver event %q to slack: rejected with status %d", e.ID, resp.StatusCode)
	default:
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q to slack: unexpected status %d", e.ID, resp.StatusCode)
	}
}

// withoutURL returns the cause of a request error without the URL the standard
// library attaches to it.
func withoutURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// slackKindLabel is the heading each lifecycle kind gets in a message.
func slackKindLabel(k Kind) string {
	switch k {
	case KindCompleted:
		return "completed"
	case KindFailed:
		return "failed"
	case KindWaitingForHuman:
		return "needs a human decision"
	default:
		return string(k)
	}
}

// SlackText renders an event as the message text. Slack control characters in
// the event's text are escaped so a reason cannot form a link or a mention.
func SlackText(e Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*SOP %s* — project %s", slackKindLabel(e.Kind), slackEscape(e.Project))
	if detail := firstNonEmpty(e.Reason, e.Outcome); detail != "" {
		fmt.Fprintf(&b, "\n%s", slackEscape(detail))
	}
	if e.ResolutionCommand != "" {
		fmt.Fprintf(&b, "\nResolve: `%s`", strings.ReplaceAll(slackEscape(e.ResolutionCommand), "`", "'"))
	}
	if e.NextAction != "" {
		fmt.Fprintf(&b, "\nNext: %s", slackEscape(e.NextAction))
	}
	var ids []string
	if e.TaskID != "" {
		ids = append(ids, "task "+slackEscape(e.TaskID))
	}
	if e.JobID != "" {
		ids = append(ids, "job "+slackEscape(e.JobID))
	}
	short := e.ID
	if len(short) > 12 {
		short = short[:12]
	}
	ids = append(ids, "event "+short)
	fmt.Fprintf(&b, "\n%s", strings.Join(ids, " · "))

	text := b.String()
	if len(text) > maxSlackText {
		text = text[:maxSlackText-1] + "…"
	}
	return text
}

// slackEscape escapes the three characters Slack treats as control characters.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(strings.TrimSpace(s))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
