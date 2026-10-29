package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/notify"
	"github.com/imhttran/agentic-sop/internal/store"
)

const notifyUsage = "usage: sop notify status [--json] | sop notify drain | sop notify test"

// Notification availability values. Configuration alone never reports "active":
// only an observed, confirmed delivery does.
const (
	NotifyUnavailable          = "unavailable"
	NotifyConfiguredUnverified = "configured_unverified"
	NotifyActive               = "active"
	NotifyUnknown              = "unknown"
)

// runNotify is the operator surface for the notification outbox: read status, run
// one bounded drain pass, or enqueue and attempt one test event so a channel can be
// verified before work is handed off in the background.
func runNotify(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, notifyUsage)
		return exitUsage
	}
	switch args[0] {
	case "status":
		return notifyStatus(args[1:], stdout, stderr, d)
	case "drain":
		if len(args) != 1 {
			fmt.Fprintln(stderr, notifyUsage)
			return exitUsage
		}
		return notifyDrain(stdout, stderr, d)
	case "test":
		if len(args) != 1 {
			fmt.Fprintln(stderr, notifyUsage)
			return exitUsage
		}
		return notifyTest(stdout, stderr, d)
	default:
		fmt.Fprintln(stderr, notifyUsage)
		return exitUsage
	}
}

// notifyStatus reports the configured channel, the availability derived from
// observed evidence, and the per-status delivery counts.
func notifyStatus(args []string, stdout, stderr io.Writer, d deps) int {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		fmt.Fprintln(stderr, "usage: sop notify status [--json]")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	defer st.Close()

	doc, err := readNotificationStatusDoc(st, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return exitError
		}
		return exitOK
	}
	channel, _ := doc["channel"].(string)
	availability, _ := doc["availability"].(string)
	fmt.Fprintf(stdout, "Channel: %s\n", channel)
	fmt.Fprintf(stdout, "Availability: %s\n", availability)
	fmt.Fprintf(stdout, "Deliveries: delivered=%d pending=%d exhausted=%d\n",
		intField(doc, "delivered"), intField(doc, "pending"), intField(doc, "exhausted"))
	if lastError, _ := doc["last_error"].(string); lastError != "" {
		fmt.Fprintf(stdout, "Last error: %s\n", lastError)
	}
	if reason, _ := doc["reason"].(string); reason != "" {
		fmt.Fprintln(stdout, reason)
	}
	return exitOK
}

// notificationStatusReason returns the actionable reason for an availability that
// is not active, or an empty string for active.
func notificationStatusReason(availability string) string {
	switch availability {
	case NotifyUnavailable:
		return "Notifications are unavailable: no channel is configured. Configure notifications.channel before relying on delivery."
	case NotifyConfiguredUnverified:
		return "No delivery has been confirmed yet: configuration alone does not prove the channel works. Run `sop notify test`."
	default:
		return ""
	}
}

// notificationStatusDoc builds the shared, read-only notification status
// document from configuration and observed delivery state. It is the single
// projection every status surface (CLI and MCP) renders, so availability, counts,
// the last error, the actionable reason, and credential presence agree
// everywhere.
//
// It reports credential presence only: the configured webhook URL and file path
// are never placed in the document. See redactNotificationStatusDoc.
func notificationStatusDoc(cfg config.Config, byStatus map[string]int, lastError string) map[string]any {
	channel := cfg.NotifyChannel()
	delivered := byStatus[notify.StatusDelivered]
	availability := notificationAvailability(channel, delivered)
	if channel == config.NotifyChannelSlack && !slackConfigured(cfg) {
		// The channel cannot send without its webhook URL, whatever was delivered
		// before, so it is not available now.
		availability = NotifyUnavailable
	}
	deliveries := map[string]int{}
	for status, n := range byStatus {
		deliveries[status] = n
	}
	doc := map[string]any{
		"channel":            channel,
		"availability":       availability,
		"active":             availability == NotifyActive,
		"deliveries":         deliveries,
		"delivered":          delivered,
		"pending":            byStatus[notify.StatusPending] + byStatus[notify.StatusClaimed],
		"exhausted":          byStatus[notify.StatusExhausted],
		"reason":             notificationStatusReason(availability),
		"webhook_configured": channel == config.NotifyChannelWebhook && strings.TrimSpace(cfg.Notifications.Webhook.URL) != "",
		"slack_configured":   slackConfigured(cfg),
		"file_configured":    channel == config.NotifyChannelFile && strings.TrimSpace(cfg.Notifications.File.Path) != "",
	}
	if lastError != "" {
		doc["last_error"] = lastError
	}
	return redactNotificationStatusDoc(doc, cfg)
}

// redactedNotificationKeys are the document keys that would carry a configured
// credential value if a caller ever added them. Redaction removes them so no
// serialization path can emit a secret.
var redactedNotificationKeys = []string{"webhook_url", "file_path", "url", "password", "token", "secret"}

