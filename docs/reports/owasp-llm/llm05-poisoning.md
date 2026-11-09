# Plan 05 — Data and model poisoning (LLM05): owned poisoning boundaries

Status: report-only trace. This document records what SOP owns, what an external
provider owns, and which confirmed gaps were fixed at the current baseline. It adds
no classifier, trust service, poisoning-detection claim, or runtime control.

Task: `POISON-001` (`docs/history/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`).
Dependency: `POISON-002` (outcome-ingestion integrity repair).
Recovery stage: `POISON-003` (recovery and invalidation evidence, provenance
dimensions, Plan 09 handoff).

Read this report as a source-to-consumer trace. It is not a compliance
certification, and it does not claim that ingested data is benign.

## Scope and method

The trace covers every ingestion path SOP writes or reads itself:

1. repository documents and the structural repository index;
2. external or retrieved content (RAG);
3. historical outcome evidence (persisted run artifacts);
4. persistent decision memory;
5. model selection recorded in a run report.

For each path the report names the writer, the source identity, the revision or
content digest, the trust classification, and the consumer. It then answers whether an
imported record can claim a verified outcome or override authoritative state, and
names the function that decides it.

A missing control that would require new policy is recorded as `GAP`, not fixed
here. Evidence states used in this report:

- `IMPLEMENTED` — the code path exists at the cited file and symbol.
- `ENABLED` — the path runs in the default workflow at the cited call site.
- `VERIFIED` — a focused test or command passed on the recorded platform.
- `GAP` — supported behaviour is absent or unproven.
- `NOT_RUN` — evidence was not produced for this report.
- `NOT_APPLICABLE` — the path does not exist or does not apply.

## Source-to-consumer trace

### Path 1 — Repository documents and the structural repository index

| Field | Value |
| --- | --- |
| Source | Files in the repository working tree (Go sources, Markdown, YAML, JSON, TOML, config) |
| Writer | `internal/repoindex` `Build` (`internal/repoindex/index.go`) |
| Source identity | Repository-relative path in `File.Path` and `Document.Path` |
| Revision / digest | `Identity.Digest` — SHA-256 over `(relative path, file content SHA-256)` pairs for every indexed file, truncated to 16 hex characters; `Identity.Head` records the commit when supplied |
| Trust classification | `DocumentClass` (`plan`, `spec`, `history`, `documentation`, `config`, `other`) plus `Authority` (`current`, `historical`, `unknown`) with `AuthoritySource` = `sop` \| `header` \| `path` \| `unknown` |
| Consumer | Index artifact `FileName` = `context/index.json` under the SOP state directory, for later Context Engine consumption; building the index invokes no model |
| Digest scope note | The per-file content hash feeds the single index digest only. `File` and `Document` rows carry path, kind, class and authority — they do **not** carry a per-file digest. An individual document row is therefore not digest-verified on its own; the index identity as a whole is. |

### Path 2 — External or retrieved content (RAG)

| Field | Value |
| --- | --- |
| Source | Configured external retrieval endpoint; retrieved run artifacts such as `rag.json` |
| Writer | `internal/ragclient` (`ragclient.go`, `exec.go`) performs retrieval; `internal/cli` `writeRAGTelemetry` (`rag_verifier.go`) writes the run's `rag.json`; `internal/outcomefile` `IngestRunFor` digests that file as a source artifact of the outcome record |
| Source identity | The retrieval endpoint identity from configuration, and the artifact path (`<run dir>/rag.json`) for the ingested record |
| Revision / digest | Each retrieved `ragclient.Evidence` item carries a `document_hash` reported by the retrieval server; SOP does not recompute it from the returned text. For the ingested record, the SHA-256 in `SourceDigests[<absolute artifact path>]` and `SourceHash` over `report.json` |
| Trust classification | Untrusted external content. It enters SOP only as advisory record text (`Text`, `Rationale`) and the record's rationale, failure-signature and uncertainty fields pass through `outcomefile.Redact` before they are written |
| Consumer | The outcome record, then the memory index (`internal/memindex`), then advisory retrieval |
| Handoff | Retrieval query isolation belongs to Plan 09 (`docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`). This report does not claim query-isolation coverage. |

### Path 3 — Historical outcome evidence

