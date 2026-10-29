# Notifications

SOP records a durable notification event when background work **completes**,
**fails**, or **stops at a human decision**, and delivers it through one configured
channel. Events are not a second workflow: they are provenance derived from the
lifecycle transitions SOP already records, and a delivery failure never changes a
task's outcome.

## Configuration

```yaml
notifications:
  channel: none        # none | file | webhook | slack
  file:
    path: .agent-sdlc/notifications/events.jsonl
  webhook:
    url: https://example.invalid/hook
  slack:
    url_env: SOP_SLACK_WEBHOOK_URL   # names the variable; the URL itself is never in this file
  retry:
    max_attempts: 3
    base_delay_ms: 500
    max_delay_ms: 30000
    request_timeout_seconds: 10
```

An omitted block resolves to channel `none`, so an existing project is unchanged.
The block holds no credentials; a webhook URL is policy, and any secret belongs in
the environment of the receiver.

## Slack

The `slack` channel posts each event as a text message to a Slack
[incoming webhook](https://api.slack.com/messaging/webhooks). The generic `webhook`
channel posts SOP's raw event JSON, which a Slack incoming webhook refuses (it
requires a `text` field), so use `slack` for Slack.

A Slack webhook URL is a **secret**: its path is what authorizes posting. SOP
therefore never reads it from the config file and never prints it.

```bash
export SOP_SLACK_WEBHOOK_URL='<your incoming-webhook URL>'   # in the environment of the process that runs SOP
sop notify test      # posts one test message
sop notify status    # reports active only after Slack confirms the post
```

- Set the variable in the environment of **every process that delivers events**, including
  background jobs started by an agent; a process without it reports the channel as
  `unavailable` and names the variable.
- `notifications.slack.url_env` renames the variable. It must be a variable *name*; a URL
  or token placed there is rejected, and an inline `slack.url` key is rejected as unknown.
- Slack does not honour the idempotency key, so the channel does not claim duplicate-free
  delivery: an event retried after an ambiguous attempt (timeout, 5xx) can post twice. The
  message ends with a short event id so a duplicate is recognizable.
- A revoked or deleted webhook returns a 4xx, which is recorded as rejected, not delivered.
- Event text is escaped so a reason cannot form a link or an `@` mention.

## Availability is observed, never assumed

`sop notify status` reports one of:

| Availability | Meaning |
| --- | --- |
| `unavailable` | no channel is configured (or the configured one lacks its required setting) |
| `configured_unverified` | a channel is configured, but no delivery has been confirmed |
| `active` | at least one delivery was confirmed on the current channel |
| `unknown` | delivery state could not be read |

Configuration alone never reports `active`. Run `sop notify test` to enqueue and
attempt one test event; only a confirmed acknowledgement moves the channel to
`active`.

## Delivery guarantees

- Delivery is **at-least-once** with deduplication by the stable event id.
- Duplicate-free delivery is claimed **only** for a channel whose capabilities prove
  it honours the idempotency key. The `file` channel deduplicates on the event id;
  the `webhook` channel sends `Idempotency-Key` and can only claim duplicate-free
  delivery if the receiver honours it.
- An **ambiguous** acknowledgement (timeout, transport error, 5xx) is recorded as
  ambiguous. It is neither a delivery nor a refusal, and a retry may duplicate on a
  receiver that does not honour the key.
- Retries are bounded by `retry.max_attempts`, which counts **total** attempts
  including the first. Backoff starts at `base_delay_ms` and doubles per attempt up
  to `max_delay_ms`. A delivery that spends its attempts is recorded `EXHAUSTED` and
  is never retried again.

## What is never exposed

No credential is written to an event, a log, an artifact, or status output. Status
reports only the non-secret channel settings.

## Operator runbook

```bash
sop notify status            # channel, availability, delivery counts
sop notify status --json     # the same document, machine-readable
sop notify test              # enqueue + attempt one test event
sop notify drain             # run one bounded drain pass
```

`sop run` and `sop prompt` attempt one bounded drain after they finish, so a retry
resumes on the next invocation after a restart. A delivery failure is isolated: it
is reported as a notification error and never changes the command's outcome.

## Agent handoff rule

Whenever starting background work, confirm whether notifications are active. If
notifications are unavailable, explicitly say so and offer a scheduled status check.
If notification status cannot be verified, report it as unverified. Do not claim a
status check is scheduled until scheduling succeeds.

## Limits

- The outbox and the task graph are separate stores. A lock serializes participating
  writers, but a crash between the JSON and SQLite writes can still leave them
  disagreeing.
- A job that outlives the process that started it records its own exit and event; a
  job started by a process that cannot pass its job identity is reported as `ended`.
- No production channel is enabled by this feature. Enabling one is an operator
  decision.
