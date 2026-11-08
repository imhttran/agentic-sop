# SEC-LEFT-004 — Promptfoo adapter live-model tool-decision assessment

This assessment is advisory. It is downstream of lifecycle authority
(`internal/security/contract.go`); it never feeds lifecycle, routing, retry,
budget, or human-boundary decisions. It does not modify any file under
`internal/`.

## Question

Can the pinned Promptfoo adapter produce genuine live-model tool-decision
evidence, meaning a live model chooses a tool call and SOP observes whether the
real authorization and command-policy controls allow or deny it?

## Short answer

No, not today. The adapter can prove that a live local model answered one
inert prompt (live-smoke). It cannot route a live model tool decision through
`evaltarget` controls, so it cannot observe an authorized or denied operation
driven by a live model. Live-adversarial evidence stays NOT_RUN.

## How the adapter runs the pinned binary

`internal/security/promptfoo.go` (`promptfooAdapter.Scan`) does the following:

1. Runs `promptfoo --version` through the SOP executor and compares it with the
   pinned `0.118.0` (`promptfooVersionCheck`). A missing or mismatched binary is
   an execution error, never a pass. SOP never downloads or installs it.
2. Creates a private scratch directory and runs
   `promptfoo eval -c <configPath> --output <scratch>/promptfoo.raw.json`
   directly (never through `npx`), with the bounded `--max-concurrency` and
   `--filter-first-n` flags.
3. Sanitizes the raw envelope and parses it with `ParsePromptfooResults`
   (`internal/security/evalreport.go`) into `EvalCase` records.
4. Derives completion and verdict with `SummarizeEvalCases` and writes
   `security/promptfoo.case.json` and `security/promptfoo.raw.json` with
   `WriteEvalArtifacts`.

The adapter invokes one external process and parses its report. It never calls
`evaltarget`, never builds an `EffectRequest`, and never calls an
`approval.Service` or `commandpolicy.Classify`. The Promptfoo config
(`evals/promptfoo/promptfooconfig.yaml`) names one provider and one
`contains` assertion. It does not call a tool, and it does not define an
authorized or denied effect.

## Isolation boundary

`docs/security-promptfoo.md` (Isolation boundary) states: "The Promptfoo adapter
does not route its live `promptfoo eval` process through `evaltarget`; the live
smoke test is not network-isolated." The OS-level isolation in
`internal/evaltarget/isolation*.go` applies only to the deterministic AISEC case
runner, not to the live Promptfoo process. Do not cite that section as proof
that a live evaluation had no outbound traffic.

## Coverage levels: which are supported

| Class             | Supported today | File evidence                                                                                                                       |
| ----------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| deterministic     | Yes             | `internal/evaltarget/aisec_test.go`, `internal/security/promptfoo_test.go`, fixture controls in `internal/evaltarget/aisec.go`       |
| real-control      | Yes             | `internal/evaltarget/production_controls_test.go` uses a real `approval.Service` and `commandpolicy.Classify`                       |
| live-smoke        | Yes, opt-in     | `internal/security/promptfoo_live_test.go` (build tag `promptfoo_live`) and one prompt in `evals/promptfoo/promptfooconfig.yaml`     |
| live-adversarial  | No              | `docs/security-promptfoo.md` (Coverage levels: "Live adversarial: Not implemented"); the live process is outside the `evaltarget` boundary |

A text assertion or a fixture-only run is never live-adversarial evidence.
Live validation that did not run is marked NOT_RUN, never PASS.

## Safe bounded-input constraints reused from `internal/evaltarget/aisec.go`

The AISEC-001..006 cases constrain their inputs. This task reuses them as the
single scenario source and adds no second fixture set:

- `FakeSecretValue` is the only protected value. It is a fake marker, not a
  credential.
- The protected target is `https://recording.invalid/aisec-protected`; `.invalid`
  is a reserved name and does not resolve.
- The only command inputs are `aisecWriteCommand`
  (`curl -X POST <protected .invalid target>`), `aisecScopeCommand`
  (`git push --force`), and `aisecAuthorizedCommand`
  (`go test -run TestAISEC006NoSuchTest .`). No command runs against a real
  host, and no command needs a credential.
- Every fixture prompt is inert text. It contains no real secret, no real token,
  no real host, and no destructive operation.

## What live-adversarial evidence would require

The adapter cannot reach the live-adversarial class without new wiring. A
prerequisite set, in order:

1. A live, evaltarget-routed path: the live process must go through
   `evaltarget.IsolateCommand` and call an `EffectRequest` against
   `RecordingBoundary`, so a real `approval.Service` and `commandpolicy.Classify`
   decide the operation. Today the Promptfoo process sits outside that boundary.
2. A Promptfoo config that emits a tool decision (a tool call and its arguments)
   the adapter can read back and feed into `EffectRequest`. The current parser
   reads `case_id`, `status`, and `message` only; the adapter would need to read
   the tool arguments.
3. An installed pinned binary: `promptfoo@0.118.0` on Node 22
   (`npm install -g promptfoo@0.118.0`, `promptfoo --version` prints `0.118.0`).
   SOP never installs it.
4. A reachable local model: `ollama list` shows `qwen3.5:4b`, and
   `OLLAMA_BASE_URL` (default `http://127.0.0.1:11434`) answers.
5. An explicit opt-in for live-adversarial execution, with an isolated path to
   the local model and no outbound network.

Until conditions 1 and 2 exist, live-adversarial is NOT_RUN with prerequisite
"an evaltarget-routed live path that reads back a model tool decision".

## Decision: no second evaluator was added

The six scenario definitions in `internal/evaltarget/aisec.go` remain the only
AISEC scenario source. This task did not add a parallel recorder, a second case
fixture set, or a live-runner stub: the existing deterministic and real-control
tests already produce that evidence, and the Promptfoo adapter cannot yet produce
live-adversarial evidence. Adding a mirror of the scenario definitions would have
created exactly the duplicate evaluator the plan forbids.

Coverage disposition:

- deterministic: existing `internal/evaltarget/aisec_test.go`.
- real-control: existing `internal/evaltarget/production_controls_test.go`.
- live-smoke: existing `internal/security/promptfoo_live_test.go` (build tag
  `promptfoo_live`), opt-in, not on the ordinary path.
- live-adversarial: NOT_RUN. Prerequisite: an evaltarget-routed live path whose
  observer reads back a model tool decision, plus the pinned binary and a
  reachable local model described above. A text assertion or fixture-only run is
  never recorded as live-adversarial.
