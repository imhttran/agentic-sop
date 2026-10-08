# CONV-001 — Convergence Baseline (IMPLEMENT/FIX phased engine)

**Type:** Read-only baseline capture ("before" state)
**Status:** COMPLETE — no production change made
**Scope:** `internal/ollamaagent` (shared `executePhased` engine) and
`docs/specs/AGENT-PROVIDER.md` §9
**Plan of record:** `docs/plans/PLAN-Implementation-Convergence.md`

This report records the exact current convergence behavior of the shared
IMPLEMENT/FIX phased engine and the authoritative implementation points that
govern it, so the "before" state is unambiguous and the CONV-004 change is
anchored. It is a baseline artifact only: **no production (non-report) file was
modified.**

---

## 1. Authoritative inputs inspected

| Input | Role |
| --- | --- |
| `internal/ollamaagent/state.go` | `executionState`, counters, checkpoint bounds, stale accounting |
| `internal/ollamaagent/policy.go` | `CapabilityPolicy`, `policyFor`, all named thresholds, budget seam |
| `internal/ollamaagent/orchestrate.go` | `executePhased` loop, phase transitions, stale/repeat termination, steering, `staleLimitFor`, diagnostics |
| `internal/ollamaagent/protocol.go` | `turnProgress.observe`, `noProgressThreshold`, `actionFingerprint`, termination reasons |
| `internal/ollamaagent/evidence.go` | `discoveryIdentity`, `controlledMutation`, `commandMutates` |
| `internal/budget/budget.go` | `DefaultStaleIterations` (the arithmetic's second term) |
| `docs/specs/AGENT-PROVIDER.md` §9 | Normative spec of record |

---

## 2. The DISCOVER → CHANGE → FINALIZE state machine

`implementPhase` and its labels are defined in `internal/ollamaagent/orchestrate.go`:

```go
const (
	implDiscover implementPhase = iota
	implChange
	implFinalize
)
```

A new invocation starts in DISCOVER: `newExecutionState()` in
`internal/ollamaagent/state.go` returns `&executionState{phase: implDiscover}`.

### Phase entry points

| Phase | Entered when | Anchor |
| --- | --- | --- |
| **DISCOVER** | Invocation start (`newExecutionState`) | `state.go` `newExecutionState` |
| **CHANGE** | The **first successful, verified repository mutation** — the `justMutated` branch setting `st.phase = implChange` | `orchestrate.go` `executePhased`: `if justMutated && st.phase == implDiscover { st.phase = implChange; ... }` |
| **FINALIZE** | A mutated run crosses `CompletionWindow`/`FinalizeAfter` (`finalizeEligible`), crosses `ForceFinalizeAfter`, or an unmutated run reaches `LateStageAfter` | `orchestrate.go` `executePhased` (`st.phase = implFinalize`), `state.go` `finalizeEligible` |

### The exact point CHANGE is entered

CHANGE is entered only on the first verified mutation. The classification in
`executePhased` is:

```go
justMutated := candidate && toolErr == nil && observation.Succeeded &&
	observation.Verified && observation.Changed
if justMutated {
	st.observeMutation()
	st.counters.interactions++
	...
}
...
if justMutated && st.phase == implDiscover {
	st.phase = implChange
	h.recordImplementEvent(req, implementChangeEvent, "")
}
```

So all five conditions must hold: the tool is mutation-capable
(`candidate := controlledMutation(name, args)`), the tool did not error, the
observed mutation succeeded, verification was available, and repository state
differed. A failed, denied, unverified, or no-op write never enters CHANGE.

### FINALIZE transition (mutation-aware)

`state.go` `finalizeEligible(iteration, policy)` decides entry:

- A mutated run at/after `policy.ForceFinalizeAfter` is finalized regardless
  (checked in `executePhased` before the next model turn, so narration cannot
  outrun it).
- Otherwise finalization requires `counters.interactions >= policy.FinalizeAfter`;
  a mutated run then needs `sinceMutation >= policy.CompletionWindow` (still-writing
  protection), and an unmutated run needs `counters.interactions >= policy.LateStageAfter`.

FINALIZE is terminal: every tool request there, mutation included, is denied at
`executePhased` (`"tools are unavailable during IMPLEMENT finalization"`).

---

## 3. Progress semantics table (reproduced from the implementation)

Signals and their accounting, as encoded in `state.go` (`observeMutation`,
`observeDiscovery`, `stalled`, `countNonMutatingInteraction`), `protocol.go`
(`turnProgress.observe`, `actionFingerprint`), and `evidence.go`
(`discoveryIdentity`):

| Signal | Counts as progress? | Resets the stale streak? | Anchor |
| --- | --- | --- | --- |
| Model iteration (a turn) | no | no | `orchestrate.go` loop; every turn counts |
| File read (`read_file`, first-seen, non-empty) | "discovery" | yes, only while turn ≤ 12 | `evidence.go` `discoveryIdentity`; `state.go` `observeDiscovery` |
| Listing/search (first-seen, non-empty) | "discovery" | yes, only while turn ≤ 12 | `discoveryIdentity` (rejects `(empty)` / `(no matches)`) |
| Successful non-mutating command (`run_command` that cannot mutate) | "discovery" | yes, only while turn ≤ 12 | `discoveryIdentity` + `commandIdentity` |
| Failed read / empty listing / no-match search | no | no | `discoveryIdentity` early `return {}, false` |
| Repeated identical action | no (repetition guard) | no | `protocol.go` `turnProgress.observe`, `actionFingerprint` |
| Narration / denied tool call | no | no | `orchestrate.go` narration + denial branches |
| Test execution / test failure | no | no | test run is a non-mutating command; earns only discovery credit ≤ 12 |
| Phase transition | no | no | `recordImplementEvent` |
| Mutation attempt (failed, denied, or unverified) | no | no | `orchestrate.go` `mutation_failed` / `verification_unavailable` |
| **Successful verified repository mutation** | **yes (the only mutation signal)** | **yes** | `state.go` `observeMutation` |

Key implementation facts:

- `observeDiscovery` returns `false` when `st.mutationObserved` is already set or
  `iteration > implementNowAfter` (12), so discovery credit never extends past the
  window and never coexists with mutation.
- `stalled(mutated, discovered, staleLimit)` resets `consecutiveNoProgress` to 0
  when `mutated || discovered`, otherwise increments it and returns true only when
  no mutation has been observed and the streak reaches `staleLimit`.
- Only `observeMutation` resets the streak **unconditionally** (it also zeroes
  `sinceMutation` and increments `repositoryMutations`).
- `turnProgress.observe` is the independent repetition guard: identical
  fingerprints reach `noProgressThreshold = 3`, send `progressReminder`, and on one
  more repetition terminate.

---

## 4. Termination arithmetic

For a run that continuously performs credited discovery and never mutates, the
engine stops at:

```
implementNowAfter + staleIterations
= 12 + 5 = 17   (default)
```

**Sources:**

- `implementNowAfter = 12` — `internal/ollamaagent/policy.go` (soft threshold;
  after it, every non-mutating turn increments the stale streak).
- `staleIterations` — `internal/ollamaagent/policy.go`
  `func staleIterations(b budget.Budget) int { return b.StaleIterations }`, sourced
  from `internal/budget/budget.go` `DefaultStaleIterations = 5`.
- `maxNoProgressIterations = budget.DefaultStaleIterations` — `policy.go`.
- Spec of record: `docs/specs/AGENT-PROVIDER.md` §9 — "Continuous novel discovery
  with no mutation therefore stops by turn 17."

### Observed run evidence (from the governing plan)

| Run | Capability | staleIterations | iterations at stop | mutations |
| --- | --- | --- | --- | --- |
| CLEF-004 | IMPLEMENT | 8 | 20 | 0 |
| CLEF-005 | IMPLEMENT | 8 | 20 | 0 |
| CLEF-016 | IMPLEMENT | 5 | 17 | 0 |
| CLEF-014 | IMPLEMENT | 5 | 17 | 0 |
| CLEF-013 | FIX | 5 | 17 | 0 |

Every observed stop lands exactly on `12 + staleIterations`. The run is bounded
and fails closed, but is not *required* to converge: the model may consume the
full discovery window (12 turns) and then the stale allowance doing read-only work.

> **Evidence provenance.** The CLEF run artifacts live in the external
> `sop-decision-adapters` repository (`.agent-sdlc/runs/CLEF-014/`: `attempt.txt`,
> `trace.json`, `classification.json`, `report.md`) and are **not present in this
> repository**. This table reproduces the observed-run evidence carried in the
> governing plan (`docs/plans/PLAN-Implementation-Convergence.md`, "Termination
> arithmetic (verified across runs)"). Comparing against the preserved raw
> artifacts is CONV-006's scope and is out of scope here; the in-repo
> implementation (`implementNowAfter` + budget `StaleIterations`) and spec §9 are
> the authoritative source for this baseline.

---

## 5. Continuation / checkpoint behavior

Defined in `internal/ollamaagent/state.go`:

- `inspected []string` — the repository paths this invocation inspected (read,
  listed, or searched). First-seen ordered, deduplicated, and **paths only** —
  never file contents, prompts, or secrets.
- `const maxCheckpointFiles = 12` — bounds the checkpoint so a long discovery run
  cannot grow the recorded context without limit.
- `const maxCheckpointShown = 5` — bounds how many paths the diagnostic renders;
  the remainder is summarized as a count.
- `recordInspected(path)` — trims whitespace, no-ops on empty, skips duplicates,
  and stops appending once `maxCheckpointFiles` is reached.
- `inspectedSummary()` — renders the first `maxCheckpointShown` paths joined by
  `", "`, appending ` (+N more)` when more were recorded; returns `""` when
  nothing was inspected.

The checkpoint is populated in `executePhased` from `evidence.go`
`checkpointPath(name, args)` (for `read_file`, `list_files`, `search_files`). A
failed read is recorded too (the intent is the useful signal; only the path is
stored). The checkpoint is rendered by `implementNoProgressError` and
`noChangeError` as:

```
continuation checkpoint (phase=<PHASE>, inspected=<paths>): resume from the intended change rather than repeating repository discovery
```

---

## 6. NO_PROGRESS diagnostics and disposition

`implementNoProgressError` in `internal/ollamaagent/orchestrate.go` builds the
capability-specific diagnostic (e.g. `IMPLEMENT_NO_PROGRESS`, `FIX_NO_PROGRESS`):

```go
marker := strings.ToUpper(string(req.Capability)) + "_NO_PROGRESS"
msg := fmt.Sprintf("%s: the Ollama agent %s made no repository progress after %d consecutive stale iterations (provider=ollama, model=%s, iterations=%d, discovery_inspections=%d, repository_mutations=%d, changed_files=%d, tool_calls=%d, termination=%s%s); a retry may succeed", ...)
```

Recorded facts:

- Marker: `<CAPABILITY>_NO_PROGRESS` → `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS`.
- `termination=no_progress` (`terminationNoProgress` in `protocol.go`).
- `repository_mutations=0` for a stalled discovery run (grounded on
  `st.repositoryMutations`, not a model claim).
- The trailing `; a retry may succeed` marker keeps the failure classifier's
  **retryable-incomplete disposition (CONTINUE)** — the run is resumable, not a
  hard block.
- Returned as `*noProgressError` (`orchestrate.go`), whose doc comment states it is
  "a resumable continuation (retry, do not block)".
- The diagnostic carries the continuation checkpoint (§5) when anything was
  inspected.

### Distinct from early no-change completion

`IMPLEMENT_NO_PROGRESS` (stalled execution, `termination=no_progress`, reached via
the no-progress guard) is distinct from early `IMPLEMENT_NO_CHANGES` completion
(`termination=no_change`, via `noChangeError`/`changeIncompleteError`), which is
now the fallback path reachable only when `policy.MaxIterations` is configured
below the discovery window plus stale bound. §9 states the two MUST remain
distinct.

---

## 7. Threshold anchor summary

| Constant | Value | File |
| --- | --- | --- |
| `implementNudgeAfter` | 6 | `policy.go` |
| `implementNowAfter` | 12 | `policy.go` |
| `implementFinalizeAfter` | 18 | `policy.go` |
| `implementClosingAfter` | 19 | `policy.go` |
| `implementLateStageAfter` | 22 | `policy.go` |
| `implementForceFinalizeAfter` | 28 | `policy.go` |
| `implementFinalizeTurns` | 3 | `policy.go` |
| `implementCompletionWindow` | 2 | `policy.go` |
| `maxNoProgressIterations` | `budget.DefaultStaleIterations` (5) | `policy.go` / `internal/budget/budget.go` |
| `fixForceFinalizeAfter` | 20 | `policy.go` |
| `fixFinalizeTurns` | 3 | `policy.go` |
| `noProgressThreshold` | 3 | `protocol.go` |
| `maxCheckpointFiles` | 12 | `state.go` |
| `maxCheckpointShown` | 5 | `state.go` |

---

## 8. Baseline conclusion

The current engine is **bounded but not enforced**: it stops a non-mutating run at
`implementNowAfter + staleIterations` (12 + 5 = 17 by default) with a retryable
`*_NO_PROGRESS` diagnostic, while a single successful verified mutation
unconditionally resets the stale streak and enters CHANGE. This is the
unambiguous "before" state that CONV-004 is anchored to. No production file was
modified by this stage.
