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

// IdempotencyHeader is the request header that carries the event id as the
// idempotency key on every webhook attempt.
const IdempotencyHeader = "Idempotency-Key"

// WebhookCapabilities is the delivery guarantee of the webhook channel.
//
// The channel sends the event id as the idempotency key on every attempt, but it
// cannot prove the remote receiver honours that key. The channel therefore does
// not claim duplicate-free or exactly-once delivery: the receiver may apply the
// same event more than once.
func WebhookCapabilities() Capabilities {
	return Capabilities{
		KeyHonored: false,
		Guarantee: "at-least-once attempt; the receiver may not honour the idempotency key, " +
			"so the same event may be applied more than once and this channel makes no " +
			"single-application promise",
	}
}

// Transport performs one outbound HTTP request. net/http.Client satisfies it, and
// a test injects a stub so no test needs network access.
type Transport interface {
	Do(req *http.Request) (*http.Response, error)
}

// WebhookChannel delivers events to an HTTP endpoint.
//
// Each Send transmits the event id in the Idempotency-Key header, so every retry
// of one event carries the same key. The adapter holds no per-call state: the
// same Event value always produces the same key.
type WebhookChannel struct {
	// URL is the endpoint each event is posted to.
	URL string
	// Transport performs the request. A nil Transport uses http.DefaultClient.
	Transport Transport
	// Timeout bounds one attempt. A zero value means 10 seconds.
	Timeout time.Duration
	// Payload selects the request body style: native (the SOP event JSON, the
	// default) or slack (a Slack incoming-webhook message). An empty value
	// selects native.
	Payload string
}

// NewWebhookChannel returns a webhook channel for url that uses transport. A nil
// transport uses http.DefaultClient.
func NewWebhookChannel(url string, transport Transport) *WebhookChannel {
	return &WebhookChannel{URL: url, Transport: transport}
}

// Capabilities reports that the webhook channel does not honour the idempotency
// key from the sender's point of view.
func (c *WebhookChannel) Capabilities() Capabilities { return WebhookCapabilities() }

// Send posts e to the channel endpoint and classifies the transport outcome.
//
// It returns AckConfirmed for a 2xx response, AckRejected for a definitive 4xx
// refusal (the receiver read the request and refused it), and AckAmbiguous for a
// timeout, a transport error, or a 5xx response, because the outcome is then
// unknown.
func (c *WebhookChannel) Send(d Delivery) (Ack, error) {
	if err := d.Validate(); err != nil {
		return AckRejected, err
	}
	e := d.Event
	if c == nil || strings.TrimSpace(c.URL) == "" {
		return AckRejected, errors.New("notify: webhook channel requires a URL")
	}

	var payload []byte
	var err error
	switch NormalizePayloadStyle(c.Payload) {
	case PayloadSlack:
		payload, err = EncodeSlackPayload(e)
	default:
		payload, err = json.Marshal(e)
	}
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
		// A construction error can wrap a *url.Error that repeats the endpoint.
		// last_error is persisted in the outbox and rendered by the CLI, so the
		// endpoint (which can carry a token) must not appear in the message.
		return AckRejected, fmt.Errorf("notify: build webhook request for event %q: %s", e.ID, sanitizeWebhookError(err, c.URL))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(IdempotencyHeader, d.Key())

	transport := c.Transport
	if transport == nil {
		transport = http.DefaultClient
	}

	resp, err := transport.Do(req)
	if err != nil {
		// A timeout or a transport error leaves the outcome unknown. The transport
		// error can be a *url.Error that repeats the full request URL, which may
		// carry a token; sanitize it before it reaches last_error or the CLI.
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q: %s", e.ID, sanitizeWebhookError(err, c.URL))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return AckConfirmed, nil
	case resp.StatusCode >= 500:
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q: server status %d", e.ID, resp.StatusCode)
	case resp.StatusCode >= 400:
		return AckRejected, fmt.Errorf("notify: deliver event %q: rejected with status %d", e.ID, resp.StatusCode)
	default:
		// Any other status (for example an unexpected redirect) is not a proof of
		// delivery and not a definitive refusal.
		return AckAmbiguous, fmt.Errorf("notify: deliver event %q: unexpected status %d", e.ID, resp.StatusCode)
	}
}

// sanitizeWebhookError returns a URL-free description of err for the webhook
// endpoint rawURL. It keeps the operation and the underlying reason (for example
// "context deadline exceeded") but never the request URL, its host, its userinfo,
// its path, or its query.
//
// The endpoint can embed a token and the text is stored as the outbox last_error
// and printed by the CLI, so the URL must not leak at the source. Any occurrence
// of the raw endpoint and of its token-bearing components is replaced.
func sanitizeWebhookError(err error, rawURL string) string {
	if err == nil {
		return "unknown error"
	}
	msg := err.Error()
	if u, perr := url.Parse(rawURL); perr == nil {
		if u.Scheme != "" && u.Host != "" {
			msg = strings.ReplaceAll(msg, u.Scheme+"://"+u.Host, "<redacted>")
		}
		if u.Host != "" {
			msg = strings.ReplaceAll(msg, u.Host, "<redacted>")
		}
		if u.User != nil {
			if user := u.User.String(); user != "" {
				msg = strings.ReplaceAll(msg, user, "<redacted>")
			}
			if pass, ok := u.User.Password(); ok && pass != "" {
				msg = strings.ReplaceAll(msg, pass, "<redacted>")
			}
		}
		if u.Path != "" {
			msg = strings.ReplaceAll(msg, u.Path, "<redacted>")
		}
		if u.RawPath != "" && u.RawPath != u.Path {
			msg = strings.ReplaceAll(msg, u.RawPath, "<redacted>")
		}
		if u.RawQuery != "" {
			msg = strings.ReplaceAll(msg, u.RawQuery, "<redacted>")
			msg = strings.ReplaceAll(msg, "?"+u.RawQuery, "?<redacted>")
			for _, kv := range strings.Split(u.RawQuery, "&") {
				if _, v, ok := strings.Cut(kv, "="); ok && v != "" {
					msg = strings.ReplaceAll(msg, v, "<redacted>")
				}
			}
		}
		if rawURL != "" {
			msg = strings.ReplaceAll(msg, rawURL, "<redacted>")
		}
	}
	// net/url reports a construction failure as `parse "<url>": <reason>`; keep
	// the reason and drop the quoted URL entirely.
	if ue := (*url.Error)(nil); errors.As(err, &ue) {
		return fmt.Sprintf("%s %s: %s", ue.Op, "request failed", scrubURLLikeTokens(ue.Err.Error()))
	}
	return scrubURLLikeTokens(msg)
}

// scrubURLLikeTokens removes any remaining URL-shaped token from msg so a
// transport error that rebuilds the endpoint textually cannot re-introduce it.
func scrubURLLikeTokens(msg string) string {
	fields := strings.Fields(msg)
	for i, f := range fields {
		if strings.Contains(f, "://") {
			fields[i] = "<redacted-url>"
		}
	}
	return strings.Join(fields, " ")
}
