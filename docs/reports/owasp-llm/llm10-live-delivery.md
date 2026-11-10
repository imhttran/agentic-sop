# Plan 10 — live Slack delivery probe

Status: **DELIVERY EXECUTED ONCE (transport confirmed); RENDERING NOT OBSERVED.**
One synthetic notification was accepted by the configured Slack webhook on
2026-11-10T02:33:17Z. The durable outbox recorded `DELIVERED` on attempt 1 with no
retry and no exhausted delivery. No authenticated Slack session was available on
the test host, so the rendered message, the bound channel name, and the absence of
pings were not observed. A 2xx response is not a rendering claim.

Related: [llm10-output-handling.md](llm10-output-handling.md) (Plan 10),
[llm10-acceptance-request.md](llm10-acceptance-request.md),
[NOTIFICATIONS.md](../../guides/NOTIFICATIONS.md), and the cumulative
[evidence matrix](README.md).

## Tested revision and host

| Item                | Value                                                                                                                                                                                           |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository revision | `0200ad3f77604d828acc90c3b1f9dab990e63042` (`origin/main`)                                                                                                                                      |
| Host                | Darwin arm64, Go 1.27.1                                                                                                                                                                         |
| Binary              | built from the tested revision with `go build ./cmd/sop`                                                                                                                                        |
| Sink code           | `internal/notify/slack.go`, `internal/notify/webhook.go`, `internal/cli/terminal.go`, `internal/cli/cli.go`, and `internal/cli/activity.go` are unchanged since the wrap-up revision `ed9ec2dc` |
| Earlier records     | The original Linux `x86_64` verification and the `ed9ec2dc` Darwin re-verification remain historical; this probe is a separate live observation at `0200ad3f`                                   |

The `internal/notify` package did change after `ed9ec2dc` for the unrelated LLM08
work (the event `Nonce` identity discriminator and redaction tests). The Slack
encoder and webhook transport did not change. The live probe exercised the
current encoder at `0200ad3f`, not the `ed9ec2dc` binary.

## Authorization and configuration

The operator commissioning this task explicitly authorized one synthetic test
notification through SOP's existing Slack notification path to `#sop-alert`, using
the existing configured credentials and inert content. That task instruction is
the authorization record; no separate signed approval reference exists in the
repository.

The probe ran in a disposable Git worktree, not in the owning checkout:

- `notifications.channel: webhook` and `notifications.webhook.payload: slack` were
  enabled in the worktree's `.agent-sdlc/config.yaml` for the test only;
- the gitignored `.env` was copied from the main checkout, used without printing,
  and never staged or committed;
- the temporary channel settings were reverted before the documentation commit.

The worktree's `.agent-sdlc/state.db` is disposable probe state. It is not an
owning SOP plan state, and it records no Plan 10 task, run, outcome, or approval.

The webhook value was not printed, copied into this record, or passed as a command
argument. Only the presence and the `https://hooks.slack.com/services/` prefix were
checked. A Slack incoming-webhook URL does not encode the channel name, so the
`#sop-alert` binding is the operator's assertion, not a value SOP can verify.

## Commands and delivery result

The probe used the existing `sop notify drain` delivery path. The producer was a
one-shot, untracked fixture that used the existing `notify.NewEvent`,
`notify.EncodeSlackPayload`, and `store.EnqueueNotification` APIs; it added no
product command or policy layer.

```sh
sop notify status --json     # pre-send
# one-shot fixture: enqueue the synthetic event into the durable outbox
sop notify drain             # existing bounded drain; performs the delivery
sop notify status --json     # post-send
```

Pre-send status (before the fixture was enqueued):

```json
{
  "active": false,
  "availability": "configured_unverified",
  "channel": "webhook",
  "delivered": 0,
  "deliveries": {},
  "exhausted": 0,
  "file_configured": false,
  "pending": 0,
  "reason": "No delivery has been confirmed yet: configuration alone does not prove the channel works. Run `sop notify test`.",
  "webhook_configured": true
}
```

After the fixture was enqueued, status reported `PENDING: 1` and `pending: 1`.
The drain result:

```text
Drain: claimed=1 delivered=1 retried=0 exhausted=0
```

Post-send status:

```json
{
  "active": true,
  "availability": "active",
  "channel": "webhook",
  "delivered": 1,
  "deliveries": { "DELIVERED": 1 },
  "exhausted": 0,
  "file_configured": false,
  "pending": 0,
  "reason": "",
  "webhook_configured": true
}
```

The durable rows agree with the CLI output:

| Row                        | Observed value                                                     |
| -------------------------- | ------------------------------------------------------------------ |
| Event id                   | `54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16` |
| Event kind / transition    | `completed` / `NOTIFY_TEST`                                        |
| Event outcome              | `LLM10 synthetic probe & <probe> > done`                           |
| Event next action          | `no action required; & <probe> > are inert`                        |
| Event recorded             | `2026-11-10T02:33:15.559453Z`                                      |
| Delivery channel / status  | `webhook` / `DELIVERED`                                            |
| Delivery attempt / maximum | `1` / `3`                                                          |
| Delivery `last_error`      | empty                                                              |
| Delivery `delivered_at`    | `2026-11-10T02:33:17.836286Z`                                      |

## Payload fixture

