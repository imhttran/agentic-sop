# Proposal: one credential detector, per-surface markers

Status: **PROPOSAL — P1 and P2 implemented; P3 not started.** This document
records the target design; the phases below say what has landed.

Related: the diagnostics redaction that exposed the divergence
(`internal/controller/sopclient`, PR #221 and its follow-up), and
[`PLAN-SOP-ACP-ZED.md`](PLAN-SOP-ACP-ZED.md) for the ACP diagnostic surface.

## Goal

Decide **in one place** whether a piece of text carries a credential, so a
credential cannot leak merely because the code path that printed it happened to
use a narrower pattern. Keep every surface's existing redaction marker.

## What exists today

Ten mechanisms across seventeen non-test files, with fourteen credential regexes:

| # | Location | Mechanism | Marker(s) |
| --- | --- | --- | --- |
| 1 | `internal/security/normalize.go:163` | field-key based | `[REDACTED]` |
| 2 | `internal/security/normalize.go:187` | exact known values | `[REDACTED]` |
| 3 | `internal/handoff/redact.go:12` | env name-marker → value replace | `[redacted]` |
| 4 | `internal/hipcompare/write.go:86` | patterns + real env values | `<redacted>`, `key=<redacted>` |
| 5 | `internal/outcomefile/outcomefile.go:416` | patterns, context preserving | `<redacted-token>`, `<redacted-access-key>`, `$1=<redacted>` |
| 6 | `internal/controller/sopclient/commands.go` | line-based patterns | `[redacted]` |
| 7 | `internal/acp/diagnostics.go:56` | patterns (prose-aware) + JWT | `[redacted]` |
| 8 | `internal/ollamaagent/activity.go:117` | `KEY=VALUE` name masking | `***` |
| 9 | `internal/config/notifications.go:264` | structured URL userinfo | (dropped) |
| 10 | `internal/notify/webhook.go`, `internal/cli/notify.go` | parsed URL + config driven | `<redacted>` |

## Why it matters

The detectors are not supersets of one another, so the same credential is caught
or missed depending on which surface prints it. Column numbers follow the table
above; mechanism 10 redacts structured URLs rather than free text, so it has no
column.

| Shape | 1/2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `api_key=…`, `token: …` | ✘ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✘ |
| `{"api_key": "…"}` (JSON body) | ✘ | ✘ | ✘ | ✘ | ✔ | ✔ | ✘ | ✘ |
| `Incorrect API key provided: …` (prose) | ✘ | ✘ | ✘ | ✘ | ✔ | ✔ | ✘ | ✘ |
| `sk-…`, `ghp_…`, `AKIA…` | ✘ | ✘ | ✘ | ✔ | ✔ | ✔ | ✘ | ✘ |
| JWT `eyJ…` | ✘ | ✘ | ✘ | ✘ | ✘ | ✔ | ✘ | ✘ |
| `Bearer …` | ✘ | ✘ | ✔ | ✔ | ✔ | ✔ | ✘ | ✘ |
| URL `scheme://user:pw@host` | ✘ | ✘ | ✘ | ✘ | ✔ | ✘ | ✘ | ✔ |
| PEM private-key header | ✘ | ✘ | ✘ | ✘ | ✔ | ✘ | ✘ | ✘ |
| **actual env secret values** | ✔ | ✔ | ✔ | ✘ | ✘ | ✘ | ✘ | ✘ |

Two concrete consequences of the current split:

- PR #221's first redaction pass missed the two shapes a provider error actually
  takes (a JSON body, and the prose form "Incorrect API key provided: …"),
  because only `internal/acp` had learned them.
- Only `handoff`, `hipcompare` and `security` scrub **real environment values**,
  which is the only rule that catches a credential of unknown shape.

The name vocabularies disagree too: `handoff` matches
`KEY/TOKEN/SECRET/PASSWORD/PASSWD/CREDENTIAL/AUTH`, `hipcompare` matches an
explicit prefix list, `ollamaagent` adds `SESSION` and `APIKEY`. A variable is
secret on one surface and not another.

## Decision: one detector, per-surface markers

**Locked.** Surfaces keep their own marker text and their own rendering policy:

| Surface | Rendering | Marker |
| --- | --- | --- |
| `outcomefile` | value only, key kept, per rule kind | `<redacted-token>`, `<redacted-access-key>`, `<redacted-key>`, `<redacted>` |
| `hipcompare` | value only, key kept | `<redacted>` |
| `acp` diagnostics | value only, key kept; token shapes whole | `[REDACTED]` |
| `sopclient` diagnostics | whole line | `[redacted]` |
| `handoff` | value replacement | `[redacted]` |
| `ollamaagent` | `KEY=***` | `***` |
| `security` | value replacement | `[REDACTED]` |

Rationale: these markers appear in **persisted artifacts** (outcome records,
handoff capsules, compare output) and in operator-facing text where keeping the
key is deliberate context ("`api_key=<redacted>`" says more than
"`[redacted]`"). Unifying the string would churn artifacts for no security gain;
what must be unified is the *decision* that a value is a credential.

## Design

A leaf package `internal/redact` (standard library only, so it can be imported
from any layer without a cycle).

```go
// Kind classifies what matched, so a surface can render its own marker per kind.
type Kind string

const (
	KindAssignment Kind = "assignment" // key=value where the key names a credential
	KindBearer     Kind = "bearer"
	KindToken      Kind = "token"      // provider prefixes: sk-, ghp_, xox[bap], github_pat
	KindAccessKey  Kind = "access-key" // AKIA…
	KindJWT        Kind = "jwt"
	KindPEM        Kind = "pem"
	KindURLUserinfo Kind = "url-userinfo"
)

// Marker renders the replacement for one match. Fixed(m) is the common case.
type Marker func(Kind) string

func Fixed(m string) Marker

// Shapes replaces credential-shaped text. Whole replaces the entire match;
// Values keeps the key and replaces only the captured value.
func Shapes(s string, m Marker) string
func ShapesValues(s string, m Marker) string

// Line replaces an entire line that looks credential-bearing with one marker,
// for surfaces where the line itself is not worth keeping.
func Line(s string, m Marker) string

// Values replaces exact known secret strings, longest first.
func Values(s string, secrets []string, m Marker) string

// EnvSecrets returns the values of environment variables whose names name a
// secret, longest first. One name vocabulary for every surface.
func EnvSecrets(env []string) []string
```

The pattern set is the **union** of what exists today, with the prose and JSON
forms from `acp`/`sopclient` and the token shapes from `outcomefile`/`acp`
included for every caller. `Line`'s behaviour (`sopclient`) and `ShapesValues`'
key-preserving behaviour (`outcomefile`, `hipcompare`, `acp`) are separate
renderers over one detector, not separate detectors.

### Not in scope

- **Structured redaction.** `internal/config/notifications.go` and
  `internal/notify/webhook.go` redact parsed URLs and configured notification
  targets. They operate on structure, not free text; folding them into a text
  detector would make the API worse. They stay as they are, referenced from this
  document so the next audit does not re-discover them.
- **Field-key redaction** (`security.redactValue`) is schema-aware and stays in
  `security`.

## Phases

Each phase is independently shippable.

| Phase | Scope | Risk | State |
| --- | --- | --- | --- |
| P1 | `internal/redact` + shared corpus; adopt in `acp`, `sopclient`, `ollamaagent` (nothing persisted) | low | **implemented** |
| P2 | `outcomefile`, `hipcompare` — persisted artifacts; adapters must reproduce today's markers | medium, golden tests | **implemented** |
| P3 | `handoff`, `security` — `EnvSecrets` + `Values`; retires `redactCapsule`, `redactList` duplication | medium | not started |

### What P2 changed in practice

- `outcomefile.Redact` and `hipcompare.redactText` are `redact.ShapesValues` calls.
  `outcomefile` renders its typed markers per kind (`<redacted-key>`,
  `<redacted-token>`, `<redacted-access-key>`, `<redacted>`), which required one
  addition to the shared detector: `KindKey` separates provider API keys (`sk-`,
  `rk-`, `pk-`) from tokens (`ghp_`, `github_pat_`, `xox[bap]-`).
- `hipcompare` keeps its curated environment-variable list for the arm executor's
  env scrubber. That is a narrower question than "does this text look like a
  credential": `redact.IsSecretName` matches `AUTH` as a segment, which would
  scrub `SSH_AUTH_SOCK` from an agent's environment.
- Both artifacts now redact shapes their own patterns never saw: JSON bodies,
  provider prose, `sk-`/`ghp_`/`AKIA` values and bearer tokens without a key name.
  Verified by running the same corpus through the previous and the current
  implementations.

Deliberate rendering changes, all of which keep the marker vocabulary:

| Case | Before | After |
| --- | --- | --- |
| `api_key: value` | `api_key=<redacted>` | `api_key: <redacted>` |
| `api_key="value"` | `api_key=<redacted>` | `api_key="<redacted>"` |
| `bearer <token>` (hipcompare) | `<redacted>` | `bearer <redacted>` |
| `Incorrect API key provided: sk-…` (outcomefile) | `<redacted-key>` | `<redacted>` |

The first two keep the artifact's own separator and quoting instead of
normalising it; the third keeps the scheme word for context; the fourth is a
token that reaches the detector as an assignment value, so it is classified as
one.

### What P1 changed in practice

- `internal/redact` owns the shapes; `sopclient.SanitizeDiagnostic` is now
  `redact.Line` with `[redacted]`, and the ACP's `redactLine` is
  `redact.ShapesValues` with `[REDACTED]`. Both keep their public behaviour and
  their tests.
- `ollamaagent` uses `redact.IsSecretName`, so a variable one surface treats as a
  secret is a secret on every surface. The old list matched `AUTH` as a substring,
  which also caught `GIT_AUTHOR_DATE`; segment matching fixes that.
- The ACP kept one rule of its own: `[A-Za-z0-9_-]{32,}`. Redacting a long opaque
  blob is right for a transport diagnostic, but it is not a credential shape —
  applying it everywhere would redact commit SHAs. It is documented in
  `internal/acp/diagnostics.go` as ACP-only.
- Matching is span-based: the first rule to claim a span wins, so text a rule has
  already replaced is never re-examined. That fixed a real leak in the ACP: its
  key rule ran before the bearer rule, so `Authorization: Bearer <token>` redacted
  the word "Bearer" and left the token.

## Tests

One corpus of about twenty cases shared by every adapter, covering the matrix
rows above plus false positives that must survive:
`token budget exhausted`, `GET https://example.com/v1/status`,
`sk-learn tutorial`, `internal/controller/sopclient/commands.go`.

Each adapter asserts it catches the full union and renders its own marker.
`outcomefile` and `hipcompare` keep their existing golden assertions, so any
marker drift fails loudly. A conformance test enumerates the adapters so a new
surface cannot be added without entering the corpus.

## Risks

- **Artifact churn.** Mitigated by per-surface markers (this decision) and by
  P2's acceptance criterion: golden output unchanged on the existing fixtures.
- **Stricter detection is lossier.** A unified detector will redact text that
  survives today (for example the prose "API key" form). That is the intent, but
  it makes diagnostics less informative; surfaces that need context keep it by
  choosing `ShapesValues` over `Line`.
- **Performance.** One compiled alternation; measure against the 1 MiB activity
  line and a large `sop run` output before adopting in hot paths.
- **Coordination.** `security`, `acp` and `hipcompare` have all changed recently.

## Open questions

1. P3: include `security`'s exact-value redaction, or leave it to the scanner
   boundary that produces the values? (Recommendation: include the *helper*, keep
   the scanner's value discovery where it is.)
2. Whether `ollamaagent`'s `***` marker should become `[redacted]` for
   consistency. It is display-only and not persisted, so this is free — but it is
   a marker change, so it needs an explicit yes.

## Acceptance criteria

- One exported detector decides credential-shaped text; each existing redactor
  becomes a thin adapter that renders its own marker.
- The shared corpus passes for every adapter.
- No persisted-artifact fixture changes.
- `go vet ./...`, the full test suite, and the Windows build stay green.
