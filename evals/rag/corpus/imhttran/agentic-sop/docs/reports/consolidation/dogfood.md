# End-to-end dogfood regression (MIG-011)

Bounded, non-destructive. No running oMLX/Nimble/controller/Caddy instance was
stopped, restarted or manipulated.

## Verified evidence

- **Candidate built from current source** (HEAD `54aa6a1a8f73a3b6debef0c54472351c2a817080`, dirty tree): `sop` sha256
  `5ea4151ac8835a36f977491b21751469ad4d9ac695c499afb0141bdcafa7c0a8`;
  `sop-controller` `6bbba3f5108a0ab048e086d4a4f1caf793da0ea1bf6a9aa5811dc7252ae94126`;
  `sop-decision-adapter` `0e0010edf97b3bfa5dc78189e8506a4b7e778abf0ab5371c87931c63a1f8ad3d`. The
  `sop` binary is byte-reproducible from the tree (`go build ./cmd/sop` yields the same hash).
- **Controller launches against a disposable project from another CWD**: starting
  `sop controller serve` outside any checkout with
  `SOP_CONTROLLER_ADDR=127.0.0.1:0 SOP_CONTROLLER_PROJECTS=/tmp/dogfood/project
SOP_BIN=<candidate sop>` printed the startup summary — _"SOP Controller bound to
  127.0.0.1:59366 (1 project(s), poll 3s, attention 1s)"_ — i.e. it built the
  read-only SOP client, discovered the disposable project and delegated to the
  **candidate** `sop` via `SOP_BIN`.
- **Auth enforced**: `GET /` → **401**; `GET /api/state` → _"authentication
  required"_. The dashboard is not silently open.
- **Clean stop**: SIGTERM stopped the controller; no listener remained.
- **Approval boundary holds**: across this migration, protected MIG-003 / MIG-003B
  work stopped at real `NEEDS_HUMAN` gates and no task was marked done from an
  agent's completion claim (SOP independently validated every completion).
- **Governed execution evidence** (real SOP runs, not mocked): `sop run` produced
  trace/validation/review/outcome artifacts throughout (`.agent-sdlc/runs/*`), and
  `sop validate` is **PASS** for build/unit-test/lint.

## Phase-4 check map

| Check                                           | Status   | Evidence                                                                                                                                                                                                                                                                                       |
| ----------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLI compatibility (`sop run`)                   | **PASS** | full suite + `sop run` lifecycle                                                                                                                                                                                                                                                               |
| Unified `sop controller serve` / `sop decision` | **PASS** | startup summary; `--help`; legacy wrapper runs the shared CLI                                                                                                                                                                                                                                  |
| Controller security / discovery / assets        | **PASS** | auth 401; embedded assets; allow-listed discovery                                                                                                                                                                                                                                              |
| Real governed execution through the controller  | **PASS** | authenticated `POST /projects/<p>/commands/run` returned HTTP 200 and the disposable project executed the runnable task (T001 `LOCAL_DONE`, `runs/T001/report.md` gate PASS); the Run action now delegates `sop run`, never the read-only `sop resume` (`sopclient/start.go`, `start_test.go`) |
| Provider evidence (oMLX / Bespoke)              | **PASS** | live CLEF/oMLX consultation served by the clef primary (`provider=clef model=clef-4bit`, ~1.7 s); live Nimble/Bespoke `/score` served (`bespokelabs/Bespoke-Nimble-9B`); no service manipulation                                                                                               |
| Simulated primary/backup failure                | **PASS** | `fallback_bounds_test.go` (malformed / invalid-confidence / deadline / cancellation / both-unavailable)                                                                                                                                                                                        |
| Deterministic fallback                          | **PASS** | SOP's deterministic decision provider remains terminal authority                                                                                                                                                                                                                               |
| Installer / skills                              | **PASS** | `internal/dist`, `internal/skill`                                                                                                                                                                                                                                                              |
| Startup from outside the checkout               | **PASS** | controller bound from `/tmp/dogfood`                                                                                                                                                                                                                                                           |
| Protected action stops at the boundary          | **PASS** | MIG-003 / MIG-003B gates                                                                                                                                                                                                                                                                       |

## Blocked / not run

- **Live both-unavailable with real backends: NOT RUN** — one of the two local backends
  (Nimble) is reachable, so a live "both unavailable" state cannot be produced without
  manipulating a service; the both-unavailable path is covered by fixtures
  (`fallback_bounds_test.go`).
