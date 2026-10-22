# Learn from normal SOP work

Goal: accumulate source-backed experience from everyday runs and retrieve it during
later planning and failure recovery. This is retrieval from engineering memory, not
model training. More records do not by themselves prove better decisions.

## Enable this workspace

After installing a version with these commands, run from the SOP project:

```sh
sop memory enable
sop memory status
```

Enable writes `advisory_retrieval` in `.agent-sdlc/config.yaml`. It preserves existing
settings and fills missing values with workspace-local absolute index/outcome paths,
`failure` and `outcome` corpora, a 2000 ms retrieval budget, a 5000 ms capture budget,
five results, and a 2000-token budget. Existing custom values stay intact.

Environment variables win over the file: `SOP_ADVISORY_RETRIEVAL_*` overrides are applied
before the command validates and prepares storage. Enable resolves that effective
configuration, checks the corpora and budgets against the retrieval boundary, prepares
the effective index parent and outcomes directory, and only then writes the file. It
reports the workspace flag separately from the effective flag and lists the overrides
that applied in the current shell. With `SOP_ADVISORY_RETRIEVAL_INDEX` set to a new
`.../index.db`, enable prepares that directory and later capture uses it. An invalid
effective configuration (for example an unknown corpus, a budget above the retrieval
ceiling, a relative path, or a missing required value) fails before anything is written
and names what to fix. Re-run enable after correcting invalid existing values; the
command does not silently replace them. Absolute paths must be updated if the workspace
moves.

Run SOP normally. Automatic capture records completed outcomes and updates the local
SQLite index. Planning and recovery can retrieve that history on later attempts.
An empty initial memory is expected. No separate benchmark is required for use.
Installer defaults remain off. To pause capture and retrieval without deleting data:

```sh
sop memory disable
```

Disable never needs healthy memory storage: it writes `enabled: false` even when the
preserved paths are unusable. An environment override that still enables memory is
reported, because the environment takes precedence.

## Observe the loop

`sop memory status` reads configuration and the five most recently updated run
observations. It does not open or repair the index. It reports the applied environment
overrides, retrieval phase, status, failure reason, elapsed time, retrieved/included
counts, source references, and capture status with indexed, excluded and rejected
counts. An unhealthy index is reported when normal retrieval/capture encounters it;
status is not an integrity scan.

Run artifacts include:

- `rag.json`: latest retrieval and prompt-inclusion observation.
- `memory-retrieval-<timestamp>.json`: retained observations, including updates after
  prompt compilation. Group by task and attempt; multiple observations are not
  independent decisions. Use the latest observation for an attempt.
- `memory-capture.json` and `memory-capture-<timestamp>.json`: latest and retained
  capture status, reason, duration, indexed/excluded/rejected counts, and bounded
  diagnostic reasons for rejected outcome files.
- Outcome documents: authoritative recorded results, verification and source links.
  Their source artifacts now include `rag.json` when present.

These records distinguish a failed retrieval from a successful query with no matches.
Prompt inclusion only proves that the source reached the prompt. It does not prove
that the model followed the lesson or that the lesson caused success.

Capture has four distinct statuses:

- `succeeded`: the outcome was written and every selected document was indexed.
- `degraded`: at least one outcome document was rejected (for example malformed JSON,
  an unknown schema, or a hash mismatch). Every valid document stays indexed. The run
  prints a degraded line, telemetry records up to five rejected file names with bounded
  reasons, and `sop memory status` shows them.
- `failed`: capture or the index rebuild did not complete. The run reports it and keeps
  the task verdict.
- `disabled`: workspace memory is off (or an environment override turned it off), so no
  capture was attempted.

## Reliability and authority

Optional retrieval failure supplies no memory context. Capture failure is reported
without changing the task verdict, and a rejected outcome document degrades capture
without discarding the valid records. The retrieval budget covers index initialization,
source scans and all requested corpora together. Capture has a separate budget.
Database operations honor cancellation; source scans check it between files. A blocked
individual operating-system filesystem call is not forcibly interrupted. Telemetry is
best effort and must not stop execution.

Memory remains untrusted advisory evidence. Current code, explicit required evidence,
and approval gates retain authority. A successful historical outcome is a candidate,
not an automatically applied fix. Failed/blocked outcomes remain failure history.

## Assess improvement later

Keep real observations and inspect failures as they occur. Once enough relevant history
exists, compare decisions using source-backed tasks. Do not infer causality from prompt
inclusion followed by a passing run. Explicit adopted/rejected feedback and feedback-based
ranking are future work; this change does not manufacture those labels or promote records
based on repeated retrieval. Retained observations have no automatic retention policy;
archive them when needed and preserve outcome provenance for later evaluation.
