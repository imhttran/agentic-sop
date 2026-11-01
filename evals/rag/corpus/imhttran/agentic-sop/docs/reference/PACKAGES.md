# Package Map

**Type:** Descriptive reference (code map)

Every Go package under `internal/` and every binary under `cmd/`, grouped by the
job it does, with its main entry points and the doc that describes it in depth.
Use it to find which package owns a behavior before you open the code. The
component view is [architecture/COMPONENTS.md](../architecture/COMPONENTS.md);
the ownership boundary is [architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md).

Generated from the package doc comments and import graph at commit `cb86f27d`.
"Used by" lists non-test importers inside this module. A package marked
**not wired** has no non-test importer at that commit: it is built and tested,
but no production path calls it yet.

## Contents

1. [Binaries](#1-binaries)
2. [Lifecycle and state](#2-lifecycle-and-state)
3. [Planning](#3-planning)
4. [Agents, providers, and routing](#4-agents-providers-and-routing)
5. [Context, prompts, and retrieval](#5-context-prompts-and-retrieval)
6. [Validation, review, and gates](#6-validation-review-and-gates)
7. [Approvals, questions, and recovery](#7-approvals-questions-and-recovery)
8. [Git, GitHub, and delivery](#8-git-github-and-delivery)
9. [Decision layer and JEV](#9-decision-layer-and-jev)
10. [Observability and notifications](#10-observability-and-notifications)
11. [Command surfaces and configuration](#11-command-surfaces-and-configuration)
12. [Evaluation harnesses](#12-evaluation-harnesses)
13. [Test-only packages](#13-test-only-packages)

---

## 1. Binaries

| Binary | What it does | See |
| --- | --- | --- |
| `cmd/sop` | The `sop` CLI. A thin `main` over `internal/cli`. | [CLI.md](CLI.md) |
| `cmd/sop-ollama-agent` | The bootstrap coding agent. SOP's command provider sends it a JSON request on stdin; it runs a local Ollama model with the controlled repository tools from `internal/ollamaagent` and returns the result on stdout. | [specs/PROVIDERS.md](../specs/PROVIDERS.md) |
| `cmd/sop-controller` | Legacy wrapper for the dashboard. Same code as `sop controller serve`. | [guides/SOP-CONTROLLER-DASHBOARD.md](../guides/SOP-CONTROLLER-DASHBOARD.md) |
| `cmd/sop-decision-adapter` | Legacy wrapper for the decision adapter. Same code as `sop decision`. | [architecture/JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) |
| `cmd/sop-memory` | Ingests completed runs into outcome files, rebuilds the advisory memory index, and runs the advisory retrieval path for a task or failure. Calls no model. | [guides/DECISION-MEMORY-USAGE.md](../guides/DECISION-MEMORY-USAGE.md) |
| `cmd/deceleval-baseline` | Decision-model evaluation, arm A: deterministic classification only. | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `cmd/deceleval-advisory` | Decision-model evaluation, arm B: runs the real advisory checkpoint and writes the paired A/B artifact. | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `cmd/deceleval-jev` | JEV shadow-mode evaluation over the frozen 61-scenario corpus. JEV is observed, never consulted. | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `cmd/deceleval-recovery` | DME-005 experiment: asks the advisory adapter a recovery question (AUTO_FIX, CONTINUE or HUMAN) and scores it against frozen labels. | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `cmd/rdm-pilot-corpus` | Builds the planning-only decision-memory corpus from recorded run artifacts. | [reports/rdm-decision-memory/](../reports/rdm-decision-memory/) |
| `cmd/rdm004-eval` | Runs the frozen RDM-004 A/B comparison with call, retry, time and token limits. Spends model calls only with `--authorize`. | [reports/rdm-decision-memory/](../reports/rdm-decision-memory/) |

## 2. Lifecycle and state

The single lifecycle authority and the stores it writes to.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `domain` | The task lifecycle: `Task`, `TaskStatus`, legal transitions, dependencies, attempts, retry policy, blocked reasons, approval values. The only place states and transitions are defined. | `Task`, `TaskStatus`, `CanTransitionTo`, `IsRunnable`, `DefaultRetryPolicy` | most packages | [specs/TASK-LIFECYCLE.md](../specs/TASK-LIFECYCLE.md) |
| `store` | SQLite persistence (`.agent-sdlc/state.db`) for the task graph, handoff capsules, required evidence and the notification outbox. | `Open`, `Get`, `List`, `EnqueueNotification`, `ClaimNotificationDeliveries` | `cli`, `approval` | [COMPONENTS.md § State Store](../architecture/COMPONENTS.md#state-store) |
| `run` | The durable record of one run under `.agent-sdlc/runs/<id>/`: stage, attempts, checkpoints, approval history, JEV and model-selection artifacts. A run that ends for any reason stays inspectable. | `New`, `Open`, `Load`, `PersistDurable`, `ReadAttemptRecords` | `cli`, `planflow` | [COMPONENTS.md § Run State and Report](../architecture/COMPONENTS.md#run-state-and-report) |
| `scheduler` | Picks at most one legally runnable task. In-flight work holds the single slot; otherwise the next PLANNED task with satisfied dependencies. | `Next`, `HasRunnable`, `IsActive` | `cli`, `completion` | [architecture/orchestration.md](../architecture/orchestration.md) |
| `projectlock` | Serializes writers of durable state, because the JSON artifacts and `state.db` can't share a transaction. | `Acquire`, `AcquireWait`, `Release` | `cli`, `planflow` | [specs/PROJECT-STATE-LOCK.md](../specs/PROJECT-STATE-LOCK.md) |
| `durablefs` | Crash-safe small-file write: temp file, fsync, atomic rename, fsync the directory. | `WriteFile` | `cli`, `question`, `run`, `runmetrics` | — |
| `resume` | Reconciles a task's stored status with the branch and PR that actually exist after an interruption, and reports the next legal action without duplicating either. | `Resolve`, `ResolveActive`, `ActionFor` | `cli` | [COMPONENTS.md § Resume](../architecture/COMPONENTS.md#resume) |
| `continuation` | Read-only gate before another `sop run` on an active plan: compares the source plan, the machine plan and the stored lifecycle, and says whether continuing is safe and why not. | `Check`, `NextAction` | `cli` | [STATUS-AND-RECOVERY.md](STATUS-AND-RECOVERY.md) |
| `workitem` | One input shape for both planned tasks and ad-hoc prompts, so both go through the same governed execution. Carries no policy. | `FromTask`, `FromPrompt` | `cli` | [specs/WORK-ITEMS.md](../specs/WORK-ITEMS.md) |

## 3. Planning

From a PRD or plan file to persisted tasks.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `planflow` | Finds the human plan source, keeps `.agent-sdlc/plan.json` in step with it, reconciles changes, historicalizes finished plans, and creates tasks when none exist. Safe to repeat. | `Prepare`, `Reconcile`, `Inspect`, `PreflightPlan`, `Historicalize` | `cli`, `continuation` | [specs/PLAN-HISTORICALIZATION.md](../specs/PLAN-HISTORICALIZATION.md) |
| `planner` | Turns a PRD into a validated `Plan` by asking an agent; also parses and renders plan Markdown and lints it. No file or workflow I/O. | `Generate`, `PlanFromMarkdown`, `RenderMarkdown`, `Validate`, `Lint` | `cli`, `planflow`, `taskbuilder`, `bootstrap`, `continuation` | [COMPONENTS.md § Planner](../architecture/COMPONENTS.md#planner) |
| `taskbuilder` | Converts a validated plan into `domain.Task`s, checks the dependency graph is a DAG, and persists them atomically. | `CreateTasksFromPlan`, `ValidateDAG` | `cli`, `planflow` | [COMPONENTS.md § Plan Preparation](../architecture/COMPONENTS.md#plan-preparation) |
| `taskfile` | Loads one task from a hand-written Markdown file; recognized sections are extracted and the rest becomes the description. | `Load`, `Parse`, `Render` | `cli`, `workitem`, `cmd/sop-memory` | [COMPONENTS.md § Task Loader](../architecture/COMPONENTS.md#task-loader) |
| `bootstrap` | Makes every task depend on the plan's environment stage, so feature work waits for the dev environment. | `Apply` | `planflow`, `taskbuilder` | [COMPONENTS.md § Environment Bootstrap](../architecture/COMPONENTS.md#environment-bootstrap) |

## 4. Agents, providers, and routing

Who does the work, on which model, under which limits.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `agent` | The provider-neutral boundary to an LLM: the `Agent` interface, capabilities, harness selection, and the command agent that runs an external process. Agents never touch files or workflow state directly. | `Agent`, `Harness`, `FromConfig`, `NewCommandAgent`, `Capabilities` | many | [specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) |
| `toolharness` | The only place a model can read and edit repository files, run policy-checked commands and inspect Git. Never commits, pushes or touches `state.db`; audits every call. | `New`, `Run`, `CheckCommand`, `NewAuditLog` | `cli`, `ollamaagent` | [specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) |
| `ollamaagent` | The built-in coding agent behind `sop-ollama-agent`: chats with a local or cloud Ollama model and drives `toolharness`. Configuration selects it in-process with `harness: tool` and `provider: ollama`, which is tran's setup. | `New`, `Run`, `ConfigFromEnv`, `NewNativeAgent` | `cli`, `jev`, `cmd/sop-ollama-agent` | [specs/PROVIDERS.md](../specs/PROVIDERS.md) |
| `agentbin` | Finds the installed `sop-ollama-agent` binary, using the same order as the bootstrap script. | `Resolve`, `BinaryPath` | `ollamaagent` | — |
| `commandpolicy` | Classifies a command (as an argument list, never a shell string) as SAFE, REQUIRES_APPROVAL or DENIED. Project policy can tighten but never loosen. | `Classify`, `CargoClassify` | `cli`, `toolharness`, `evaltarget` | [COMPONENTS.md § Command Policy](../architecture/COMPONENTS.md#command-policy) |
| `budget` | The execution limits an agent runs under. The harness sets and enforces them; the model never sees or extends them. | `Resolve`, `Defaults`, `Validate` | `cli`, `ollamaagent`, `planner` | [specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) |
| `provider` | Reports which model server is configured, whether it's reachable, which models it serves and what they can do. Doesn't choose a model class. | `NewRegistry`, `Registry`, `ResolveAvailability` | `agent`, `cli`, `config` | [architecture/provider-runtime.md](../architecture/provider-runtime.md) |
| `provider/ollama`, `provider/openai`, `provider/llamacpp`, `provider/mlx`, `provider/command` | Provider adapters. `llamacpp` and `mlx` share the OpenAI-compatible code in `openai`. | — | `cli` via registry | [specs/PROVIDERS.md](../specs/PROVIDERS.md) |
| `provider/httpx` | Size-capped HTTP GET helpers shared by the adapters. | — | provider adapters | [architecture/provider-runtime.md](../architecture/provider-runtime.md) |
| `model` | Resolves which model and provider a run uses from a small, medium or large class. Precedence: `--model-class`, `SOP_MODEL_*`, the `models:` block, built-in defaults. Off unless configured. | `Resolve`, `DefaultRoute` | many | [specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) |
| `router` | Maps task structure and JEV evidence to a model class, deterministically. | `Decide`, `SignalsFrom` | `cli`, `verify` | [architecture/model-routing.md](../architecture/model-routing.md) |
| `adaptiveroute` | Moves routing toward the smallest model class that has succeeded at a capability, based on recorded outcomes. An operator override always wins. | `Route` | `cli`, `orchestration` | [specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) |
| `verify` | Assigns a verification risk tier (LOW, MEDIUM, HIGH) that decides which checks run and whether JEV is required. Escalate-only. | `Derive`, `RequiresJEV` | `cli` | [specs/VALIDATION.md](../specs/VALIDATION.md) |
| `orchestration` | Domain model for splitting a task across workers (orchestrator, workers, integrator, reviewer) and the SINGLE, SEQUENTIAL or PARALLEL decision. Reads `domain`; never defines states. | `Decide`, `Assign`, `BoundedIntegrate`, `Coordinator` | `cli`, `orchadopt` | [architecture/orchestration.md](../architecture/orchestration.md) |
| `parallel` | Runs independent tasks concurrently, each in its own Git worktree, up to a maximum. **Not wired.** | `New`, `Run`, `Select` | — | [COMPONENTS.md § Parallelism](../architecture/COMPONENTS.md#parallelism) |
| `normalize` | Cleans provider output safely (whitespace, one code fence, empty response). Malformed JSON fails rather than being guessed at. | `Response`, `JSON` | `agent`, `planner`, `review` | — |
| `writing` | SOP's one shared writing rule, inserted into model prompts and used for status output. | `PromptBlock` | `agent`, `ollamaagent` | — |

## 5. Context, prompts, and retrieval

What evidence the model sees, and where it comes from. The integration story is
[architecture/RAG-RETRIEVAL-INTEGRATION.md](../architecture/RAG-RETRIEVAL-INTEGRATION.md).

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `context` | The Context Engine: selects, orders, labels and truncates the evidence for one execution, measured in bytes, lines and items, never tokens. Also turns RAG evidence into low-priority, untrusted items. | `Build`, `Render`, `KnowledgeItems`, `DefaultLimits` | `cli`, `orchestration`, `prompt` | [specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) |
| `prompt` | The Prompt Compiler: turns a `Context`, the task and the previous attempt into a bounded, byte-identical model request. No retrieval, no model calls. | `Compile`, `Render`, `LimitsFor` | `cli`, `prompttuning` | [specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) |
| `promptcache` | Caches responses from read-only capabilities only. IMPLEMENT and FIX are never cached, and a hit never counts as success. | `New`, `Lookup`, `Eligible` | `cli` | [EVIDENCE-REUSE.md](EVIDENCE-REUSE.md) |
| `repoindex` | Structural index of the repository (modules, packages, symbols, imports, doc authority). No source bodies, no model. | `Build` | `cli`, `retrieval` | [architecture/RAG-RETRIEVAL-INTEGRATION.md](../architecture/RAG-RETRIEVAL-INTEGRATION.md) |
| `retrieval` | Deterministic BM25 ranker over the repository index. This is the built-in code retrieval every run uses. | `New`, `Search`, `CandidatesFromIndex` | `cli`, `retrievalboundary`, `retrievalgate`, `vectoreval` | [architecture/RAG-RETRIEVAL-INTEGRATION.md](../architecture/RAG-RETRIEVAL-INTEGRATION.md) |
| `ragclient` | Client for the external `rag-retrieval/v1` producer (for example rag-template-rust's `cmd/retrieval`). Runs the configured executable, sends one bounded request, validates the response. Used only when `rag.enabled` is true. | `New`, `Client.Retrieve` | `cli`, `context` | [specs/RAG-RETRIEVAL-CONTRACT.md](../specs/RAG-RETRIEVAL-CONTRACT.md), [specs/RAG-CONFIGURATION.md](../specs/RAG-CONFIGURATION.md) |
| `retrievalboundary` | Backend-neutral bounded retrieval: scoped query, hard count, time and token limits, typed errors that separate "optional, fall back" from "required, block". | `Retrieve`, `ScopedQuery`, `ApplyBounds` | `cli`, `memindex`, `outcomefile`, `dqharness`, `cmd/sop-memory` | [guides/DECISION-MEMORY-USAGE.md](../guides/DECISION-MEMORY-USAGE.md) |
| `memindex` | The advisory memory index: a rebuildable SQLite FTS5 database, separate from `state.db`, filtered by corpus and eligibility before ranking. Backs the `advisory_retrieval:` block. | `Open`, `Rebuild`, `RebuildFromDir`, `Search`, `DetectDrift`, `LocalRetriever` | `cli`, `dqharness`, `cmd/sop-memory` | [guides/DECISION-MEMORY-USAGE.md](../guides/DECISION-MEMORY-USAGE.md) |
| `outcomefile` | Versioned, hashed outcome documents (one per task attempt) that feed `memindex`, plus the eligibility gate. | `Write`, `Read`, `ReadAll`, `Gate`, `IngestRun` | `cli`, `memindex`, `cmd/sop-memory` | [guides/DECISION-MEMORY-USAGE.md](../guides/DECISION-MEMORY-USAGE.md) |
| `decisionmemory` | Bounded store of engineering decisions, keyed to the repository state they were made under; stale records are reported, never applied. | `New`, `Add`, `Applicable` | `cli` | — |
| `handoff` | Builds a small capsule after a task completes so later tasks can reuse what it established without re-reading its history. Derived, never authoritative. | `NewBuilder`, `Build`, `Select`, `Compress` | `cli`, `store` | [specs/EXECUTION.md](../specs/EXECUTION.md) |
| `handoff/caveman` | Optional compressor that delegates to a configured command. | — | `handoff` | — |

## 6. Validation, review, and gates

Deciding whether a change is good enough, without asking a model.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `testrunner` | Runs the project's configured verification commands and returns structured results. Never builds commands from untrusted input. | `New`, `Run`, `RunAll` | `cli`, `validate`, `review`, `ci` | [specs/VALIDATION.md](../specs/VALIDATION.md) |
| `validate` | The validation stage: runs checks in a fixed order and classifies PostgreSQL integration commands. | `Run`, `Checks`, `PostgresChecks` | `cli` | [specs/VALIDATION.md](../specs/VALIDATION.md), [specs/RAG-POSTGRES-VALIDATION-POLICY.md](../specs/RAG-POSTGRES-VALIDATION-POLICY.md) |
| `verifcache` | Reuses a verification result only when revision, inputs, command, environment, policy, scope and verifier all match exactly. | `New`, `Lookup`, `Decide` | `cli` | [EVIDENCE-REUSE.md](EVIDENCE-REUSE.md) |
| `review` | Structured pre-CI review: the agent returns findings, SOP decides whether they block. Includes the fix loop. | `Run`, `NewLoop`, `NewAgentProvider`, `ProviderFromEnv` | `cli`, `jev`, `quality` | [specs/REVIEW.md](../specs/REVIEW.md) |
| `quality` | The quality gate: PASS, FAIL or NEEDS_HUMAN from verification, unresolved findings, fix budget and human flags. JEV and security findings are extra evidence only. | `Evaluate`, `NewJEVEvidence` | `cli`, `eval` | [specs/QUALITY.md](../specs/QUALITY.md) |
| `outcome` | Checks an objective's acceptance criteria against the workspace with deterministic predicates. A model's own PASS is never an input. | `Verify`, `VerifyAcceptance`, `ValidateWaiver` | `cli` | [LIFECYCLE-VERDICT-GATE.md](LIFECYCLE-VERDICT-GATE.md), [OUTCOME-RECOVERY-WAIVER.md](OUTCOME-RECOVERY-WAIVER.md) |
| `security` | Optional scanner add-on: runs replaceable scanner adapters, normalizes their findings, and produces a policy verdict for the quality gate. | `BuildAdapters`, `Apply`, `ClassifyCapabilities` | `cli` | [guides/SECURITY-SCANNER-ADDON.md](../guides/SECURITY-SCANNER-ADDON.md), [specs/SECURITY-SCANNER-ADDON.md](../specs/SECURITY-SCANNER-ADDON.md) |

## 7. Approvals, questions, and recovery

What happens when something fails or needs a human.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `failure` | Classifies why a lifecycle stopped short of a pass and returns a disposition (AUTO_FIX, CONTINUE, RETRY, NEEDS_HUMAN). Executes nothing. | `Classify` | `cli`, `autonomy`, `recovery`, `eval`, `deceleval` | [specs/RECOVERY.md](../specs/RECOVERY.md) |
| `recovery` | Maps typed failure evidence to retry, escalate to a larger class, hand to a human, or block, within fixed budgets. | `Decide`, `Recover`, `NextClass` | `cli` | [specs/RECOVERY.md](../specs/RECOVERY.md) |
| `autonomy` | The one place that decides whether a failure or action can be handled automatically or must wait for a human, so approvals stay rare and meaningful. | `Decide`, `DecideEarly`, `DecidePlanChange`, `PolicyFor` | `cli`, `config`, `continuation`, `planflow`, `deceleval` | [CONFIGURATION.md](CONFIGURATION.md) |
| `approval` | The only boundary for reading a human approval gate and recording a decision. Clients never infer approval from a task status. | `New`, `Request`, `Approve`, `Decline`, `Supersede`, `Invalidate` | `cli`, `evaltarget` | [specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md), [guides/APPROVALS.md](../guides/APPROVALS.md) |
| `question` | Durable question and answer records a task can raise to the planner, with its own closed state vocabulary and continuation handling. Backs the `sop_question_*` and `sop_answer_submit` tools. | `CreateQuestion`, `CreateAnswer`, `ApplyAnswer`, `DispatchContinuation` | `cli` | [CLI.md](CLI.md) |
| `taskresult` | Stores results an external executor (for example a Zed agent) publishes outside the run loop. Evidence only; never changes task state. Backs `sop_task_result`. | `Publish`, `List` | `cli` | — |

## 8. Git, GitHub, and delivery

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `git` | Small adapter over the `git` CLI: branches, worktrees, diffs, commits. Agents never call it directly. | `New`, `Adapter`, `TaskBranchName`, `AddWorktree`, `Commit` | `cli`, `parallel`, `security`, `toolharness` | [COMPONENTS.md § Git Adapter](../architecture/COMPONENTS.md#git-adapter) |
| `github` | Remote operations: push, pull requests, checks, merge. | `NewCommandClient`, `Client` | `cli`, `resume`, `mergegate`, `ciremediation` | [COMPONENTS.md § GitHub Adapter](../architecture/COMPONENTS.md#github-adapter) |
| `commitgate` | Decides whether a task commit is allowed, then commits through a port. Never bypasses hooks. **Not wired.** | `New`, `Commit` | — | [COMPONENTS.md § Commit Gate](../architecture/COMPONENTS.md#commit-gate) |
| `mergegate` | Decides from required evidence whether validated work may merge, then merges through a port. **Not wired.** | `New`, `Merge` | — | [COMPONENTS.md § Merge Gate](../architecture/COMPONENTS.md#merge-gate) |
| `ci` | Renders a GitHub Actions workflow from the configured verification commands. **Not wired.** | `Render` | — | [COMPONENTS.md § CI Generation and Remediation](../architecture/COMPONENTS.md#ci-generation-and-remediation) |
| `ciremediation` | Bounded loop that classifies a CI failure and hands actionable ones to a remediator. **Not wired.** | `NewLoop`, `Run` | — | [COMPONENTS.md § CI Generation and Remediation](../architecture/COMPONENTS.md#ci-generation-and-remediation) |
| `completion` | Unsupervised loop: next task, drive to CI, merge, refresh main, repeat. **Not wired.** | `New`, `Run`, `Advance` | — | [COMPONENTS.md § Completion Loop](../architecture/COMPONENTS.md#completion-loop) |

## 9. Decision layer and JEV

Optional, advisory analysis. Never a lifecycle authority.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `decision` | The bounded decision seam: a provider returns a choice and confidence; policy outside the provider decides what it means. The default provider is deterministic rules. | `Decide`, `Select`, `DeterministicProvider`, `NewProvider` | `cli`, `config` | [architecture/JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) |
| `decision/command` | Adapts an external decision process (request on stdin, result on stdout) to `decision.Provider`. | — | `decision` | [architecture/JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) |
| `decision/bridge`, `decision/commandprofile` | Connect the decision seam to the batched decision adapter. | — | `decision` | [architecture/JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) |
| `decisionadapter/cli` | Shared implementation of `sop decision` and `sop-decision-adapter`. | — | `cli`, `cmd/sop-decision-adapter` | [guides/REPOSITORY-CONSOLIDATION.md](../guides/REPOSITORY-CONSOLIDATION.md) |
| `decisionadapter/providers/clef`, `nimble`, `julia`, `fallback` | Decision model adapters (Clef and Nimble through an Ollama-compatible backend, Julia through ONNX) and a primary-then-backup composer. | — | `decisionadapter/cli` | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `decisionadapter/shadow` | Observational shadow evaluation on the adapter side. | — | `decisionadapter/cli` | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `jev` | Boundary to JEV, the optional engineering-analysis capability. JEV may inspect and report findings; it may not change task state or mark anything as passed. | `Analyze`, `NewOllamaAnalyzerFromEnv`, `DefaultPolicy` | `cli`, `autonomy`, `quality`, `router`, `run`, `deceleval` | [specs/OPENJEV.md](../specs/OPENJEV.md), [JEV-OPERATIONS.md](JEV-OPERATIONS.md) |

## 10. Observability and notifications

Recorded for people to read. None of it feeds a decision.

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `activity` | Live event stream of what a run is doing. A nil recorder is a no-op. | `New`, `Emit`, `WithRecorder` | `cli`, `ollamaagent`, `run`, `runtrace` | — |
| `perf` | Where a task spends time and how many agent calls, validations, reviews and fix cycles it costs. Written to `metrics.json`. | `NewRecorder`, `Measure`, `NewRun` | `agent`, `cli`, `ollamaagent`, `runmetrics` | [PERFORMANCE.md](PERFORMANCE.md) |
| `runtrace` | Versioned execution trace written as `trace.json` beside each run. Failing to write it never changes the outcome. | `NewCollector`, `Build`, `Emit` | `cli`, `eval` | [specs/DECISION-TELEMETRY.md](../specs/DECISION-TELEMETRY.md) |
| `runmetrics` | Aggregates metrics across recorded runs with explicit numerators, denominators and coverage. | `Aggregate`, `AggregateProject`, `RenderText` | `cli` | [specs/DECISION-TELEMETRY.md](../specs/DECISION-TELEMETRY.md) |
| `notify` | Notification events with stable identities, the outbox, and file and webhook (Slack-style) channels. | `NewEvent`, `Drain`, `NewFileChannel`, `NewWebhookChannel` | `cli` | [guides/NOTIFICATIONS.md](../guides/NOTIFICATIONS.md) |
| `controller` | Read-only web dashboard over SOP state that delegates commands to the installed `sop`. | `Serve` | `cli`, `cmd/sop-controller` | [guides/SOP-CONTROLLER-DASHBOARD.md](../guides/SOP-CONTROLLER-DASHBOARD.md) |
| `controller/sopclient`, `controller/config`, `controller/web`, `controller/assets` | The dashboard's read-only SOP client, settings, handlers and embedded templates. | — | `controller` | [guides/SOP-CONTROLLER-DASHBOARD.md](../guides/SOP-CONTROLLER-DASHBOARD.md) |

## 11. Command surfaces and configuration

| Package | What it does | Main entry points | Used by | See |
| --- | --- | --- | --- | --- |
| `cli` | The `sop` command set and the wiring between every layer above (about 100 files). Main areas: `run`, `drive`, `task`, `tasks`, `plan`, `reconcile`, `continue` (execution); `validate`, `review`, `gate`, `waiver`, `acceptance_*` (verification); `approval*`, `question`, `planner_raise`, `result` (human and planner interaction); `memory*`, `advisory_*`, `rag_verifier`, `retrieve`, `index` (memory and retrieval); `mcp*` (MCP server tools, including `sop_start_work`); `report*`, `trace*`, `status` (reporting). | `Run` (via `cmd/sop`) | `cmd/sop`, `cmd/sop-memory`, `cmd/deceleval-*` | [CLI.md](CLI.md) |
| `mcp` | Minimal stdio MCP server (JSON-RPC 2.0). Tools call the same services as the CLI; each is scoped to one project. | `NewServer`, `Serve`, `TextTool` | `cli` | [guides/MCP-LIVE-VALIDATION.md](../guides/MCP-LIVE-VALIDATION.md) |
| `config` | Loads `.agent-sdlc/config.yaml` with strict decoding. Policy only: no task state, no secret fields. Includes the `rag:` and `advisory_retrieval:` blocks. | `Load`, `Default`, `Config` | many | [CONFIGURATION.md](CONFIGURATION.md) |
| `dotenv` | Applies an optional `.env` file without overriding variables that are already set. | `LoadDir`, `Apply` | `cli`, `config` | [CONFIGURATION.md](CONFIGURATION.md) |
| `projectdetect` | Detects project types from manifests and Makefile targets and proposes validation commands. Unknown repositories get no guessed commands. | `Detect` | `cli`, `config` | [guides/PROJECT-SETUP.md](../guides/PROJECT-SETUP.md) |
| `skill` | Defines the shipped skill command surface and validates the skill tree and per-agent installers against it. Exercised by tests. **Not wired.** | `Command` | — | [guides/CLAUDE-SKILLS.md](../guides/CLAUDE-SKILLS.md), [guides/ZED-SKILLS.md](../guides/ZED-SKILLS.md) |

## 12. Evaluation harnesses

Off the default execution path. They measure; they don't run tasks.

| Package | What it does | Used by | See |
| --- | --- | --- | --- |
| `eval` | Runs a case corpus through a supplied runner and aggregates success, fix cycles, findings and time. | `cli` | [EVALUATION.md](EVALUATION.md) |
| `evaltarget` | AI-security behavioral suite (AISEC-001 to 006): sandboxed cases with recorded tool calls and deterministic assertions. | tests | [plans/S3-AISEC-ACCEPTANCE.md](../plans/S3-AISEC-ACCEPTANCE.md) |
| `deceleval` | Loads and validates the decision-model corpus for the A/B comparison. | `cmd/deceleval-*` | [DECISION-MODEL-FINDINGS.md](DECISION-MODEL-FINDINGS.md) |
| `decisionquality` | RDM-004 frozen corpora, evidence rules and scoring. | `dqharness`, `cmd/rdm004-eval` | [reports/rdm-decision-memory/](../reports/rdm-decision-memory/) |
| `dqharness` | Runs the RDM-004 A/B comparison with limits and checkpoints. | `cmd/rdm004-eval` | [reports/rdm-decision-memory/](../reports/rdm-decision-memory/) |
| `hipcompare` | Runs one task twice (baseline and candidate instructions) in disposable worktrees and compares the arms. | `cli` | [guides/HIP-COMPARE.md](../guides/HIP-COMPARE.md) |
| `orchadopt` | Gate comparing single-agent and multi-agent execution on parallelizable tasks. | `cli` | [architecture/orchestration.md](../architecture/orchestration.md) |
| `retrievalgate` | Gate on whether structural index plus BM25 beats unranked discovery. | `cli`, `vectoreval` | [architecture/RAG-RETRIEVAL-INTEGRATION.md](../architecture/RAG-RETRIEVAL-INTEGRATION.md) |
| `vectoreval` | Compares BM25 with a model-free vector model and a hybrid. No vector retriever is wired into execution. | `cli` | [specs/RAG-EVALUATION.md](../specs/RAG-EVALUATION.md) |
| `prompttuning` | Promotes a prompt candidate only when evaluation shows a measurable gain and all invariants hold. **Not wired.** | — | [PROJECT-STATUS.md](PROJECT-STATUS.md) |
| `worktreecleanup` | Classification model for the `/sop-cleanup` skill: assigns each dirty path a class and a proposed action. Never deletes. **Not wired.** | — | — |

## 13. Test-only packages

| Package | What it holds |
| --- | --- |
| `archtest` | Architecture tests: core context, retrieval, prompt and agent packages must not depend on a provider implementation. |
| `testfast` | Tests for the `make test-fast` wrapper script. |
| `e2e/harness`, `e2e/lifecycle` | Fake infrastructure and lifecycle tests for the full harness regression suite. |
| `dist` | Install contract tests, including Windows. |
| `tests` | Empty placeholder. |

## Keeping this current

Package descriptions come from each package's `// Package` doc comment; update the
comment first, then this table. When you add a package, put it in the section that
matches its job and fill every column. A package with no non-test importer gets the
**not wired** mark.