The live payload contained `&`, `<`, and `>` in both the outcome and the next
action, so it exercised the mrkdwn escaping that the deterministic tests cover.
It contained no mention, no credential, no real secret, no external URL, and no
external or tracking link.

Source event fields:

```text
outcome: LLM10 synthetic probe & <probe> > done
next: no action required; & <probe> > are inert
```

The encoder's Slack `text` (decoded from the JSON body) was:

```text
SOP completed in agentic-sop: event 54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16 (attempt 1, NOTIFY_TEST)
outcome: LLM10 synthetic probe &amp; &lt;probe&gt; &gt; done
next: no action required; &amp; &lt;probe&gt; &gt; are inert
```

The transmitted JSON body is the following (Go's JSON encoder also writes `&` as
the equivalent `\u0026`, which is valid JSON for the same character):

```json
{
  "text": "SOP completed in agentic-sop: event 54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16 (attempt 1, NOTIFY_TEST)\noutcome: LLM10 synthetic probe \u0026amp; \u0026lt;probe\u0026gt; \u0026gt; done\nnext: no action required; \u0026amp; \u0026lt;probe\u0026gt; \u0026gt; are inert",
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "SOP completed in agentic-sop: event 54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16 (attempt 1, NOTIFY_TEST)\noutcome: LLM10 synthetic probe \u0026amp; \u0026lt;probe\u0026gt; \u0026gt; done\nnext: no action required; \u0026amp; \u0026lt;probe\u0026gt; \u0026gt; are inert"
      }
    }
  ]
}
```

The escaping chain is deliberate: `slackMrkdwn` turns `& < >` into
`&amp; &lt; &gt;`, and JSON encoding then writes the remaining `&` as `\u0026`.
Slack decodes the JSON to the `&amp;`/`&lt;`/`&gt;` form and its mrkdwn parser
decodes those entities back to the original visible characters. The fixture shows
the bytes SOP sent; only a human looking at the channel can confirm how Slack
rendered them.

## Delivery versus rendering

| Observation                               | Result                                    | What it establishes                                                 | What it does not establish                                                                 |
| ----------------------------------------- | ----------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| HTTP 2xx from the configured endpoint     | observed (`AckConfirmed`)                 | The endpoint accepted the POST                                      | That the endpoint is the intended `#sop-alert` webhook, or that Slack rendered the message |
| Durable outbox status                     | `DELIVERED`, attempt 1, no retry          | SOP recorded one confirmed transport delivery                       | Slack-side rendering, mention behavior, or unfurl behavior                                 |
| `delivered=1`, `pending=0`, `exhausted=0` | observed                                  | SOP's recorded transport state                                      | The channel identity or a visible message                                                  |
| Payload fixture                           | escaped `&amp;`/`&lt;`/`&gt;` for `& < >` | The encoder escaped the mrkdwn-sensitive characters before the POST | That a client decoded or rendered the entities as intended                                 |
| Slack message visible in `#sop-alert`     | **not observed**                          | —                                                                   | Nothing about delivery or rendering was inferred without it                                |

The webhook client drains and discards the response body, so Slack's own `ok`
response was not inspected. A wrong-but-valid webhook can still return 2xx. Only a
human looking at the channel can confirm the destination and the rendered text.

## What this probe changes in the evidence state

- Live Slack **transport**: `VERIFIED` once at `0200ad3f` for the configured
  webhook, with one event delivered on attempt 1 and no retry.
- Live Slack **rendering**: `NOT_OBSERVED`. No authenticated Slack session existed
  on the test host: `https://app.slack.com/client` redirected to the workspace
  sign-in page. No channel history, message, permalink, or screenshot was
  available.
- Channel binding to `#sop-alert`: `NOT_CONFIRMED` by SOP; the operator named the
  channel in the authorization.
- The deterministic encoder and loopback transport evidence is unchanged and is
  not replaced by this probe.

## Exact observation still needed

A human with access to `#sop-alert` must confirm all of the following for the
message carrying event id
`54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16`:

1. the channel is `#sop-alert`;
2. the visible message shows the literal text
   `outcome: LLM10 synthetic probe & <probe> > done` and
   `next: no action required; & <probe> > are inert` (not raw `&amp;`/`&lt;`
   entities, and not a hyperlink);
3. no mention or notification ping occurred;
4. a permalink or screenshot is supplied for the record.

If the message is not visible, the configured webhook may be bound to a different
channel. Do not send another notification without a new explicit authorization;
the delivery already recorded above is the one authorized test.

## Residual limits (unchanged)

- Observed Slack rendering is a point-in-time human observation, not an automated
  check. This probe did not obtain that observation.
- Third-party client rendering and auto-unfurl remain external.
- Slack still interprets non-angular mrkdwn formatting and emoji shortcodes; the
  encoder neutralizes mentions and masked links, not all formatting.
- Windows terminal behavior remains `NOT_RUN`.
- Unicode bidi/format characters are bounded by the existing terminal handling,
  not removed.
- The probe used one platform (`Darwin arm64`) and one configured webhook. It does
  not generalize to other channels, workspaces, clients, or operating systems.

## Not claimed

- No claim that Slack rendered the message correctly, or that the channel is
  `#sop-alert`, until the observation above is recorded.
- No claim that the probe covers all adversarial payloads, mentions, masked links,
  control characters, or unfurl behavior.
- No manual acceptance, SOP lifecycle transition, or plan completion is implied by
  this transport evidence.