- **oMLX/clef consultation with an ambiguous question id: FLAKY** — the local 4-bit model
  mis-keys the answer when the question id is not descriptive (see the diagnosis below);
  this is a model-prompting limitation, not a harness defect.

## Additional authenticated evidence

- `GET /login` returns the CSRF-protected form; `POST /login` with the operator token
  and CSRF → **HTTP 303** (session established); the authenticated `GET /` →
  **HTTP 302** (redirect to the discovered project page). So the dashboard
  authenticates, discovers the disposable project, and routes — all served by the
  candidate binary. The controller-triggered governed run is exercised in the final
  MIG-011 results below (Run now delegates `sop run`).

## Controller-triggered run attempt (historical, superseded below)

Authenticated with `X-Access-Token` (+ CSRF from the `sop_ctrl_csrf` cookie); a
`POST /projects/dogfood/commands/run` returned **HTTP 200** and the run status
went **running → done** ("sop run · done"), i.e. the controller accepted and
delegated a real SOP command against the disposable project.

**Original blocker (now fixed, see below):** the delegated command was
`StartOrContinue`, which selected **`sop resume`** (a read-only next-action) rather
than `sop run`, because `planStarted(detail)` reported the plan as started, so the
pending task was **not executed through the controller** by that path. This is retained
as history; the fix and its regression are recorded in _Final MIG-011 results_ below.

Fixture note: the disposable project uses the sanctioned existing `.git` baseline
(`63c2432`, no new commit); a fresh git-less directory made the controller's diff
fail (`git diff` with no HEAD).

## Final MIG-011 results (controller-triggered run fixed)

After correcting `internal/controller/sopclient/start.go` (Run now delegates `sop run`
for a started-but-incomplete plan; `resume` stays a separate read-only action), an
authenticated controller `POST /projects/dogfood/commands/run` returned **HTTP 200**,
the controller reported _"Ran the plan (sop run); SOP selected the runnable task."_,
and the disposable project executed the task: **T001 `LOCAL_DONE`**, with
`.agent-sdlc/runs/T001/report.md` recording **"## Gate — all required checks passed"**.
Regressions: `start_test.go` (delegated argv is `run`, never `resume`; a complete plan
dispatches nothing) and the corrected `web/dogfood_test.go` expectations.

## Bounded live probes (read-only, no service manipulation)

- **Nimble / Bespoke: PASS** — `sop decision decide -provider nimble` returned a real
  decision (model `bespokelabs/Bespoke-Nimble-9B`, choice `HIGH`, confidence 0.42).
- **CLEF / oMLX: PASS** — `sop decision decide -provider clef -decision-id change-risk
-choices LOW,HIGH` (the project `.env`: `CLEF_ENABLED=1`, `CLEF_TRANSPORT=chat`,
  `OLLAMA_BASE_URL=http://127.0.0.1:8000`, `CLEF_MODEL=clef-4bit`) returned a decision served
  by the **clef primary itself** — `provider=clef model=clef-4bit answers{change-risk:HIGH
confidence 1.0}` (no fallback diagnostic), in ~1.7 s. This is distinct from the earlier
  5 s-bounded probe that **did** hit its deadline (recorded below) and from the
  malformed-response observed for an ambiguous question id (recorded below).
- **Live CLEF→Nimble failover: PASS** — injecting an unreachable primary
  (`-provider clef -base-url http://127.0.0.1:1`, `SOP_DECISION_FALLBACK=nimble`) served from
  Nimble (`diagnostics {fallback:"primary unavailable", served_provider:"nimble"}`) in 0.42–0.53 s.
- **Installation verification: PASS** — `internal/dist` + `internal/skill` (isolated
  temp HOMEs, no personal install).
- **Disposable-state reset:** the controller-triggered-run fixture's `.agent-sdlc` was
  re-initialised between attempts (state only; source and `.git` baseline `63c2432`
  preserved). Earlier failed-run evidence is preserved under `/tmp/dogfood` and prior
  `.agent-sdlc/runs` were not rewritten.

## Decision latency

Measured on this host with the candidate binary (bounded, read-only):

| Path                                         | Latency       | Evidence                                          |
| -------------------------------------------- | ------------- | ------------------------------------------------- |
| CLEF/oMLX primary consultation (no fallback) | ≈ 1.55–1.86 s | 3 runs, `served_provider` absent, `provider=clef` |
| Injected primary failure → Nimble backup     | ≈ 0.42–0.53 s | `served_provider:nimble`                          |
| Nimble live decision                         | < 5 s         | `bespokelabs/Bespoke-Nimble-9B`                   |