| Field | Value |
| --- | --- |
| Source | Persisted run directory `report.json`, `counts.json` and sibling artifacts (`report.md`, `state.json`, `trace.json`, `rag.json`) |
| Writer | `internal/outcomefile` `IngestRunFor` (writes the record), `Write` (atomic versioned document) |
| Source identity | `TaskID` from the run directory; the report must declare the same `id` (`declaredID != taskID` is rejected). `RunID` = `<taskID>-attempt-<attempt>` |
| Revision / digest | `SourceHash` = SHA-256 of `report.json`; `SourceDigests` = SHA-256 per immutable artifact, keyed by absolute path; document `ContentHash` = `Hash(body)` |
| Trust classification | `Status` — `success` only when `successCorroborated` agrees; otherwise `partial` / `failed` / `blocked`. `RationaleSource` = `observed` for ingested fields. Advisory records stay usable as data |
| Consumer | `internal/memindex` `RebuildFromDir`, then `LocalRetriever`; the eligibility gate `Eligible` and `GateWithSourcesAndDigests` |

### Path 4 — Persistent decision memory

| Field | Value |
| --- | --- |
| Source | Operator-recorded engineering decisions (`sop memory add --decision --reason [--scope] [--evidence] [--source]`) |
| Writer | `internal/cli` `runMemoryAdd` (`memory.go`) → `decisionmemory.Store.Add` (`internal/decisionmemory/decisionmemory.go`) → `saveDecisionStore`, which writes `.agent-sdlc/context/decisions.json` with `os.WriteFile` |
| Source identity | `Record.Source` (default `operator`) and `Record.Repository` = `verifcache.RepositoryIdentity(HEAD, dirty, "")` at the time of recording; `Record.ID()` = SHA-256 over scope, repository, source, decision, reason and sorted evidence |
| Revision / digest | `Record.ID()` identifies a record's content. The file itself carries no digest or signature, and nothing checks who wrote it. `Store.Add` rejects a record with no decision, scope, reason, repository or source; `Unmarshal` drops invalid records and loads an empty store for an unknown schema |
| Trust classification | Advisory context. Records become `sopctx.Item`s with `Source: memory` and `Priority: PriorityGeneric` (5), which ranks below task, lifecycle, repository, execution and recovery evidence and above only optional RAG knowledge (6) |
| Consumer | `internal/cli` `applicableDecisionItems` (`run.go`) adds the records to the bounded agent context through `Store.Applicable`, which returns only records whose `Repository` equals the current `RepositoryIdentity(HEAD, dirty, "")`. Records from another HEAD or dirty state are `Stale` and are reported by `sop memory list`, not applied |
| Not part of this path | `internal/memindex` does not read `decisions.json`; it indexes outcome records (Path 3). `internal/decisionadapter` is the decision-provider seam, not decision memory |
| Removal | `sop memory remove --id <id>` (`runMemoryRemove` → `Store.Remove`) deletes exactly one record; `sop memory list` prints each record's `id`. An unknown id fails and leaves the file unchanged (`GAP-7`, fixed) |

### Path 5 — Model selection recorded in a run report

| Field | Value |
| --- | --- |
| Source | `model_selection` in `report.json` (provider, model, locality, source, fallback) |
| Writer | The run recorder; ingested by `internal/outcomefile` `IngestRunFor` |
| Source identity | Provider and model names as declared by the configuration used for the run |
| Revision / digest | None. The provider model revision and weight provenance are not recorded and cannot be verified from this repository |
| Trust classification | Provider-declared, not SOP-verified. Copied verbatim into `Environment` |
| Consumer | The outcome record `Environment` field, for human context only |

## Ownership designations

| Path | Owning layer | Designation |
| --- | --- | --- |
| 1 — Repository documents and structural index | SOP (`internal/repoindex`) | SOP-owned. SOP writes and reads the index it builds. |
| 2 — External / retrieved content | External source; SOP owns only acceptance | SOP accepts retrieved content as untrusted advisory text. Query isolation is Plan 09. |
| 3 — Historical outcome evidence | SOP (`internal/outcomefile`, `internal/memindex`) | SOP-owned and gated. |
| 4 — Persistent decision memory | SOP (`internal/decisionmemory`, `internal/cli` `memory.go` / `run.go`) | SOP-owned advisory context. Not evidence, and never an outcome record. |
| 5 — Model selection metadata | External provider | Provider-owned value carried as advisory context, not as verified evidence. |
| Training, fine-tuning, weight loading | External provider | Provider-owned boundary. Outside direct SOP control. Not marked secure. See §Provider-owned boundary. |