// redactNotificationStatusDoc is the serialization-boundary guard: it removes any
// configured credential value from the document and replaces it with presence
// only. Every status surface passes through it before encoding.
func redactNotificationStatusDoc(doc map[string]any, cfg config.Config) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		out[k] = v
	}
	for _, k := range redactedNotificationKeys {
		delete(out, k)
	}
	// Remove any residual value that equals a configured credential, even under
	// an unexpected key, so the boundary holds for callers that add fields.
	secrets := notificationCredentialValues(cfg)
	for k, v := range out {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		for _, secret := range secrets {
			if s == secret {
				delete(out, k)
				break
			}
		}
	}
	return out
}

// notificationCredentialValues lists the configured credential values that must
// never be serialized.
func notificationCredentialValues(cfg config.Config) []string {
	var out []string
	if u := strings.TrimSpace(cfg.Notifications.Webhook.URL); u != "" {
		out = append(out, u)
	}
	if p := strings.TrimSpace(cfg.Notifications.File.Path); p != "" {
		out = append(out, p)
	}
	return out
}

// readNotificationStatusDoc reads the observed delivery state and returns the
// shared notification status document. It performs SELECT-only reads and never
// enqueues an event or mutates delivery state.
func readNotificationStatusDoc(st *store.Store, cfg config.Config) (map[string]any, error) {
	counts, err := st.CountNotificationDeliveriesByStatus()
	if err != nil {
		return nil, err
	}
	byStatus := map[string]int{}
	for _, c := range counts {
		byStatus[c.Status] = c.Count
	}
	lastError, err := st.LastNotificationDeliveryError()
	if err != nil {
		return nil, err
	}
	return notificationStatusDoc(cfg, byStatus, lastError), nil
}

// notificationStatusText renders the shared document as a stable text block for
// text-only status surfaces such as the sop_status MCP tool.
func notificationStatusText(doc map[string]any) []string {
	availability, _ := doc["availability"].(string)
	channel, _ := doc["channel"].(string)
	lines := []string{
		"notifications: " + availability,
		fmt.Sprintf("notifications_channel: %s", channel),
		fmt.Sprintf("notifications_deliveries: delivered=%d pending=%d exhausted=%d",
			intField(doc, "delivered"), intField(doc, "pending"), intField(doc, "exhausted")),
		fmt.Sprintf("notifications_credentials: webhook_configured=%t file_configured=%t",
			boolField(doc, "webhook_configured"), boolField(doc, "file_configured")),
	}
	if lastError, _ := doc["last_error"].(string); lastError != "" {
		lines = append(lines, "notifications_last_error: "+lastError)
	}
	if reason, _ := doc["reason"].(string); reason != "" {
		lines = append(lines, "notifications_reason: "+reason)
	}
	return lines
}

// intField reads an int-valued document field, defaulting to zero. It accepts
// every numeric shape a status document can carry: the live projection stores an
// int, while a document that was JSON-encoded and decoded again (for example a
// test reading --json output) stores a float64.
func intField(doc map[string]any, key string) int {
	switch n := doc[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if v, err := n.Int64(); err == nil {
			return int(v)
		}
	}
	return 0
}

// boolField reads a bool-valued document field, defaulting to false.
func boolField(doc map[string]any, key string) bool {
	if b, ok := doc[key].(bool); ok {
		return b
	}
	return false
}

// slackConfigured reports whether the Slack channel is selected and its webhook
// URL is present in the environment. It reports presence only, never the value.
func slackConfigured(cfg config.Config) bool {
	return cfg.NotifyChannel() == config.NotifyChannelSlack && strings.TrimSpace(os.Getenv(cfg.SlackURLEnv())) != ""
}

// notificationAvailability derives availability from observed evidence, never from
// configuration alone.
func notificationAvailability(channel string, delivered int) string {
	if strings.TrimSpace(channel) == "" || channel == config.NotifyChannelNone {
		return NotifyUnavailable
	}
	if delivered > 0 {
		return NotifyActive
	}
	return NotifyConfiguredUnverified
}

// notifyDrain runs exactly one bounded drain pass.
func notifyDrain(stdout, stderr io.Writer, d deps) int {
	dir, st, cfg, ok := openNotifyState(d, stderr)
	if !ok {
		return exitError
	}
	defer st.Close()

	ch, err := notifyChannel(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	report, err := notifyDrainer(st, ch, cfg).Drain(0)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	_ = dir
	fmt.Fprintf(stdout, "Drain: claimed=%d delivered=%d retried=%d exhausted=%d\n",
		report.Claimed, report.Delivered, report.Retried, report.Exhausted)
	return exitOK
}

// notifyTest enqueues and attempts one test event through the configured channel.
func notifyTest(stdout, stderr io.Writer, d deps) int {
	dir, st, cfg, ok := openNotifyState(d, stderr)
	if !ok {
		return exitError
	}
	defer st.Close()

	ch, err := notifyChannel(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	now := time.Now().UTC()
	e, err := notify.NewTestEvent(cfg.Project.Name)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	if err := st.EnqueueNotification(storeNotificationEvent(e), []string{cfg.NotifyChannel()}, cfg.Notifications.Retry.MaxAttempts, now, now); err != nil {
		fmt.Fprintf(stderr, "notify: enqueue test event: %v\n", err)
		return exitError
	}
	report, err := notifyDrainer(st, ch, cfg).Drain(0)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitError
	}
	_ = dir
	if report.Delivered == 0 {
		fmt.Fprintf(stderr, "notify: test delivery not confirmed (claimed=%d delivered=%d retried=%d exhausted=%d)\n",
			report.Claimed, report.Delivered, report.Retried, report.Exhausted)
		return exitError
	}
	fmt.Fprintf(stdout, "Test event %s delivered (confirmed by channel %s).\n", e.ID, cfg.NotifyChannel())
	return exitOK
}

// openNotifyState opens the project store and configuration for a notify command.
func openNotifyState(d deps, stderr io.Writer) (string, *store.Store, config.Config, bool) {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return "", nil, config.Config{}, false
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return "", nil, config.Config{}, false
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", nil, config.Config{}, false
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", nil, config.Config{}, false
	}
	return dir, st, cfg, true
}

