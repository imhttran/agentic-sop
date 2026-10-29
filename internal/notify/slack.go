package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Payload styles select the body shape the webhook channel posts. The native
// style posts the SOP event JSON verbatim, which is the default and preserves
// existing behavior. The slack style posts a Slack incoming-webhook message.
const (
	// PayloadNative posts the SOP event JSON as the request body.
	PayloadNative = "native"
	// PayloadSlack posts a Slack incoming-webhook message body.
	PayloadSlack = "slack"
)

// payloadStyles is the accepted payload value set, kept here so a bad value
// fails with a message that names the accepted values.
var payloadStyles = map[string]bool{
	PayloadNative: true,
	PayloadSlack:  true,
}

// NormalizePayloadStyle returns the canonical payload style for style. An empty
// value selects the native style, so an omitted setting preserves existing
// behavior.
func NormalizePayloadStyle(style string) string {
	s := strings.TrimSpace(strings.ToLower(style))
	if s == "" {
		return PayloadNative
	}
	return s
}

// ValidPayloadStyle reports whether style selects a supported payload style. An
// empty value is valid and selects the native style.
func ValidPayloadStyle(style string) bool {
	s := strings.TrimSpace(strings.ToLower(style))
	if s == "" {
		return true
	}
	return payloadStyles[s]
}

// PayloadStyleStrings returns the accepted payload spellings in a stable order.
func PayloadStyleStrings() []string {
	return []string{PayloadNative, PayloadSlack}
}

// slackMessage is the Slack incoming-webhook message body. Slack requires a
// top-level text field, so a receiver renders at least the summary line.
type slackMessage struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

// slackBlock is one Slack Block Kit section block. The encoder emits only
// top-level text in mrkdwn form so the body stays valid without a block toolkit.
type slackBlock struct {
	Type string     `json:"type"`
	Text *slackText `json:"text,omitempty"`
}

// slackText is the Slack plain/mrkdwn text object.
type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// EncodeSlackPayload converts e into a Slack incoming-webhook message body.
//
// The body is valid JSON with a non-empty top-level text field that always
// carries the event id. It carries no credential: the event never holds one, and
// the encoder renders only event fields.
func EncodeSlackPayload(e Event) ([]byte, error) {
	text := slackEventText(e)
	msg := slackMessage{
		Text: text,
		Blocks: []slackBlock{{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: text},
		}},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("notify: encode slack payload for event %q: %w", e.ID, err)
	}
	return body, nil
}

// slackEventText renders the single mrkdwn summary line the Slack body carries.
func slackEventText(e Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SOP %s in %s: event %s", string(e.Kind), e.Project, e.ID)
	if e.TaskID != "" {
		fmt.Fprintf(&b, " task=%s", e.TaskID)
	}
	if e.JobID != "" {
		fmt.Fprintf(&b, " job=%s", e.JobID)
	}
	fmt.Fprintf(&b, " (attempt %d, %s)", e.Attempt, e.Transition)
	if e.Outcome != "" {
		fmt.Fprintf(&b, "\noutcome: %s", e.Outcome)
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, "\nreason: %s", e.Reason)
	}
	if e.NextAction != "" {
		fmt.Fprintf(&b, "\nnext: %s", e.NextAction)
	}
	if e.ResolutionCommand != "" {
		fmt.Fprintf(&b, "\nresolve: %s", e.ResolutionCommand)
	}
	return b.String()
}