## Verified-override analysis

For each path, this section answers: can an imported record claim a verified outcome or
override authoritative state?

| Path | Deciding function | Outcome under the current contract |
| --- | --- | --- |
| 1 | `internal/repoindex` `Build`, `classifyAuthority` | An indexed document cannot claim a verified run outcome. It receives a deterministic `DocumentClass` and `Authority` (`current`, `historical`, `unknown`), so a document header can at most claim document authority, never execution evidence. A new content change yields a new `Identity.Digest`. |
| 2 | `internal/outcomefile` `IngestRunFor` (`Redact`), then `Eligible` | Retrieved content cannot create a success. It enters record text as advisory data and is redacted; a success claim must still pass `successCorroborated`. |
| 3 | `successCorroborated`; `declaredID != taskID` check; strict `counts.json` decode; `Eligible`; `GateWithSourcesAndDigests`; `memindex.RebuildFromDir` digest-less downgrade | A forged `outcome.status` is downgraded to `partial` because `successCorroborated` requires a non-failing quality-gate verdict and decision, at least one recorded validation result with every result passing (exit code 0 and a passing status), every boundary criterion met, and at least one required criterion with every required criterion met. A report substituted under another task's directory is rejected. A malformed `counts.json` is rejected. A success whose recorded verification failed is excluded as `verification contradicts success`. A record whose source is deleted, modified, or has no recorded digest is excluded (`source deleted`, `source modified`, `source unverified`). A success written before digests existed is rebuilt as `partial`. |
| 4 | `decisionmemory.Store.Applicable`; `internal/cli` `applicableDecisionItems` | A decision record cannot claim a verified outcome: it is never written as an outcome record, never passes through `Eligible`, and reaches the model only as generic-priority context text. It cannot override lifecycle or approval state, which live in `state.db`. Limit: anyone who can write `.agent-sdlc/context/decisions.json` can add a record stamped with the current repository identity, and it will be included as context (`GAP-6`, fixed: `memory add` and `memory remove` now refuse to overwrite a file they cannot round-trip). A record is retired with `sop memory remove --id <id>` (`GAP-7`, fixed). |
| 5 | None | Model selection is copied into `Environment` as advisory context. It cannot claim a verified outcome. |

## Provider-owned boundary

Provider-owned training is **outside direct SOP control**. This report does not mark
it secure, and it does not claim provider training, weights, or hosted behaviour are
benign or verified.

### Absence search — training, fine-tuning and weight loading

The absence of a training, fine-tuning or weight-loading path in SOP was established
by searching the code for the patterns below and inspecting the configuration surface
in the next table. No search found an SOP-owned training, fine-tuning, model-checkpoint
or weight-loading entry point. SOP's model code selects a provider and model and calls
it for inference.

Searches were run with `git grep -iE` over non-test Go files in `cmd/` and
`internal/`. The inspected tree is commit `87d68ad93682a71a3d222f4eba438cde1fe52ec4`
(`origin/main` `87d68ad9`), the verified baseline for this report:

| Pattern | Non-test matches | Result classification |
| --- | --- | --- |
| `fine.?tun` | 0 files | `NOT_APPLICABLE` — no fine-tuning entry point |
| `\blora\b` | 0 files | `NOT_APPLICABLE` — no adapter training |
| `\btrain(ing)?\b` | 0 files | `NOT_APPLICABLE` — no training entry point |
| `safetensors\|\.gguf\|from_pretrained` | 0 files | `NOT_APPLICABLE` — no weight file loading |
| `api/pull\|ollama pull\|api/create` | 0 matches | `NOT_APPLICABLE` — SOP does not pull or create models |
| `weights?` | 4 files | `NOT_APPLICABLE` — BM25 length weights, a "lightweight" header reader, and a comment stating that memory operations do not change model weights |
| `checkpoint` | 49 files | `NOT_APPLICABLE` — lifecycle, decision and evaluation-run checkpoints (for example `cmd/rdm004-eval`, `cmd/deceleval-advisory`); none saves or loads model state |
| `adapter` | — | `NOT_APPLICABLE` — `internal/decisionadapter` is a decision-provider adapter, not a model adapter |

### Configuration surface inspected

