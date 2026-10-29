package cli

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/store"
)

// ReasonUnreadableDeliveryState is the reason recorded when SOP cannot read the
// observed delivery state. Availability is then unknown: an unreadable outbox
// proves neither a confirmed delivery nor the absence of one, so reporting
// configured_unverified (or any other derived value) would be a false claim.
const ReasonUnreadableDeliveryState = "delivery_state_unreadable"

// unknownNotificationStatusDoc returns the shared notification status document
// for the case where delivery state cannot be read. It carries the same keys as
// notificationStatusDoc, sets availability to unknown, never sets active, and
// keeps the credential redaction boundary so no configured secret is emitted.
func unknownNotificationStatusDoc(cfg config.Config) map[string]any {
	channel := cfg.NotifyChannel()
	doc := map[string]any{
		"channel":            channel,
		"availability":       NotifyUnknown,
		"active":             false,
		"deliveries":         map[string]int{},
		"delivered":          0,
		"pending":            0,
		"exhausted":          0,
		"reason":             "Notification delivery state could not be read, so availability is unknown. Fix the reported error and run `sop notify status`.",
		"last_error":         ReasonUnreadableDeliveryState,
		"webhook_configured": channel == config.NotifyChannelWebhook && strings.TrimSpace(cfg.Notifications.Webhook.URL) != "",
		"slack_configured":   slackConfigured(cfg),
		"file_configured":    channel == config.NotifyChannelFile && strings.TrimSpace(cfg.Notifications.File.Path) != "",
	}
	return redactNotificationStatusDoc(doc, cfg)
}

// readOrUnknownNotificationStatusDoc reads the observed delivery state and falls
// back to the unknown document when the read fails, so every status surface can
// distinguish unknown from the three derived values instead of omitting the
// notifications object. It performs SELECT-only reads.
func readOrUnknownNotificationStatusDoc(st *store.Store, cfg config.Config, readErr error) map[string]any {
	doc := unknownNotificationStatusDoc(cfg)
	doc["error"] = readErr.Error()
	return doc
}
