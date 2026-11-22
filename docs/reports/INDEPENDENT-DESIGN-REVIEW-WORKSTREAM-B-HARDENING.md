# Independent Design Review — Workstream B Execution Hardening

**Type:** Point-in-time verification report (non-normative). It closes or bounds the
remaining execution-path limitations recorded in
[INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md](INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md)
and [WORKSTREAM-B-RECONCILIATION.md](independent-design-review-corpus/WORKSTREAM-B-RECONCILIATION.md).
It is **not** an experiment result, authorizes no experiment, pilot or real inference,
changes no threshold or eligibility rule, and does not touch the corpus or its freeze.

**Baseline (sections 1-8):** `origin/main` @ `5a94dfe1` (PRs #574, #577 and #579 are on it). No change was
made to SOP routing, providers, prompt construction or the execution lease. The only code
added is experiment-local: [`internal/expharness`](../../internal/expharness) and its thin
front end [`cmd/expcheck`](../../cmd/expcheck). Nothing in SOP calls either.

**Evidence classes** are those of the earlier report: **CODE**, **EXISTING TEST**,
**SYNTHETIC RUNTIME** (a real `sop` binary, an explicit environment, local fake providers
behind a recording proxy; no real provider, model or credential) and **PILOT**. A result
is not called resolved from CODE alone. Reproduce:
`go test ./internal/expharness -count=1 -race`.

**Platform:** the task brief assumed Linux; this was run on macOS (Darwin, arm64). Nothing
in the code is platform-specific. Two macOS observations: `/bin/true` is absent (not used
here), and the operator's ambient shell already exports `SOP_*` variables
(`SOP_OMLX_*`, `SOP_CONTROLLER_*`), so `expcheck preflight` fails in it by design; run it
and `sop` under `env -i PATH=… HOME=…` plus the allow-list.

## 1. Routing hazards and the control for each

| Hazard | Evidence | Control |
| --- | --- | --- |
| `SOP_AGENT_PROVIDER` / `SOP_AGENT_HARNESS` win over config and are **not** reflected in `routing.provider` | SYNTHETIC RUNTIME `TestInheritedProviderOverrideIsInvisibleInRoutingButVisibleOnTheWire`: routed `ollama`, `routing.provider` says `ollama`, the request went to `/v1/chat/completions` (the MLX client) | Preflight rejects any `SOP_*` variable outside an explicit allow-list; the proxy records the protocol path actually spoken |
| `SOP_AGENT_MODEL` and provider model variables change the model sent while `routing.model` keeps the routed name | SYNTHETIC RUNTIME `TestInheritedModelOverrideIsDetectedByProxyAndPreflight` (also the earlier `TestVerify*OverrideChangesRequestedModel`) | Same allow-list; attribution compares the model the **provider received** (proxy) with `routing.model` and with the `tokens` key |
| `SOP_MODEL_*` class variables replace `models.<class>` fields | CODE (`internal/model`) + preflight tests | Allow-list, plus SOP's own `model.Resolve` run against the supplied environment must equal the overlay, so an override fails even if its variable were allow-listed (`TestPreflightResolverCatchesAllowlistedOverride`) |
| Project `.env` is applied at startup | EXISTING `TestVerifyDotEnvCanInjectModelOverride` | Preflight fails when `.env` exists in the project |
| An undefined class routes to the built-in table (cloud `medium`) | EXISTING `TestVerifyModelClassWithoutModelsBlockRoutesToBuiltInDefault` | Preflight requires provider, name **and** locality on `models.small`, `medium` and `large`, an approved provider list, and rejects a non-local class unless a cloud decision is recorded |
| **New finding:** the `small` class carries a built-in cloud fallback candidate (`ollama` / `nemotron-3-nano:30b-cloud`) that **an overlay cannot remove**: a local `small` with no `fallback:` block still resolves with `LocalFallback` set | SYNTHETIC (resolver) `TestPreflightSmallClassCarriesUnremovableCloudFallback` against `internal/model` | The experiment must not use `--model-class small`. Preflight fails when a used class has a fallback candidate and notes it for an unused one. `medium` and `large` have none. The candidate only runs when SOP's local probe reports the runtime healthy but unable to serve the model (`applyLocalFallback`, covered by `routing_local_fallback_test.go`); this report did not re-test that trigger |
| `models.allow_cloud_fallback_for_local`, `routing_enabled`, `escalation_enabled`, `replan_enabled` could change class or model after the request | CODE | Preflight rejects each when true |
| History after a plan's cutoff reachable from the clone | Protocol §5.2 | Optional `--cutoff SHA`: HEAD must equal it and `git rev-list --all --not HEAD` must be empty (`TestPreflightCutoffRequiresNoLaterHistory`) |

**What the preflight cannot prove.** It checks configuration, the supplied environment and
SOP's own resolver. It does not probe the network and does not prove what the provider ran.
The harness is checked as written in the overlay (`agent.harness`); a harness override
variable is caught by the allow-list rather than by observing the harness.

**Prevented-fallback claim.** Cloud fallback is prevented by: no cloud class without a
recorded decision, no `fallback:` blocks, no `small`, no automatic routing/escalation, and
a call-ceilinged proxy in front of the only endpoint. It is **not** prevented at the
provider: a configured local daemon that itself proxies a `:cloud` model (as Ollama does)
looks "local" to SOP. The overlay's `locality` is the operator's statement, not a probe.

## 2. Evidence attribution

`expharness.Attribute` assembles one record per run from `metadata.json`, the run's own
proxy records and the tool-audit file. Everything below is **experiment-local**; no
production telemetry changed.

| Field | Source | Status |
| --- | --- | --- |
| Requested provider / model / class | `metadata.json` `routing` (what SOP selected) | recorded |
| Effective provider | protocol path and upstream seen by the proxy (`/api/chat` vs `/v1/chat/completions`) | observed on the wire; **not** an identity |
| Requested vs sent model | proxy-observed request body vs `routing.model` and `tokens` key | recorded; mismatch is a problem |
| Provider-returned model identifier | `model` field of the response body | recorded **if the server sends one**; self-reported, **not attestation** |
| Effective harness | overlay value, valid only with a passing preflight | recorded with that condition |
| Environment-override state | preflight result for that project and environment | recorded |
| Run-directory identity | absolute path, `work_item_id`, directory-name match | recorded; mismatch is a problem |
| Tool invocations | `SOP_TOOL_AUDIT_LOG` (existing SOP sink, JSONL) | observed (`TestNativeToolUseAndRetriesAreObservable`: one `read_file`, `ok`, no file content in the record). "Sink absent" and "sink set, nothing audited" are recorded as different states |
| Native retry count | proxy forwarded requests minus `tokens.calls` | observed as **extra provider requests** (the test shows 3 requests, 2 counted calls). It counts failed or retried requests together; it cannot say which |
| Reported vs unreported tokens | SOP `tokens` (incl. `unreported_calls`) next to proxy-parsed usage | both recorded; missing or unreported usage is a problem |
| Output of record | sha256 of `result.md`; sha256 of the raw provider response per request | hashes only; the raw response body is not kept |

No provider attestation is claimed anywhere: the record's `identity_basis` states "client-side
name plus proxy-observed request; a server-returned model field is self-reported, not
attestation; provider-side substitution is not detectable".

**Ceilings.** The proxy enforces a **hard call ceiling** per run and a reported-token **stop
threshold**, and answers `429` without forwarding beyond either. The token threshold is
compared with responses already received, so it does not bound a single response or the
requests in flight: worst-case spend is the threshold plus the output of the in-flight
requests, and SOP has no per-request generation limit to cap it (`TestProxyCeilingsRefuseWithoutForwarding`,
`TestFailureAndCeilingLeaveNoRunDirectory`). A native client may retry a `429`; refused
requests are recorded and never reach the provider. Zero or negative ceilings refuse
everything, so a ceiling has to be set deliberately. The call ceiling is the hard cost bound; tokens are a stop threshold, not money.

## 3. Instruction and input boundaries

**What SOP sends** (SYNTHETIC RUNTIME `TestRenderedRequestReachesProviderVerbatimInsideSOPWrapper`,
text-only path): one user message `Capability: REVIEW` / `Task:` + the request file
**verbatim** / `Output requirements:` SOP's fixed line ("Return concise review findings with
severities. Do not modify the repository.") / SOP's writing-rule block. Consequences:

- SOP's own output requirement comes **after** the experiment's request and is prose-shaped;
  a template that demands JSON conflicts with it. The conflict is identical across arms on
  one harness, so it is not a fairness defect, but the template's output requirements must
  be written knowing it follows. The native harness adds a different system prompt (EP-11).
- The wrapper text is identical for every arm, so arms still differ only in the request file.

**Deterministic request template.** `expharness.Render` produces, byte-stably, four sections
in fixed order: review instructions; permitted repository references; the plan inside a
fence whose token is derived from the plan's sha256 (and refused if the plan contains it);
output requirements. `ScanRequest`, `ScanTree` (files of 1 MiB or more are reported as unscanned, not as clean) and `CheckRefs` (the deny list applies to a symlink's resolved target too) check a rendered request,
the project tree and the reference list for operator-supplied forbidden terms (answer-key and
later-defect vocabulary), denied paths, absolute or escaping paths and symlinks leaving the
project.

**A delimiter is not a security boundary.** The fence only makes the plan's extent
unambiguous and unforgeable by a closing marker. A plan can still contain text that steers
the model (`TestRenderedRequest…` includes a plan with a fake `## OUTPUT REQUIREMENTS`; it
stays inside the fence, which says nothing about how the model weighs it). What remains
dependent on controls outside this change:

1. **Isolation:** one project per call; the project contains only that run's request and the
   permitted references; the answer key, manifest, other arms' requests and other runs'
   outputs live outside it (protocol §5.1).
2. **Repository access controls:** the native harness can read everything in the project
   (read-only). Leakage is prevented by what is in the clone, which `ScanTree`,
   `CheckRefs` and `--cutoff` check but cannot make true on their own.
3. **The review harness:** SOP does not enforce an instruction/data boundary on this path.
4. **Process working directory:** the native agent roots its tools at the process working
   directory, not at the project SOP resolves, so the call must run with `cwd` = the project.

## 4. Concurrency and isolation (SYNTHETIC RUNTIME, real binary)

| Check | Test | Result |
| --- | --- | --- |
| Sequential run identity | `TestSameProjectProcessesKeepSeparateRuns` | a later run never reuses an id |
| Concurrent run identity, same project | same test, 4 processes | 4 reported = 4 on disk (PR #574 fix still effective) |
| Separate projects, concurrent | `TestThreeConcurrentCallsAreIsolatedAndAttributed` | 3 distinct absolute run directories |
| Artifact non-overwrite | both tests | each run keeps its own `metadata.json`, `result.md` |
| Request-file isolation | the three-call test | each provider saw only its own marker; no run's `prompt.md` holds another's |
| Environment isolation | `TestInherited*` + explicit `cmd.Env` | a harness-built environment is not the parent's; an inherited override is detected two ways |
| Failure cleanup | `TestFailureAndCeilingLeaveNoRunDirectory` | provider `500` and ceiling refusal leave no empty run directory and nothing crosses the ceiling |

The experiment should keep one project per call; these tests show same-project concurrency is
safe, not that it is preferable.

## 5. EP-15 verification readiness (still UNVERIFIED)

**Dry run done:** a synthetic provider that holds each reply until all three requests are in
flight confirms the harness issues three calls concurrently, one project each, with clean
attribution (`TestThreeConcurrentCallsAreIsolatedAndAttributed`), and the proxy's ceilings
hold. Concurrency is observed at the provider, not inferred from wall time: a first version
that compared parallel and serialized batch wall times failed on loaded CI and was replaced.
This proves the **procedure and instruments**, not any real provider's latency or
concurrency. EP-15 stays UNVERIFIED.

**Bounded procedure, to run only after the operator names provider, model, call count and
ceiling:**

1. Three projects, each with the approved overlay (`expcheck preflight` passes in the run
   environment), a synthetic request (no corpus content, no repository data), and its own
   audit-file path outside the project.
2. Per project, `expcheck proxy --upstream <provider> --max-calls N --max-tokens T --log
   proxy-i.json` (one proxy per run; non-loopback upstream needs
   `--allow-nonlocal-upstream`, which is the operator's authorization switch). Point the
   provider's base-URL variable at the proxy.
3. One process starts all three `sop prompt` calls together under one timer; record wall
   clock for the batch and per-request `duration_ms` from the proxies. Request sha256 is the
   proxy's `req_sha256` and the request file hash; output hashes are `result.md` and
   `resp_sha256`.
4. `expcheck attribute` per run; the batch is valid only if all three have no problems and
   three distinct absolute run directories.
5. Ceiling: `N` and `T` set per proxy (suggest `N = 2`; `T` the operator's token stop
   threshold). Hard bound: `3 × N` calls. Tokens: about `3 × T` plus any overshoot by
   in-flight responses; keep requests short so one response cannot be large.
6. Interpret: wall time near one call's duration means parallel; near the sum means
   serialized. Record it; do not compare with a pilot.

No real inference was run. If the operator does not authorize it, EP-15 stays UNVERIFIED and
C-arm latency is first measured in the full run, as the protocol already says.

## 6. Reconciliation of the remaining limitations

The task brief's EP labels differ from the authoritative report in three places (its EP-09
is native tool-use evidence, EP-10 execution and isolation, EP-16 native retries). The
authoritative definitions are used: EP-09 failure handling and retry, EP-10 evidence
persistence, EP-16 inspection behavior of real models. The brief's themes are placed where
they actually live.

| ID | Limitation | Classification after this work | Basis |
| --- | --- | --- | --- |
| EP-01 | Routing is deterministic | **Resolvable through configuration — done:** validated overlay plus preflight | §1 tests. New: do not use `small` |
| EP-02 | Provider-attested model identity | **Accepted limitation requiring operator disposition.** Bounded instrumentation now also records the server-returned `model` field (self-reported) | No client or server attests identity; whether a given provider returns the field is real-provider-dependent |
| EP-09 | Native retries invisible in `tokens` | **Resolvable through bounded instrumentation — done on synthetic runtime:** proxy count vs `tokens.calls` | `TestNativeToolUseAndRetriesAreObservable`. Confirmation against a real provider is folded into the EP-15 call |
| EP-10 | Raw reply not preserved (native `result.md` is re-serialized) | **Bounded:** `result.md` hash remains the output of record; raw-response hash added. Body not kept. Operator may choose to keep bodies | Hash-only by design |
| EP-12 | Single-call timing valid, parallel timing unmeasured; reproducibility | Timing: **bounded instrumentation** (per-request duration plus one-process timer); parallel needs EP-15. Reproducibility (no sampling controls, provider non-determinism): **accepted limitation requiring operator disposition** | §5; SOP exposes no sampling parameters |
| EP-15 | Provider concurrency and batch latency | **Requires real-provider verification.** UNVERIFIED retained | §5 |
| EP-16 | Does a real model ever inspect the repository? | **Requires real-provider verification.** The mechanism and the **observation instrument** (tool audit sink) are now verified, so a real run can say "0 tool calls" instead of inferring it from `calls` | `TestNativeToolUseAndRetriesAreObservable` |
| EP-05 / WS-B-04 | No persisted tool trace | **Resolvable through configuration — done:** `SOP_TOOL_AUDIT_LOG` per run, outside the project | existing SOP sink, verified |

## 7. Operator decisions needed

1. **Accept or reject** the EP-02 disposition: model attribution is client-side plus
   proxy-observed request, with a self-reported server `model` field where present.
2. **Accept or reject** the EP-12 reproducibility limitation (no sampling control; repetition
   carries the variance).
3. **Authorize or decline EP-15:** provider, model, call count (suggested 3 requests, one per
   project, `N = 2` ceiling each) and token/cost ceiling. Without authorization EP-15 stays
   UNVERIFIED.
4. **Keep or drop raw provider response bodies** in the manifest (hashes are recorded either
   way).
5. **Confirm the experiment never uses `--model-class small`**, because its built-in cloud
   fallback cannot be configured away.

## 8. Conduct

- No real inference, cloud call or credential was used; every provider was a local synthetic
  server. No pilot was repeated, no corpus file touched, no threshold changed.
- One observation worth noting for the freeze record: the ambient shell on the verifying
  machine exports several `SOP_*` variables. They did not affect these tests (explicit
  environments) but would trip the preflight, which is its purpose.
- The model-based `sop review` gate was not run for the same reason as in the earlier report
  (it would send the diff to a cloud model without separate authorization); see the PR for
  the gates that were.

## 9. Independent re-verification and consolidated readiness (2026-11-21)

**Type:** documentation update only. **Baseline:** `origin/main` @ `38e20a52` (PR #588
merged as `e829e5d9`). No code, default, threshold, corpus file or decision record was
changed. **Platform:** macOS (Darwin); the code under test has no platform-specific path
(this report's earlier `/bin/true` note stands). No real-provider request was sent.

**Re-run offline on this baseline** (no inference): `go test ./internal/expharness
-count=1 -race` (16 tests pass, including `TestThreeConcurrentCallsAreIsolatedAndAttributed`)
and `go test ./internal/model ./internal/cli ./internal/perf ./internal/ollamaagent
-count=1 -run 'Verify|Routing|Prompt|Usage|Fallback|Resolve'` (all pass). Code re-read:
`internal/model/model.go` (`DefaultRoute`), `internal/cli/prompt.go` (`promptRunID`),
`internal/ollamaagent/config.go`, `internal/agent/provider.go`.

**Label mapping.** The task brief's seven labels are matched to the authoritative EP
definitions (§6) by theme, not by number: request construction → EP-01 plus §3;
attribution → EP-02; tool-use evidence → EP-05/EP-16; isolation and concurrency →
EP-06/07/08; reproducibility → EP-10/EP-12; provider concurrency → EP-15; retries and
usage → EP-09/EP-03.

### 9.1 Control results

| Theme (authoritative ID) | Implementation / tests | Verified | Not established | Live provider needed? |
| --- | --- | --- | --- | --- |
| Request construction, boundaries (EP-01, §3) | `expharness.Render/ScanRequest/CheckRefs`; `TestRenderedRequestReachesProviderVerbatimInsideSOPWrapper` | Request reaches the provider verbatim inside SOP's fixed wrapper; fence is byte-stable | No instruction/data boundary exists (delimiter only); model obedience | Yes, for model behavior only |
| Identity, attribution (EP-02) | `expharness.Attribute`; `TestInheritedModelOverrideIsDetectedByProxyAndPreflight` | Sent model equals routed model; override detected | Provider-side substitution invisible; returned `model` is self-reported | Yes, whether a provider returns an id |
| Tool-use evidence (EP-05/EP-16) | `SOP_TOOL_AUDIT_LOG`; `TestNativeToolUseAndRetriesAreObservable` | Tool call observable on synthetic runtime | Whether a real model inspects the repository (0 of 4 in Pilot B) | Yes |
| Isolation, concurrency (EP-06/07/08) | `promptRunID` (`os.Mkdir` reservation, suffix on `IsExist`); `TestSameProjectProcessesKeepSeparateRuns`, `TestFailureAndCeilingLeaveNoRunDirectory` | Atomic id, distinct run dirs, no cross-run request leakage, clean failure | Provider-side behavior under load | No for local; EP-15 for provider |
| Reproducibility, sampling (EP-10/EP-12) | `ollamaagent.Config`, `agent.provider` request structs | Manifest and hash reproducibility; per-request duration | SOP sends **no temperature, seed or top_p**; only an output cap (`num_predict`/`max_tokens`, default 16384, `agent.DefaultMaxOutputTokens`). Model behavior is not reproducible; repetition carries variance | Yes, for variance |
| Provider concurrency, latency (EP-15) | Gated synthetic provider; proxy ceilings | Procedure and instruments only | Real concurrency and batch latency | **Yes** |
| Retries, usage (EP-09/EP-03) | proxy count vs `tokens.calls`; `internal/perf` tests | Retried native turns visible as extra proxy requests; `unreported_calls` handled | Real-provider usage fields; which requests were retries. Default call timeout is 120 s (`defaultTimeout`) | Yes, for real usage fields |

Result classes after this pass (no PASS granted that the evidence does not support):
**Verified (synthetic/code):** EP-01 mechanism, EP-06/07/08, EP-03 mechanics.
**Partially verified:** EP-02, EP-05/EP-16 (instrument verified, behavior not), EP-09,
EP-10, EP-12. **Unverified:** EP-15. This matches the earlier classification; nothing was
upgraded.

### 9.2 Provider configuration, re-confirmed

- **`small` cloud fallback is still present.** `DefaultRoute().Small` is local
  `qwen3:4b` with a `Fallback` to cloud `ollama` / `nemotron-3-nano:30b-cloud`; an overlay
  cannot remove it. `medium` and `large` have **no fallback but are cloud-by-default**
  (`nemotron-3-super:cloud`, `deepseek-v4.1-flash:cloud`), so an undefined `medium` or
  `large` routes to cloud (EP-13).
- **Environment overrides** that can redirect a run include `SOP_AGENT_PROVIDER`,
  `SOP_AGENT_HARNESS`, `SOP_AGENT_MODEL`, `SOP_OLLAMA_MODEL`/`BASE_URL`,
  `SOP_OPENAI_COMPATIBLE_BASE_URL`, `SOP_OMLX_*` and `SOP_MODEL_*`.
- **Configuration that prevents unintended cloud routing** (unchanged from §1; production
  defaults untouched): a validated overlay defining `small`/`medium`/`large` with explicit
  provider, name and `locality: local`; no `fallback:` blocks and never `--model-class small`;
  routing, escalation, replan and `allow_cloud_fallback_for_local` off; `expcheck preflight`
  passing under `env -i` plus the allow-list; the call-ceilinged proxy in front of the only
  endpoint. Residual: a local Ollama daemon can itself serve a `:cloud` model and still look
  local to SOP, so `locality` is the operator's statement, not a probe.
- **Raw-response retention:** hashes only (`result.md`, `resp_sha256`); bodies are not kept
  unless the operator decides otherwise (§7 item 4).

### 9.3 Remaining execution risks

1. Real-provider concurrency and batch latency unmeasured (EP-15); a missing C timing
   makes H2 INCONCLUSIVE.
2. Model identity is client-side only; provider-side substitution is undetectable.
3. No sampling control; run-to-run variance is uncontrolled.
4. Whether a real reviewer uses repository tools is unobserved.
5. SOP's trailing output-requirement line conflicts with a JSON-demanding template.
6. Ambient `SOP_*` variables on the operator's machine fail preflight (by design).
7. Token ceiling is a stop threshold, not a spend bound; the call ceiling is the hard bound.
8. A prior unintended cloud call (earlier report) is outside the experiment data.

### 9.4 EP-15 live-verification plan (not executed)

Procedure is §5. Bounds to fix before any run: **provider and model** named by the
operator, one `sop` build recorded by sha256; **3 concurrent requests**, one project each,
synthetic request only; **per-proxy** `--max-calls 2` (hard bound 6 provider calls) and an
operator token threshold `T` (about `3 × T` plus in-flight overshoot); timeout is SOP's
120 s per call, with the batch aborted after one full timeout interval; **abort** on any
`429` from a ceiling, a non-matching model in the request, a preflight failure, any
unexpected host, or an attribution problem; **evidence** is the three `expcheck attribute`
records, proxy logs, request and output hashes and the batch wall clock; **cost** bounded by
the call ceiling and `T`, and recorded as an estimate only. **Limits:** one batch measures
one provider at one time; it does not predict full-run latency and is not comparable with a
pilot.

**Exact operator authorization required before a live run:** (a) provider and model id,
(b) confirmation that cloud use is intended or that the endpoint is local, (c) call count
(3) and ceiling `N`, (d) token threshold `T` and any cost cap, (e) the credential source,
(f) permission to pass `--allow-nonlocal-upstream` if the upstream is not loopback, and
(g) a statement that no corpus or repository content is sent.

### 9.5 Operator decisions, corpus dependencies, minimum evidence

- **Operator decisions:** the five items in §7 remain open (EP-02 disposition, EP-12
  disposition, EP-15 authorization, raw-body retention, no `small`). Decisions 5–9 in the
  [operator package](independent-design-review-corpus/OPERATOR-DECISION-PACKAGE.md) are
  **not approved** here.
- **Corpus governance dependencies (kept separate):** eligibility rule, floor, seeder,
  seeded and no-known-defect cases, two human adjudicators, thresholds and freeze remain
  with the corpus workstream. Execution readiness neither depends on nor decides them.
- **Minimum evidence before experiment authorization:** a passing preflight record for the
  frozen overlay; a completed EP-15 live batch (or an explicit decision to accept it
  UNVERIFIED, which makes C latency first-measured and H2 INCONCLUSIVE if missing); the
  operator dispositions in §7; and the freeze record citing the limits above.

### 9.6 Statement

**Execution readiness is NOT established.** The harness, isolation and attribution
instruments are verified on synthetic runtime; EP-15 is UNVERIFIED, EP-02, EP-05/16, EP-09,
EP-10 and EP-12 are PARTIAL, and no real-provider run has occurred. This section authorizes
nothing.
