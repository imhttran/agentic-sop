# Plan 05 — Data and model poisoning (LLM05): owned poisoning boundaries

Status: report-only trace. This document records what SOP owns, what an external
provider owns, and which confirmed gaps were fixed at the current baseline. It adds
no classifier, trust service, poisoning-detection claim, or runtime control.

Task: `POISON-001` (`docs/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`).
Dependency: `POISON-002` (outcome-ingestion integrity repair).

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
| 4 | `decisionmemory.Store.Applicable`; `internal/cli` `applicableDecisionItems` | A decision record cannot claim a verified outcome: it is never written as an outcome record, never passes through `Eligible`, and reaches the model only as generic-priority context text. It cannot override lifecycle or approval state, which live in `state.db`. Limit: anyone who can write `.agent-sdlc/context/decisions.json` can add a record stamped with the current repository identity, and it will be included as context (`GAP-6` covers the related overwrite behaviour). |
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

## Confirmed gaps and affected contracts

| ID | Affected contract (package, function) | Disposition | Reproduction |
| --- | --- | --- | --- |
| GAP-1 | `internal/outcomefile` `IngestRunFor` — a self-declared `outcome.status` could map to a success record without corroboration | **Fixed** (`successCorroborated`, required/boundary criteria and validation checks, declared-id check, strict `counts.json` decode) | `go test ./internal/outcomefile -run TestIngestProvenance -count=1` |
| GAP-2 | `internal/outcomefile` `GateWithSourcesAndDigests` — a changed source retained an obsolete verified identity | **Fixed** (`source modified`, `source unverified`, `source deleted` exclusion reasons; use in `internal/memindex` `RebuildFromDir`) | `go test ./internal/memindex -run TestReindexProvenance -count=1` |
| GAP-3 | `internal/decisionmemory`, `internal/cli` `applicableDecisionItems` — whether a decision record can claim a verified outcome or override authoritative state | **Closed by trace, no change** — it cannot (see §Verified-override analysis, path 4). `decisions.json` has no integrity check, so it is trusted only as advisory context | Code trace above |
| GAP-4 | `internal/repoindex` `File` / `Document` — indexed document rows carry no per-file content digest, so a single document row is not independently digest-verified | **Open** | `go test ./internal/repoindex -count=1` (index identity only) |
| GAP-5 | `internal/ragclient` `Evidence.DocumentHash` — the per-item `document_hash` is reported by the retrieval server and is not recomputed or verified by SOP against the returned text | **Open** (trust limit) | No reproducing test or command exists yet |
| GAP-6 | `internal/cli` `loadDecisionStore` / `saveDecisionStore` — an unreadable `decisions.json` (malformed JSON or an unknown schema) loads as an empty store, and the next `sop memory add` overwrites the file, so the earlier records are lost without a warning | **Open** | In a disposable git repo: write `{"schema":"unknown-future","records":[{"decision":"keep me"}]}` to `.agent-sdlc/context/decisions.json`, run `sop memory add --decision new --reason r`; the command exits 0 and the file then holds only the new record |

## Disposition

- GAP-1 and GAP-2 are **fixed** under the POISON-002 repair. The affected contracts
  are named above, and the focused fixtures exist in
  `internal/outcomefile/provenance_test.go` and
  `internal/memindex/provenance_test.go`. Both fixtures pass on the current tree
  (see §Verification).
- GAP-3 is **closed by trace**: a decision-memory record cannot claim a verified
  outcome or override authoritative state. No code changed.
- GAP-4 and GAP-5 are **open** trust limits. They are recorded here rather than
  fixed, because each would need a new contract or new policy. GAP-5 states
  explicitly that no reproducing test or command exists yet.
- GAP-6 is **open** and reproducible. It is a defect in an existing contract and is
  proposed as a follow-up change: refuse to overwrite an unreadable
  `decisions.json` instead of replacing it. This report-only task does not change
  code. No new control is added by this report.
- Provider-owned training, fine-tuning and weight loading remain outside direct SOP
  control. They have no disposition of "fixed" here.

## Residual trust limits

The following dimensions stay separate. One does not imply another.

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

- Retrieval query isolation is not covered here. It is handed to Plan 09.
- Decision memory has no integrity check on `decisions.json`; it is trusted only as
  advisory context, and an unreadable file is silently replaced (`GAP-6`).
- The provider owns model training and weight provenance. SOP cannot verify either
  from this repository.

## Verification

Inspected commit: `87d68ad93682a71a3d222f4eba438cde1fe52ec4` (`origin/main` `87d68ad9`).


Evidence states: `IMPLEMENTED` and `GAP` are read from code. `VERIFIED` requires a
passing command below. `NOT_RUN` marks evidence this report does not supply.

| Item | State | Evidence path |
| --- | --- | --- |
| Outcome ingest corroboration and digest-aware gate | `IMPLEMENTED` | `internal/outcomefile/outcomefile.go` (`IngestRunFor`, `successCorroborated`, `GateWithSourcesAndDigests`) |
| Digest-aware rebuild gate | `IMPLEMENTED` | `internal/memindex/rebuild.go` (`RebuildFromDir`) |
| Structural repository index identity digest | `IMPLEMENTED` | `internal/repoindex/index.go` (`Build`, `Identity.Digest`) |
| External retrieval client | `IMPLEMENTED` | `internal/ragclient/ragclient.go` |
| Decision-memory trace | `IMPLEMENTED` (read from code); overwrite defect `GAP-6` reproduced with the scratch `sop` build of `origin/main` `87d68ad9`, macOS arm64 | `internal/decisionmemory/decisionmemory.go`, `internal/cli/memory.go`, `internal/cli/run.go` |
| Training / fine-tuning / weight loading | `NOT_APPLICABLE` | Absence search above |

Focused commands for the fixed contracts:

| Command | Exit code | Result |
| --- | --- | --- |
| `go test ./internal/outcomefile -run TestIngestProvenance -count=1` | 0 | PASS — the selected `TestIngestProvenance*` tests ran and passed, including `TestIngestProvenanceTamperedSuccessIsNotVerified`, `TestIngestProvenanceMalformedCountsRejected` and `TestIngestProvenanceModifiedSourceInvalidated` |
| `go test ./internal/memindex -run TestReindexProvenance -count=1` | 0 | PASS — all four `TestReindexProvenance*` tests ran and passed |

Repository-required checks:

| Command | Exit code | Result |
| --- | --- | --- |
| `go build ./...` | 0 | PASS — no build errors |
| `go test ./...` | 0 | PASS — every package with tests reported `ok`; packages without tests reported `[no test files]` |
| `go vet ./...` | 0 | PASS — no vet findings |

This report does not claim a live-model run. POISON-001 changes no production code;
the only file this task writes is this report.