// notifyChannel builds the configured channel, or reports unavailable.
func notifyChannel(cfg config.Config) (notify.Channel, error) {
	switch cfg.NotifyChannel() {
	case config.NotifyChannelFile:
		if strings.TrimSpace(cfg.Notifications.File.Path) == "" {
			return nil, fmt.Errorf("channel %q is configured without notifications.file.path", config.NotifyChannelFile)
		}
		return notify.NewFileChannel(cfg.Notifications.File.Path), nil
	case config.NotifyChannelWebhook:
		if strings.TrimSpace(cfg.Notifications.Webhook.URL) == "" {
			return nil, fmt.Errorf("channel %q is configured without notifications.webhook.url", config.NotifyChannelWebhook)
		}
		return notify.NewWebhookChannel(cfg.Notifications.Webhook.URL, nil), nil
	case config.NotifyChannelSlack:
		name := cfg.SlackURLEnv()
		u := strings.TrimSpace(os.Getenv(name))
		if u == "" {
			return nil, fmt.Errorf("channel %q is configured but the environment variable %s is not set", config.NotifyChannelSlack, name)
		}
		return notify.NewSlackChannel(u, nil), nil
	default:
		return nil, notify.ErrChannelUnavailable
	}
}

// notifyDrainer builds the bounded drainer over the durable outbox.
func notifyDrainer(st *store.Store, ch notify.Channel, cfg config.Config) *notify.Drainer {
	return &notify.Drainer{
		Outbox:      notifyOutbox{st: st},
		Channel:     ch,
		Backoff:     notify.Backoff{BaseDelay: msDuration(cfg.Notifications.Retry.BaseDelayMs), MaxDelay: msDuration(cfg.Notifications.Retry.MaxDelayMs)},
		MaxAttempts: cfg.Notifications.Retry.MaxAttempts,
	}
}

func msDuration(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// storeNotificationEvent projects a notify.Event into the durable row shape.
func storeNotificationEvent(e notify.Event) store.NotificationEvent {
	return store.NotificationEvent{
		ID: e.ID, Version: e.Version, Kind: string(e.Kind), Project: e.Project,
		JobID: e.JobID, TaskID: e.TaskID, Attempt: e.Attempt, Transition: e.Transition,
		Reason: e.Reason, NextAction: e.NextAction, Outcome: e.Outcome,
		ApprovalRequestID: e.ApprovalRequestID, ResolutionCommand: e.ResolutionCommand,
		OccurredAt: e.OccurredAt,
	}
}

// notifyOutbox adapts the SQLite store to the worker's claim/record contract.
type notifyOutbox struct{ st *store.Store }

func (o notifyOutbox) ClaimDue(limit int, now time.Time) ([]notify.DeliveryRecord, error) {
	claimed, err := o.st.ClaimNotificationDeliveries(limit, now)
	if err != nil {
		return nil, err
	}
	out := make([]notify.DeliveryRecord, 0, len(claimed))
	for _, c := range claimed {
		out = append(out, notify.DeliveryRecord{
			EventID: c.EventID, Channel: c.Channel, Status: c.Status,
			Attempt: c.Attempt, MaxAttempts: c.MaxAttempts, LastError: c.LastError,
			NextAttemptAt: c.NextAttemptAt, ClaimToken: c.ClaimToken,
		})
	}
	return out, nil
}

func (o notifyOutbox) RecordOutcome(token, result, lastError string, nextAttemptAt, now time.Time) error {
	return o.st.RecordDeliveryOutcome(token, result, lastError, nextAttemptAt, now)
}

// drainNotificationsBestEffort attempts one bounded drain pass after a command that
// may have produced events. It resolves the project itself, ignores every error, and
// never affects the caller's exit status: delivery is isolated from execution.
func drainNotificationsBestEffort() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	path := statePath(dir)
	if _, err := os.Stat(path); err != nil {
		return
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return
	}
	ch, err := notifyChannel(cfg)
	if err != nil {
		return
	}
	st, err := store.Open(path)
	if err != nil {
		return
	}
	defer st.Close()
	_, _ = notifyDrainer(st, ch, cfg).Drain(0)
}
