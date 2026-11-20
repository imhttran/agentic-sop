# SOP Redaction and Deliverable-Path Fixes

**Status:** Implemented (RDP-001–RDP-003; PRs #557–#559).
**Baseline:** `main` at `f4610c0e`.

## Project

agentic-sop

## Summary

Two SOP defects surfaced while running `docs/plans/PONYTAIL-DEBT.md` through
`sop run`. Both are reproducible from that run's evidence.

1. A deliverable that names a dot-directory is verified with its leading dot
   removed. `.github/workflows/ci.yml` is checked as `github/workflows/ci.yml`,
   so the deliverable always reports "not present". The task that declared it
   could not reach `VERIFIED`.
2. The model-input redaction treats any value of a secret-named environment
   variable as a secret. Placeholder keys whose value is an ordinary word
   (`LLAMACPP_API_KEY=local`) make SOP redact that word everywhere in model
   input. Because an implementing agent rewrites whole files, the redaction is
   written back to disk and corrupts unrelated lines.

A third defect made validation unusable in this workspace, but it is a test
isolation gap rather than a product bug, so it is bounded to tests.

Out of scope: changing what counts as a credential beyond the bounded heuristic
in RDP-002, adding a general DLP layer, and changing the `.env` loader's
precedence.

## RDP-001 — Keep a leading dot on a deliverable path

`deliverablePaths` in `internal/cli/outcome.go` trims each bare deliverable
token with `strings.Trim` over backticks, quotes, apostrophes, commas, and
periods (line 848). Trimming `.` from both ends turns `.github/workflows/ci.yml`
into `github/workflows/ci.yml`.

Strip at most one pair of enclosing backticks or quotes. Remove only trailing
sentence punctuation: a trailing `.` or `,`. Never remove a leading dot.

Extend the table test in `internal/cli/outcome_deliverable_paths_test.go` with a
case named `keeps a dot-directory path`. Cover three inputs: `.github/workflows/ci.yml`
is returned unchanged; `internal/cli/outcome.go.` loses the trailing period; a
backticked dot path loses the backticks and keeps the dot.

Change nothing else.

### Deliverables

- internal/cli/outcome.go
- internal/cli/outcome_deliverable_paths_test.go

### Dependencies

- None

### Acceptance Criteria

- A dot-directory deliverable keeps its leading dot.
- A trailing sentence period is still removed.
- The deliverable-path tests pass.

### Acceptance Checks

- `go test -run TestDeliverablePaths ./internal/cli`
- `go test ./internal/cli`
- `go build ./internal/cli`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2

## RDP-002 — Do not treat a common word as a secret

`EnvSecrets` in `internal/redact/redact.go` (line 118) returns every value of a
variable whose name matches `secretNameMarkers`. It applies no check on the
value, so a placeholder such as `local` becomes a redaction target for every
occurrence of that word in model input.

Require a candidate value to look like a credential before it is treated as a
secret. Values shorter than 8 bytes are skipped. Values of at least 16 bytes are
kept even when they are all lowercase letters, so a long passphrase still
redacts. Between those bounds a value is kept only when it carries a byte that is
not a lowercase ASCII letter: a digit, an uppercase letter, or a symbol. That
skips placeholders such as `local` while keeping keys, hexadecimal digests, and
mixed-case tokens. Document the tradeoff in a comment: a short lowercase word is
more likely a placeholder than a credential, and redacting it corrupts
repository text that the agent must write back.

Add a table test `TestEnvSecretsSkipsCommonWords` in
`internal/redact/redact_test.go`. Cover a secret-named variable whose value is
`local` (skipped), a token-shaped value (kept), a 20-byte lowercase value (kept),
and an empty value (skipped). Build the token at run time, for example with
`strings.Repeat`, so the fixture carries no credential-shaped literal.

Update the fixtures in `internal/handoff/handoff_test.go` that rely on a short
lowercase fake secret, so the test proves redaction with a realistic secret
value instead of a placeholder.

Do not change `IsSecretName`, `Values`, `ShapesValues`, or the inventory path.

### Deliverables

- internal/redact/redact.go
- internal/redact/redact_test.go
- internal/handoff/handoff_test.go

### Dependencies

- None

### Acceptance Criteria

- A secret-named variable whose value is `local` is not returned by `EnvSecrets`.
- A secret-named variable with a token-shaped value is still returned.
- The redaction package tests pass.

### Acceptance Checks

- `git grep -q "func TestEnvSecretsSkipsCommonWords" -- internal/redact`
- `go test -run TestEnvSecretsSkipsCommonWords ./internal/redact`
- `go test ./internal/redact`

### Acceptance Mapping

- criterion 1: check 2
- criterion 2: check 2
- criterion 3: check 3

## RDP-003 — Make notification tests independent of the ambient environment

`go test ./...` fails in a workspace whose `.env` exports
`SOP_NOTIFY_CHANNEL`, because the notification tests parse configuration while
that variable is inherited. The failing tests are
`TestNotificationsAbsentMeansNone` (`internal/config/notifications_test.go`),
`TestNotifyStatusDistinguishesConfigurationFromDelivery` and
`TestNotifyDrainWithoutChannelReportsUnavailable` (`internal/cli/notify_test.go`),
`TestNotifyStatusRemovedConfigurationIsUnavailable`
(`internal/cli/notify_status_test.go`), and
`TestAcceptanceNotifyStatusReportsConfigurationAndExhaustion`
(`internal/cli/notify_acceptance_test.go`).

Make the affected tests control the notification environment themselves. Clear
`SOP_NOTIFY_CHANNEL` and `SOP_NOTIFY_WEBHOOK_URL` with `t.Setenv` before parsing
in each test that asserts "no notification configuration", so the assertion does
not depend on the developer's environment or CI.

Add `TestNotificationsIgnoreAmbientChannel` in `internal/config`. It sets
`SOP_NOTIFY_CHANNEL=webhook` with `t.Setenv`, parses a configuration with no
`notifications` section, and asserts the channel is `none`.

Do not change product code.

### Deliverables

- internal/config/notifications_test.go
- internal/cli/notify_test.go
- internal/cli/notify_status_test.go
- internal/cli/notify_acceptance_test.go

### Dependencies

- None

### Acceptance Criteria

- The new test exists and passes with an ambient channel exported.
- The configuration package passes.
- The CLI notification tests pass.

### Acceptance Checks

- `git grep -q "func TestNotificationsIgnoreAmbientChannel" -- internal/config`
- `go test -run TestNotificationsIgnoreAmbientChannel ./internal/config`
- `go test ./internal/config`
- `go test -run Notify ./internal/cli`

### Acceptance Mapping

- criterion 1: check 2
- criterion 2: check 3
- criterion 3: check 4