| File | Keys inspected | Training / adapter / weight setting present |
| --- | --- | --- |
| `.agent-sdlc/config.yaml` | Provider, model and routing configuration | No training, adapter-training or weight-source key |
| `internal/config` | Provider and model defaults | No training, adapter-training or weight-source key |
| `internal/provider` | Provider selection and call parameters | Inference-only; no weight source |
| `internal/ollamaagent` | Local model call path | Inference-only; no weight or training setting |
| `.env.example` | Environment surface for provider credentials and endpoints | No training, adapter-training or weight-source key |

Conclusion (`NOT_APPLICABLE`): SOP contains no training, fine-tuning, or model
weight-loading path. Model weights and training are owned by the provider. This is an
explicit designation, not a security claim.

## Recovery and invalidation evidence

This section records the supported remediation paths for poisoned or stale records and
the one operation that has no supported path. It adds no control. The CLI steps (Paths
A and D) ran in a disposable clone, not in the working tree, so they could not change
user-owned files or authoritative history. The `go test` steps (Paths B and C) build
their fixtures in test temporary directories.

These steps were run by Claude Code, not by SOP's agent: the agent's first draft of
this section described a disposable copy and command output it never produced, and
that draft was replaced with the runs below.

### Disposable copy setup

The copy is a scratch directory outside the repository (`<scratch>/poison003-copy`,
under the session's temporary directory on macOS arm64). The `sop` binary is a
scratch build of `origin/main` `87d68ad9`; nothing was installed.

| Step | Command | Exit code | Output excerpt |
| --- | --- | --- | --- |
| Create copy | `git clone -q --no-hardlinks <worktree> <scratch>/poison003-copy` | 0 | (quiet) |
| Pin commit | `git checkout -q 87d68ad93682a71a3d222f4eba438cde1fe52ec4` | 0 | `87d68ad9 Merge pull request #281 …` |

No cleanup command is recorded or shipped; the scratch directory is deleted by hand.

### Path A — Structural index reindex (`sop index`)

The supported invalidation path for the structural index is a rebuild. `sop index`
recomputes `Identity.Digest` from the current file contents and rewrites
`.agent-sdlc/context/index.json`.

Run inside the copy:

| Step | Command | Exit code | Output excerpt |
| --- | --- | --- | --- |
| Before | `sop index` | 0 | `Index identity: 7ad8f2f26e6dcded (schema 1, head 87d68ad9…, dirty=false)` |
| Change indexed content | `printf '\npoison003 probe\n' >> docs/README.md` | 0 | `git status`: ` M docs/README.md` |
| After | `sop index` | 0 | `Index identity: 756465cfafbe3002 (schema 1, head 87d68ad9…, dirty=true)` |

Result: `VERIFIED`. One changed document produced a different `Identity.Digest`
(`7ad8f2f26e6dcded` → `756465cfafbe3002`), and the recorded identity also flips
`dirty=true`. The command rewrites only the derived index artifact; source files and
history are untouched. Per-document rows still carry no digest of their own (`GAP-4`).

### Path B — Outcome reindex with the digest-aware source gate

The supported invalidation path for an ingested outcome record is a rebuild through
`internal/memindex` `RebuildFromDir`, which applies
`internal/outcomefile` `GateWithSourcesAndDigests`. A record whose source artifact was
deleted, modified, or could not be digested at ingest is excluded on the next rebuild.
The persisted run directory is not touched.

Before/after evidence is reproducible with the focused test, inside the copy:

| Step | Command | Exit code | Result |
| --- | --- | --- | --- |
| Before/after rebuild | `go test ./internal/memindex -run TestReindexProvenanceBeforeAndAfterRebuild -count=1 -v` | 0 | `PASS` |

What the test proves, step by step:

1. Two runs are ingested with `IngestRunFor` and written with `Write`. A rebuild
   reports `Indexed: 2`, `Excluded: 0`, and a search returns both records.
2. One source `report.json` is modified after ingestion. The rebuild reports
   `Indexed: 1`, `Excluded: 1`, and a search returns only the unmodified record. The
   exclusion reason is `source modified`.
3. One run directory is deleted. The rebuild reports `Indexed: 0`, `Excluded: 2`, and
   a search returns no record. The exclusion reason is `source deleted`.

The test asserts the `Indexed` / `Excluded` counts and the search results at each
step; it does not assert the exclusion reason strings (`source modified`,
`source deleted` come from `outcomefile.GateWithSourcesAndDigests`). The rebuild reads
the outcome documents in `outDir` and never writes to the run directories or the
outcome documents: excluding a record removes it from the advisory index only. (The
deletion in step 3 is the test's own simulated source loss, not something the gate
does.)

Related exclusions, same contract:

| Case | Test | Exclusion reason |
| --- | --- | --- |
| Source artifact unreadable at ingest | `TestReindexProvenanceUnreadableArtifactIsExcluded` | `source unverified` |
| Success written before digests existed | `TestReindexProvenanceLegacyDigestlessSuccessIsDowngraded` | record kept as advisory history with status `partial` |
| Mutable lifecycle `state.json` changes | `TestReindexProvenanceIgnoresMutableLifecycleState` | not evidence; record stays indexed |

### Path C — Success corroboration on ingest

A self-declared `outcome.status` cannot become a success record when the recorded
verification does not corroborate it.

| Step | Command | Exit code | Result |
| --- | --- | --- | --- |
| Corroboration | `go test ./internal/outcomefile -run TestIngestProvenance -count=1 -v` | 0 | `PASS` — `TestIngestProvenanceTamperedSuccessIsNotVerified` and its nine sub-cases all `PASS` |

`successCorroborated` (`internal/outcomefile/outcomefile.go`) downgrades an
uncorroborated success to `partial`. The covered cases include a failing validation
exit code, a failing validation status, no recorded validation, an unmet boundary
criterion, no required criterion, and a failed quality gate. This is the supported
correction path for a false success claim.

### Path D — Decision memory removal: explicit gap

Decision memory has **no supported removal or invalidation subcommand**. The
operator surface is the `memoryUsage` text in `internal/cli/memory.go`:

```
usage: sop memory enable|disable|status | sop memory add --decision <text> --reason <text> [--scope repository|project|architecture] [--source <id>] [--evidence <text>] | sop memory list [--scope <scope>]
```

The absence is shown by command output, not by assertion only. Inside the
disposable copy:

| Step | Command | Exit code | Output excerpt |
| --- | --- | --- | --- |
| Add a record | `sop memory add --decision probe --reason "poison003 removal probe"` | 0 | `Recorded decision (repository): probe` |
| Try the unsupported removal | `sop memory remove --decision probe` | 2 | `unknown memory command: remove` then the `memoryUsage` line |
| List (record still present) | `sop memory list` | 0 | `repository  probe`, `reason: poison003 removal probe`, `1 applicable decision(s)` |

Result: `GAP-7` — no supported removal command exists. Owning layer:
`internal/cli` (`runMemory`, `memoryUsage` in `internal/cli/memory.go`) and
`internal/decisionmemory` (`Store`, `Store.Add`, `Unmarshal`, `Marshal`). The only
surfaces are enable, disable, status, add and list. Invalidation is not achievable
through the CLI. The only effect that resembles invalidation is staleness: a record
recorded under a different `RepositoryIdentity(HEAD, dirty, "")` is listed as stale
and not applied. Removing the records file by hand is outside the supported contract
and is not recommended here. This report adds no destructive cleanup command and no
`remove` subcommand.

Follow-up fix: `GAP-7` is now fixed. `sop memory remove --id <id>` deletes exactly one
record through `decisionmemory.Store.Remove`, `sop memory list` prints each applicable
record's `id`, and an unknown id exits 1 with the file unchanged
(`TestMemoryRemoveByID` in `internal/cli/memory_test.go`). The evidence table above is
the pre-fix reproduction.

### Summary of recovery dispositions

| Path | Remediation | State |
| --- | --- | --- |
| Structural index | Rerun `sop index` after any content change | `VERIFIED` |
| Outcome record, source changed or deleted | Rerun `internal/memindex` `RebuildFromDir`; `GateWithSourcesAndDigests` excludes the record | `VERIFIED` |
| Outcome record, uncorroborated success | `successCorroborated` downgrades to `partial` on ingest | `VERIFIED` |
| Decision-memory record | `sop memory remove --id <id>` (id from `sop memory list`) | `VERIFIED` (GAP-7 fixed, `TestMemoryRemoveByID`) |

No supported path in this section deletes authoritative history. The paths rewrite a
derived index artifact or exclude a record from the advisory index. The persisted run
artifacts and the decision records file stay on disk.

## Provenance dimensions

Four provenance dimensions stay separate. One never implies another. Each row states
what the evidence proves and what it does not prove.

| Dimension | What it proves | What it does not prove | Evidence state |
| --- | --- | --- | --- |
| Provider model name | The provider and model identifiers that the configuration selected, copied verbatim into the outcome record `Environment` (`model_selection` in `report.json`; `internal/outcomefile` `IngestRunFor`) | That the provider serves the same weights, that the model revision is unchanged, or that the model is untampered. Model revision and weight provenance are not recorded and cannot be verified from this repository | `IMPLEMENTED` (copied); weight and revision provenance `GAP` |
| Content checksum | That the bytes of `report.json` and the recorded source artifacts match the recorded SHA-256 values (`SourceHash`, `SourceDigests`, document `ContentHash`) and that the index identity matches its file set (`Identity.Digest`). GAP-2 exclusions (`source deleted`, `source modified`, `source unverified`) are `VERIFIED` | That the content is benign, correct, or free of poisoned text. Byte integrity does not prove benign semantics. An individual indexed document row carries no per-file digest (`GAP-4`, open) | `VERIFIED` for outcome artifacts; `GAP-4` open for per-document rows |
| Publisher authenticity | Nothing established in this scope. SOP does not verify who authored, published, or served retrieved or provided content | That a document came from a trusted author, or that a retrieval server is the one the operator intended. A `document_hash` reported by the retrieval server is not recomputed by SOP (`GAP-5`, open) | `GAP` (not established) |
| Semantic trust | Nothing established in this scope. Eligibility shows that a record passed the ingest and reindex gates, not that its claim is true | That evidence supports a claim, that a conclusion is correct, or that retrieved content is relevant and unpoisoned | `GAP` (not established) |

Reading across the columns is not allowed. A matching checksum does not raise
publisher authenticity. A correct-looking model name does not raise semantic trust.
A record that passes `Eligible` is eligible evidence, not a true statement.

### Plan 09 handoff — retrieval query isolation

Retrieval query isolation is out of scope for POISON-003. It is handed to Plan 09:
`docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`. This report claims
no query-isolation coverage. It records no evidence for query isolation and adds no
control that would implement it. The handoff covers isolation between queries,
corpora and callers during retrieval, and any poisoning boundary that isolation
would provide.

### Provenance limits for the PR body

The following paragraph is the PR-body summary for this stage. It matches the table
above.

> Provenance is recorded in four separate dimensions: provider model name, content
> checksum, publisher authenticity, and semantic trust. Provider model names are
> copied from configuration; they do not prove model weights or revision. Content
> checksums (`SourceHash`, `SourceDigests`, `Identity.Digest`) prove byte integrity
> only, and byte integrity does not prove benign semantics; GAP-4 (no per-document
> digest on indexed rows) and GAP-5 (retrieval `document_hash` is server-reported and
> not recomputed) stay open. Publisher authenticity and semantic trust are not
> established in this scope. Retrieval query isolation is handed to Plan 09
> (`docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`); this work
> claims no query-isolation coverage. POISON-003 changes no production code; it adds
> recovery evidence, this provenance table, and the Plan 09 handoff.

## Confirmed gaps and affected contracts

| ID | Affected contract (package, function) | Disposition | Reproduction |
| --- | --- | --- | --- |
| GAP-1 | `internal/outcomefile` `IngestRunFor` — a self-declared `outcome.status` could map to a success record without corroboration | **Fixed** (`successCorroborated`, required/boundary criteria and validation checks, declared-id check, strict `counts.json` decode) | `go test ./internal/outcomefile -run TestIngestProvenance -count=1` |
| GAP-2 | `internal/outcomefile` `GateWithSourcesAndDigests` — a changed source retained an obsolete verified identity | **Fixed** (`source modified`, `source unverified`, `source deleted` exclusion reasons; use in `internal/memindex` `RebuildFromDir`) | `go test ./internal/memindex -run TestReindexProvenance -count=1` |
| GAP-3 | `internal/decisionmemory`, `internal/cli` `applicableDecisionItems` — whether a decision record can claim a verified outcome or override authoritative state | **Closed by trace, no change** — it cannot (see §Verified-override analysis, path 4). `decisions.json` has no integrity check, so it is trusted only as advisory context | Code trace above |
| GAP-4 | `internal/repoindex` `File` / `Document` — indexed document rows carry no per-file content digest, so a single document row is not independently digest-verified | **Open** | `go test ./internal/repoindex -count=1` (index identity only) |
| GAP-5 | `internal/ragclient` `Evidence.DocumentHash` — the per-item `document_hash` is reported by the retrieval server and is not recomputed or verified by SOP against the returned text | **Open** (trust limit) — `document_hash` covers the full source document while SOP receives only a chunk, so it cannot be recomputed; deterministic fixtures pin the current non-verification and the additive proposal is [PROPOSAL-RAG-EVIDENCE-HASH.md](../../plans/PROPOSAL-RAG-EVIDENCE-HASH.md) | `go test ./internal/ragclient -run 'TestDocumentHashIsServerReportedAndNotRecomputed|TestDocumentHashMayBeEmpty' -count=1` |
| GAP-6 | `internal/cli` `loadDecisionStore` / `saveDecisionStore` — an unreadable `decisions.json` (malformed JSON or an unknown schema) loads as an empty store, and the next `sop memory add` overwrites the file, so the earlier records are lost without a warning | **Fixed** (`loadDecisionStoreForWrite`, via `decisionmemory.UnmarshalStrict`, refuses a malformed, unknown-schema or partly invalid file, exits 1 and leaves the bytes unchanged; reading for context still applies nothing and never writes) | `go test ./internal/cli -run TestMemoryAddRefusesUnreadableStore -count=1`. Pre-fix reproduction: in a disposable git repo, write `{"schema":"unknown-future","records":[{"decision":"keep me"}]}` to `.agent-sdlc/context/decisions.json` and run `sop memory add --decision new --reason r`; before the fix it exited 0 and the file held only the new record |
| GAP-7 | `internal/cli` `runMemory` / `memoryUsage`, `internal/decisionmemory` `Store` — decision memory has no supported removal or invalidation command, so a poisoned decision record cannot be retired through the CLI | **Fixed** (`sop memory remove --id <id>` via `Store.Remove`; `sop memory list` prints ids; an unknown id exits 1 with the file unchanged) | `go test ./internal/cli -run TestMemoryRemoveByID -count=1`. Pre-fix reproduction: §Recovery and invalidation evidence, Path D |

## Disposition

- GAP-1 and GAP-2 are **fixed** under the POISON-002 repair. The affected contracts
  are named above, and the focused fixtures exist in
  `internal/outcomefile/provenance_test.go` and
  `internal/memindex/provenance_test.go`. Both fixtures pass on the current tree
  (see §Verification).
- GAP-3 is **closed by trace**: a decision-memory record cannot claim a verified
  outcome or override authoritative state. No code changed.
- GAP-4 and GAP-5 are **open** trust limits. They are recorded here rather than
  fixed, because each would need a new contract or new policy. GAP-5 now has deterministic fixtures pinning the current
  non-verification, and its contract proposal is linked in the gap table above.
- GAP-6 is **fixed** in a follow-up change. `sop memory add` (and `sop memory
  remove`) refuse to overwrite a `decisions.json` that is malformed, has another
  schema, or holds records that fail validation; they exit 1 and the file bytes are
  unchanged (`TestMemoryAddRefusesUnreadableStore`).
- GAP-7 is **fixed** in a follow-up change. `sop memory remove --id <id>` retires
  exactly one record, `sop memory list` prints each record's id, and an unknown id
  fails with the file unchanged (`TestMemoryRemoveByID`).
- Provider-owned training, fine-tuning and weight loading remain outside direct SOP
  control. They have no disposition of "fixed" here.

## Residual trust limits

The following dimensions stay separate. One does not imply another. See §Provenance
dimensions for the evidence behind each row.

| Dimension | What it proves | What it does not prove |
| --- | --- | --- |
| Provider model name | Which model identifier the configuration selected | That the provider serves the same weights, or that the model is untampered |
| Content checksum | That bytes match a recorded reference | That the content is benign, correct, or free of poisoned text |
| Publisher authenticity | Not established for retrieved or provided content in this scope | That a document came from a trusted author |
| Semantic trust | Not established in this scope | That evidence supports a claim, or that a conclusion is correct |

Integrity proves that bytes match a reference. Integrity does **not** prove benign
semantics. A record that passes the eligibility gate is eligible evidence, not a
true statement.

Further limits:

- Retrieval query isolation is not covered here. It is handed to Plan 09
  (`docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`).
- Decision memory has no integrity check on `decisions.json`; it is trusted only as
  advisory context. Removal (`GAP-7`) and the overwrite refusal (`GAP-6`) are fixed.
- The provider owns model training and weight provenance. SOP cannot verify either
  from this repository.

## Verification

Inspected commit: `87d68ad93682a71a3d222f4eba438cde1fe52ec4` (`origin/main` `87d68ad9`).
POISON-003's CLI steps (Paths A and D) ran in a disposable clone pinned to `87d68ad9`
with a scratch `sop` build of the same commit. Platform: macOS, arm64.

Evidence states: `IMPLEMENTED` and `GAP` are read from code. `VERIFIED` requires a
passing command below. `NOT_RUN` marks evidence this report does not supply.

| Item | State | Evidence path |
| --- | --- | --- |
| Outcome ingest corroboration and digest-aware gate | `IMPLEMENTED` | `internal/outcomefile/outcomefile.go` (`IngestRunFor`, `successCorroborated`, `GateWithSourcesAndDigests`) |
| Digest-aware rebuild gate | `IMPLEMENTED` | `internal/memindex/rebuild.go` (`RebuildFromDir`) |
| Structural repository index identity digest | `IMPLEMENTED` | `internal/repoindex/index.go` (`Build`, `Identity.Digest`) |
| Structural index reindex before/after | `VERIFIED` | §Recovery and invalidation evidence, Path A |
| Outcome reindex invalidation before/after | `VERIFIED` | `internal/memindex/provenance_test.go` (`TestReindexProvenanceBeforeAndAfterRebuild`) |
| Success corroboration on ingest | `VERIFIED` | `internal/outcomefile/provenance_test.go` (`TestIngestProvenance*`) |
| Decision-memory removal | `VERIFIED` (GAP-7 fixed) | `internal/cli/memory.go` (`runMemoryRemove`), `internal/cli/memory_test.go` (`TestMemoryRemoveByID`) |
| External retrieval client | `IMPLEMENTED` | `internal/ragclient/ragclient.go` |
| Retrieval query isolation | `NOT_APPLICABLE` here | Handed to Plan 09 (`docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`) |
| Decision-memory trace | `IMPLEMENTED` (read from code); overwrite defect `GAP-6` reproduced with the scratch `sop` build of `origin/main` `87d68ad9`, macOS arm64, and fixed (`loadDecisionStoreForWrite` → `decisionmemory.UnmarshalStrict`, `TestMemoryAddRefusesUnreadableStore`) | `internal/decisionmemory/decisionmemory.go`, `internal/cli/memory.go`, `internal/cli/run.go` |
| Training / fine-tuning / weight loading | `NOT_APPLICABLE` | Absence search above |

Focused commands for the fixed and recovery contracts:

| Command | Exit code | Result |
| --- | --- | --- |
| `go test ./internal/outcomefile -run TestIngestProvenance -count=1` | 0 | PASS — the selected `TestIngestProvenance*` tests ran and passed, including `TestIngestProvenanceTamperedSuccessIsNotVerified`, `TestIngestProvenanceMalformedCountsRejected` and `TestIngestProvenanceModifiedSourceInvalidated` |
| `go test ./internal/memindex -run TestReindexProvenance -count=1` | 0 | PASS — all four `TestReindexProvenance*` tests ran and passed |
| `go test ./internal/repoindex -count=1` | 0 | PASS — index identity tests passed |
| `go test ./internal/cli -run 'TestMemoryAddRefusesUnreadableStore\|TestMemoryRemoveByID' -count=1` | 0 | PASS — GAP-6 overwrite refused with bytes unchanged (unknown schema, malformed JSON, invalid record); GAP-7 removal by id and unknown id rejected |

Required checks for POISON-003:

| Command | Exit code | Result |
| --- | --- | --- |
| `go build ./...` | 0 | PASS — no build errors |
| `go test ./...` | 0 | PASS — every package with tests reported `ok`; packages without tests reported `[no test files]` |
| `go vet ./...` | 0 | PASS — no vet findings |

This report does not claim a live-model run. POISON-003 changes no production code.
The only file this task writes is this report.
