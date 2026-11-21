# Comprehension Report

**Target:** `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` (lines 1-110)
**Reviewer:** `egm-comprehension` (completed; no extra context)

**What this proposes**

The document does not propose a new design. It records an open, unmet requirement: only one execution may be active per project, whichever interface starts it (CLI, MCP or ACP). It says the current change set only partly delivers this. The document says what is proven (ACP and MCP starts exclude each other) and what is not (exclusion between `sop run` and the ACP/MCP-started work). The fix it points to is a project-scoped "execution lease" held for the whole life of an execution and honoured by every interface. Part of that lease is built and the rest is not. The document stays open until the remaining wiring lands and is covered by deterministic tests.

**How the existing system works, per this document**

ACP and MCP starts go through `startWork`. It takes a file lock (`.agent-sdlc/runs/work/.lock`), re-reads the persisted started-work records, and refuses duplicates or a competing running job. It also refuses if a `sop run` is mid-task. That start lock is released once the child process is launched, so it does not cover the execution's lifetime. A separate project lock (`state.lock`) protects plan metadata and approvals only. CLI `sop run` and `sop prompt` take no execution lock, and `sop run` does not look at the started-work records. An optional lease (`execution.lease`, default `off`) now exists. `sop run` acquires it when the setting is `required`, and under `advisory` it records contention. It heartbeats while running. `sop resume` reports a held lease and releases a stale one. `sop prompt`, MCP and ACP neither take nor check the lease.

## Findings

1. **Major** - "Evidence to cite" / closing paragraph (lines 97-101, 103-110). Line 101 says the gap closes when the remaining wiring is "evidenced by the deterministic tests below". No such tests are listed. The section below it cites one existing test, which covers only the ACP/MCP exclusion that is already proven. A reader cannot tell what test would show closure. The document also does not list the remaining wiring items. They have to be inferred from line 97 and are never stated as a checklist.

2. **Major** - Lines 7-9, 11-17, 78. The document depends on `PLAN-SOP-ACP-ZED.md` ("stage ACP-003", the acceptance-matrix row) and `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`. It does not summarise what ACP-003 is or what the proposal specifies beyond one sentence. That includes the "crash/stale acceptance case" on line 91, which is referred to as if known. A reader who opens only this file cannot judge what "closed" means. A short inline statement of the acceptance criteria would fix this.

3. **Major** - Lines 17-35 and 80-84. Several project-specific terms are never defined:
   - ACP, MCP and `sop`. The expansions are not given, nor is what each interface is. "ACP bridge" is used without explanation, and so is "child `sop prompt`".
   - "governed launch" and "work item" (lines 33 and 109).
   - `HeartbeatLimit`. The "about `HeartbeatLimit/5`" refresh rate (line 88) uses a name that is neither defined nor given a value.
   - "started-work records", "fingerprint" and "`already_ran`".
   - "mid-run task state". The text does not say what task transitions are, so the claim "between task transitions it can start a second execution" (lines 52-54) is hard to follow.

4. **Major** - Lines 66-72 ("Why this is not a regression"). The text is internally confusing. It mentions "the ACP bridge's earlier design" that held the metadata lock, and "the probe-only form", with no account of when or why the design changed. The conclusion is that no exclusion was lost. But the text seems to say the earlier design released the lock too soon to exclude anything either. A reader cannot tell whether the earlier design gave any extra exclusion that the current one lacks.

5. **Major** - Lines 80-95. This part mixes status with design and reads as a changelog ("now exists", "now wired"). Three things are unclear:
   - Which `execution.lease` modes exist. `off`, `advisory` and `required` appear, but the effect of each is not tabulated. "Records contention under advisory" does not say where it records it or who sees it.
   - Line 85 says CLI-first is "now detectable", while line 99 says a `sop run` cannot see an ACP-started child. The lease does not cover the reverse direction. A matrix like the one at lines 42-48 would show which of the four races (CLI-first/ACP-second, ACP-first/CLI-second, and so on) are closed under each setting.
   - The table at lines 42-48 has not been updated for the lease. It shows "none" for CLI `sop run` in the Execution/start lock column, which now conflicts with lines 80-84.

6. **Nit** - Line 44, 62-64. The table's "see `internal/cli/acp.go`" note and the "probe only" wording are terse. It is not explained what is being probed (lock availability?) or why a probe is released immediately. Line 58 offers the reason only indirectly.

7. **Nit** - Lines 52-54 and 55-58. "Observe a mid-run task state" and "an ACP/MCP-started `sop prompt` child is still running" assume the reader knows how a `sop run` differs from a `sop prompt`. Neither command's purpose is stated.

8. **Nit** - Line 91 ("kill the owner, the lease is not stolen automatically"). "Owner", "stale" and "pid liveness" are used without definition. The staleness rule is also not given: is it by heartbeat age, by pid check, or by both?

Paraphrase check: the reviewer could paraphrase every section. The "What would close it" section is the weakest. Its status and its acceptance criteria are interleaved, so what remains to be done is stated only by inference.

_Severity note: the reviewer labelled item 6 "Minor/Nit"; recorded here as Nit._
