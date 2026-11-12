# LLM05 Slack rendering and channel binding: operator observation (OWASP gap 12)

**Status:** **OBSERVED 2026-11-12 (operator); a plain `@here` notifies.** One synthetic Slack delivery was
accepted on 2026-11-10 ([llm10-live-delivery.md](llm10-live-delivery.md)), but no one
has looked at how Slack rendered it, which channel it reached, or whether anything
unfurled or pinged. This page is the observation the operator runs. Nothing in it
has been sent by SOP.

**What this does not do:** it does not send a message from this repository, and it
does not change any configuration. The operator runs each step below in a disposable
project.

## What the encoder should do

`notify.EncodeSlackPayload` (`internal/notify/slack.go`) escapes `&`, `<` and `>` in
the event text before it is sent, and puts the text in a `mrkdwn` section. So the
expected Slack display is:

- `<!channel>`, `<!here>`, `<@U…>` and `<https://…|label>` appear as **literal text**
  (escaped to `&lt;…&gt;`), not as a mention or a link.
- A bare `@here` in plain text does not notify in a webhook message.
- `*bold*` and `` `code` `` are Slack formatting, not an escape failure. Record
  them as formatting, not as a fault.

## Steps for the operator

1. In a **disposable** project directory, set the notification channel for the probe
   only:

   ```yaml
   notifications:
     channel: webhook
     webhook:
       payload: slack
   ```

   The webhook URL goes in the gitignored `.env` of that disposable project. Do not
   print it, paste it, or commit it.

2. Add this test file to that disposable project's `internal/cli` package, or to a
   disposable checkout of this repository. It is **not** a file to commit to this
   repository.

   ```go
   package cli

   import (
   	"os"
   	"testing"
   )

   // Enqueues one synthetic event with escaped markup, a link, and mention-shaped
   // text into the project's durable outbox. It sends nothing itself.
   func TestSlackProbeEnqueue(t *testing.T) {
   	dir := os.Getenv("SOP_PROBE_PROJECT")
   	if dir == "" {
   		t.Skip("set SOP_PROBE_PROJECT to the disposable project directory")
   	}
   	_, _, err := emitTransitionFailed(dir, "probe-task", "", 1, false,
   		"Synthetic probe: <b>escaped markup</b> & `code` *bold* <https://example.com|link> <!channel> @here <@U000PROBE>",
   		"No action. This is a synthetic probe for rendering only.")
   	if err != nil {
   		t.Fatal(err)
   	}
   }
   ```

3. Enqueue the event, then deliver it with the existing drain path:

   ```bash
   SOP_PROBE_PROJECT=/path/to/disposable/project go test ./internal/cli -run TestSlackProbeEnqueue -count=1
   sop notify drain
   sop notify status --json
   ```

   Record the `status --json` output that shows `DELIVERED`, and nothing else from it.

4. Look in the bound Slack channel. Fill in the table below. Do not include
   screenshots that show private workspace content, names, or other messages. Describe
   what you see in words.

## Observation table (operator fills in)

| Check | Expected | Observed (operator) |
| --- | --- | --- |
| Message arrived in the bound channel | Yes | |
| Bound channel name, as the operator confirms it | The channel the webhook was created for | |
| `<b>escaped markup</b>` shows as literal text | Literal, with the angle brackets visible | |
| `<https://example.com|link>` shows as literal text, not a link | Literal | |
| `<!channel>` and `<!here>` shown as literal text, no ping | Literal, no notification | |
| `<@U000PROBE>` shown as literal text, no mention | Literal, no mention | |
| `@here` in plain text pinged anyone | No | |
| Auto-unfurl of `https://example.com` produced a preview | No preview | |
| `*bold*` and `` `code` `` | Formatting applied (not a fault) | |
| Any other observation | | |

## After the operator reports

A later session records the table as OBSERVED with the date and the workspace-
independent details only. Then the coverage statement's LLM05 wording changes from
"not observed" to what was observed. Until then, gap 12 stays Waiting on operator.

## Observed result (2026-11-12, operator)

One probe event was enqueued in a disposable project and delivered once through the
webhook with the Slack payload. The operator checked the bound private channel.

| Check | Expected (before observation) | Observed |
| --- | --- | --- |
| Message arrived in the bound channel | Yes | Yes |
| `<b>escaped markup</b>` shows as literal text | Literal | Literal |
| `<https://example.com|link>` shows as literal text, not a link | Literal | Literal, not clickable |
| `<!channel>` shows as literal text, no ping | Literal, no ping | Literal; not separately checked for a ping |
| `<@U000PROBE>` shows as literal text, no mention | Literal, no mention | Literal |
| A plain `@here` in the text did not notify | No ping | **Notified:** the operator received a notification |
| Link preview for `https://example.com` | None | None |
| `*bold*` and `` `code` `` | Formatting (not a fault) | Formatted |

**Finding.** Slack treats a plain `@here` in webhook message text as a broadcast
mention and notifies the channel. The encoder escapes `&`, `<` and `>` only, so `@here`
and `@channel` pass through unchanged. The expectation above that a bare `@here` would
not notify was wrong.

**Decision (operator, chat, 2026-11-12): document, do not change the encoder.**
The residual is recorded here and in the coverage statement.

**Residual.** Text that reaches a Slack payload can contain `@here` or `@channel`, and
each one pings everyone in the bound channel. Reason, next-action and outcome text can
include model and tool output, so a failure message that quotes such text pings the
channel. Impact is channel noise and urgency pressure. It does not expose secrets or
grant access: redaction still applies, and the webhook posts only to its bound channel.
Likelihood is low in normal use and higher if untrusted content reaches the model or a
tool. Operators who mute the channel should treat a ping from SOP as unverified.

**Not in place.** Escaping `@` in the encoder, which would remove the ping. Adding it
later is a small change in `internal/notify/slack.go`.
