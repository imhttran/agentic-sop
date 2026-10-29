# Notifications

SOP records a durable notification event when background work **completes**,
**fails**, or **stops at a human decision**, and delivers it through one configured
channel. Events are not a second workflow: they are provenance derived from the
lifecycle transitions SOP already records, and a delivery failure never changes a
task's outcome.

## Configuration

```yaml
notifications:
  channel: none        # none | file | webhook
  file:
    path: .agent-sdlc/notifications/events.jsonl
  webhook:
    url: https://example.invalid/hook
  retry:
    max_attempts: 3
    base_delay_ms: 500
    max_delay_ms: 30000
    request_timeout_seconds: 10
```

An omitted block resolves to channel `none`, so an existing project is unchanged.
The block holds no credentials; a webhook URL is policy, and any secret belongs in
the environment of the receiver.

## Storing the webhook secret

The webhook URL is a credential. Keep it out of tracked configuration, shell
arguments, and shell history:

```bash
cd /path/to/project
read -rs SLACK_URL && echo && umask 077 && \
  SLACK_URL="$SLACK_URL" python3 -c \
  'import os; open(".env","a").write("SOP_NOTIFY_WEBHOOK_URL="+os.environ["SLACK_URL"]+"\n")'
```

`read -rs` does not echo, the value is never an argument, and `.env` is
gitignored. SOP resolves the URL from `SOP_NOTIFY_WEBHOOK_URL` at load time — the
process environment first, then the project's `.env` — so a **detached background
job resolves it too**, not just an interactive shell. A non-empty
`notifications.webhook.url` in `config.yaml` still takes precedence.

Slack incoming webhooks post a fixed body; select it with
`notifications.webhook.payload: slack`.

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