All fit a bounded consultation budget; no operational regression vs the baseline for
this path.

## CLEF diagnosis across probe bounds (three distinct observations)

The three observations below are **separate, all preserved** — the earlier deadline
exceeded is a real observed event and is not retracted.

1. **5 s-bounded probe — deadline exceeded (observed).** An early probe with a 5 s bound
   returned `clef: malformed_response: decode chat response: context deadline exceeded`: the
   local oMLX/clef model did not answer within 5 s. This was a genuine deadline failure under
   a short bound, recorded as such.
2. **30 s-bound probes — backend responds (observed).** Repeated with the production 30 s
   bound, the endpoint returns HTTP 200 in ~4.1–4.8 s, so the 5 s failure was a too-tight
   bound for a backend whose first-token latency is that high — it was **not** an unavailable
   backend. Under the production bound it served a conforming decision for a descriptive id
   (`change-risk`, ~1.7 s), or for an ambiguous id returned a _malformed payload_, diagnosed next.
3. **Malformed payload for an ambiguous question id (diagnosed).** The default probe
   (`-decision-id example -state probe`) repeats the same free-text string as both the context
   and the question id (user message `Context: probe / Question example: probe …`). The 4-bit
   model then keys its answer by the context string (`{"answers":{"probe":{"LOW":0.6}}}`) or
   emits `{questionID: choiceString}` (`{"risk":"HIGH"}`, no confidence). The adapter
   correctly rejects both (unknown id / non-conforming answer object), so this is a
   **model-prompting limitation** for ambiguous ids — not an adapter defect and not a timeout.

- **Not a harness fix target:** `internal/decisionadapter/providers/clef/chat.go` is a
  byte-identical import of the sibling repo's `internal/providers/clef/chat.go` (modulo the
  module path), so its chat prompt is intentionally left unchanged to preserve MIG-003B
  source equivalence.
- **Verified successful consultation:** with a descriptive question id (`change-risk`) the
  clef primary serves a conforming decision (~1.7 s); see _Bounded live probes_ above.
- **Injected primary failure → Nimble backup: PASS** — `-provider clef -base-url
http://127.0.0.1:1` (injected unreachable) with `SOP_DECISION_FALLBACK=nimble` returned
  `diagnostics {fallback:"primary unavailable", served_provider:"nimble"}` in **0.42–0.53 s**.
- **Both-unavailable (full path): PASS via disposable fixtures.** The complete chain
  `SOP lifecycle → adapter connector → real unified `sop decision` → clef primary (unreachable)
→ nimble backup (unreachable) → SOP deterministic fallback` is now covered by
  `internal/cli TestDecisionEvidenceAdapterBothUnavailable` (bounded loopback port + a
  disposable `/score` testserver; no running service touched). It asserts
  `decision-evidence.json`: `outcome_class:"unavailable"`, `failure_label:"provider unavailable"`,
  empty choice/confidence, **no** `served_provider` attribution, and `baseline_action ==
final_action` (the governed `AUTO_CONTINUE` deterministic baseline stands). The backup-serves
  case in the same test asserts `outcome_class:"benign"`, `served_provider:"nimble"` and a
  non-empty `fallback_reason`.
- **Deterministic fallback:** SOP's own deterministic decision provider remains the terminal
  authority (core `internal/decision` tests + the new both-unavailable test above).
- **Provider attribution:** the failover diagnostic names the serving provider (`nimble`)
  while the result's `provider` stays the configured primary (`clef`) — attribution preserved;
  a fully unavailable chain attributes no provider.
- **Governance:** advisory results can only add attention; they cannot lower risk, approve
  work or bypass verification (`TestProviderOutputCarriesNoGovernanceAuthority`,
  `TestProviderFailureCarriesNoGovernanceAuthority`, core governance tests, and the new test).

**Consequence:** MIG-011's live CLEF consultation criterion is **VERIFIED** (a conforming
production-bound consultation succeeds). The 5 s deadline failure and the malformed-ambiguous
observation are both preserved above as distinct earlier events. The earlier
UNRESOLVED record above is retained as history: it combined a real 5 s deadline failure with
an ambiguous-id malformed response, both now separately explained.
