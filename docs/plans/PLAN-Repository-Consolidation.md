# SOP End-to-End Dogfood — Repository Consolidation

Status: PROPOSED — approval required before activation or implementation.
Date: 2026-10-13. All MIG tasks are PLANNED; none has executed.
Intended source path in agentic-sop: docs/plans/PLAN-Repository-Consolidation.md.

## Project

agentic-sop

## Summary

Consolidate controller and advisory adapters into the existing SOP repository, preferably one Go module, using SOP itself to execute and verify the migration. Preserve workflow authority, source history, CLI compatibility, skills, and local model behavior. This document is a review draft prepared outside the user's local repositories. It has not been activated as SOP state.

## Architecture assessment

Committed source inspected in disposable remote clones:

| Repository            | Inspected HEAD                           | Findings                                                                                                                                                                                                  |
| --------------------- | ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| agentic-sop           | d671aac4308433b1685ecf37d3cd18a2a46f50bb | Module github.com/imhttran/agentic-sop; Go 1.27.1; yaml.v3 and SQLite v1.59.0; cmd/sop plus helper/evaluation commands; mature internal/cli and deterministic lifecycle packages.                         |
| sop-controller        | f8502a500e52420ff0b4935b8abc0ebb5b218f2b | Module sop-controller; Go 1.27.0; same SQLite dependency; cmd/sop-controller wires config, read-only SOP client, HTTP server, embedded templates/static.                                                  |
| sop-decision-adapters | 78bba4ed37e6a6cd40e737140c3e7cb9ca10697c | Module github.com/imhttran/sop-decision-adapters; Go 1.22; no external Go dependencies; public decision contract; CLI; internal/providers/{clef,nimble,julia}; shadow evaluation and Python Julia helper. |

Source evidence: each go.mod; controller cmd/sop-controller/main.go, assets.go, internal/config/config.go, internal/sopclient/{store,commands}.go; adapters cmd/sop-decision-adapter/main.go and provider configuration/transports; SOP internal/decision/{decision.go,bridge/,commandprofile/connector.go}, internal/cli/{cli.go,decision_provider.go,decision_adapter_profile.go}, internal/planner/{markdown,plan}.go, internal/dotenv/dotenv.go, install.sh/install.ps1, and both existing .github/workflows/ci.yml files.

Key findings:

- One Go module is plausible from declared dependencies, not yet build-proven. Adopt the existing core toolchain requirement; do not silently lower it.
- Preserve current SOP packages. Do not introduce workflow/ or governance/ relocation as part of consolidation.
- Core internal/decision is the canonical bounded Request/Decision contract. Adapter decision/ is a distinct batched question/answer profile. Existing bridge and commandprofile implement translation; do not collapse these contracts by naming alone.
- The production connector appends `decide -file ... -json` to configured argv and preserves timeout, process-tree cancellation, bounded output and typed failures. A unified executable should retain this protocol with `adapter_command: ["/absolute/path/to/sop", "decision"]`, producing `sop decision decide ...`. Validate this before updating defaults.
- Controller reads state.db in read-only mode and delegates commands with executable argv. Keep this boundary even within one repository. Default command delegation to the current unified executable, while preserving explicit SOP_BIN overrides. No in-process lifecycle reimplementation.
- Controller root assets.go embeds templates/static. Move the embed file and both directories together. Preserve discovery allowlists/depth, approvals, model display, auth, CSRF, sessions, streaming, HTTPS scripts and optional Caddy integration.
- Committed CLEF and Nimble transports use /v1/systemone. The claimed local oMLX chat-completions and Bespoke /score paths are not established by these snapshots. Automatic CLEF→Nimble failover is not demonstrated. Local source and service contract reconciliation is mandatory; do not substitute the committed transport for working local behavior.
- Preserve Julia and shadow evaluation. Julia resolves tools/julia/infer.py relative to working-directory ancestors; installation from another project can break this. Retain the helper path or ship an explicitly resolved support asset; model weights remain external.
- Core uses an optional strict .env parser; controller uses a permissive .env/.env.dev loader; adapter CLI reads process environment directly. Unified configuration needs compatibility tests, not an indiscriminate loader replacement.
- Core CI covers Go build/test/vet/race, tagged JEV evidence, doc links, packaging and Windows installer/skills smoke. Controller has Go CI and development-only Playwright tests. No adapter workflow was found at its inspected HEAD. Keep Node optional for native builds and preserve tagged historical evidence.
- Existing plans and archived evidence contain operational source paths. Do not activate a second plan over an active plan, merge source state databases, move recorded active sources, or rewrite historical evidence to satisfy tests.

Recommended destinations, subject to MIG-001 reconciliation:

| Source                                   | Proposed destination                                                                                         |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Core packages, skills, existing commands | Existing locations unchanged                                                                                 |
| Controller config/sopclient/web          | internal/controller/{config,sopclient,web}/                                                                  |
| Controller assets.go, templates, static  | internal/controller/assets/ with assets.go beside templates/ and static/                                     |
| Controller startup wiring                | internal/controller/ service entry point shared by both commands                                             |
| Adapter public decision/                 | decision/ at unified module root, preserving exported shapes; document changed import path                   |
| Adapter providers/shadow                 | internal/decisionadapter/providers/{clef,nimble,julia}/ and internal/decisionadapter/shadow/                 |
| Adapter CLI implementation               | internal/decisionadapter/cli/ shared by unified and legacy commands                                          |
| Legacy executables                       | cmd/sop-controller/ and cmd/sop-decision-adapter/ as thin wrappers                                           |
| Julia helper                             | tools/julia/ retained, with installation resolution verified                                                 |
| Controller scripts/Caddy/browser tests   | Namespaced scripts/controller/, integrations/controller/, tests/controller/; update path-dependent launchers |
| Component documentation                  | Namespaced docs sections; preserve references through redirects where operational paths require them         |

## Approval and execution boundaries

Only Phase 1 source assessment and Phase 2 plan drafting are authorized now. Do not run this plan until the user approves it. No implementation, commit, push, repository archive/delete, service restart, or production configuration overwrite has occurred.

After approval, use the existing named-plan workflow: `sop run docs/plans/PLAN-Repository-Consolidation.md`. This compiles Markdown through existing planning and task persistence, then executes the graph; it is not a dry-run command. All stages use the compiler's recognized Project/Summary, task IDs, Dependencies, Deliverables, Acceptance Criteria, Acceptance Checks and Execution fields. Run sequentially in the existing checkout. Verify global policy reaches the execution provider once. Keep commit/push unavailable to task agents and require separate explicit authorization for history-integration commits.

Before activation, inspect `sop status` and recorded plan.meta.json locally. If another plan is active, use the existing supported disposition/resume workflow; do not mark unfinished work COMPLETE to make room. Snapshot current executable version/hash to a temporary location outside mutable source so self-modification does not replace the harness mid-run. Build candidate binaries separately and use them only in disposable fixtures until acceptance.

Machine state stays in .agent-sdlc, not this status text. Do not hand-author authoritative plan.json as a substitute for SOP compilation. This draft's graph and fields were structurally inspected; actual PlanFromMarkdown/Validate and SOP activation remain unverified because Go and installed SOP are unavailable here.

History strategy: record every selected source ref, local-only commit and relevant uncommitted delta; preserve full original histories with local Git bundles and provenance outside committed artifacts. Fetch existing histories without rewriting. Prepare an ancestry-preserving integration using existing refs, no squash/filter-repo/rebase/force push. A noncommitting merge using the ours strategy can join both unrelated source histories while preserving the core tree; subsequent mapped imports provide source content. Verify with merge-base --is-ancestor for every imported tip after an explicitly approved merge commit. Test this procedure in a disposable Git fixture first. Never initiate a merge on a dirty index or include unrelated staged changes. If protected local work prevents a safe noncommitting merge, stop and report that concrete boundary; do not stash or reset it automatically. Plan approval does not grant commit approval. No new branch or worktree is expected.

Rollback: retain original repos, binaries, configuration and state; record exact owned paths/hashes before each stage. Before install, discard only migration-owned edits through selective restoration, never reset --hard, blanket checkout, clean or merge-abort over unrelated work. After opt-in install, restore recorded previous binaries and only changed configuration entries; never roll back user project databases. History merges require separate authorization and can be reverted later without rewriting ancestors. Any unsupported schema change, history rewrite, unrelated-diff loss or governance weakening stops execution.

## MIG-001 — Reconcile local source and capture native baselines

Inspect all three local checkouts, active plans, refs, staged/unstaged/untracked changes, installed binaries, Go toolchain, configuration keys and running service contracts. Record the local oMLX and Bespoke integration source without exposing secrets. Run each repository's baseline commands independently and distinguish skipped live tests from passes. No service manipulation.

### Deliverables

- docs/reports/consolidation/baseline.md

### Acceptance Criteria

- All three baseline build/test/vet results and source refs are recorded, with local work ownership and active-plan conflicts explicit.
- Existing Julia helper offline tests and optional controller browser suite status are recorded.
- Unknown local model contracts block provider migration rather than being guessed.

### Acceptance Checks

- `go build ./...`
- `go test ./...`
- `go vet ./...`

### Validation

Run the same three commands from each original repository; run `python3 tools/julia/test_infer.py` from adapters. Record Go versions and installed SOP version. Live model probes are operator-local, bounded and nondestructive. The checks above run from the core project; the cross-repository results belong in the baseline report.

### Execution

- implement

## MIG-002 — Preserve histories and approve package mapping

Prepare bundle backups, source/provenance manifests, path mappings, external public-contract consumers and a tested no-rewrite integration procedure. Do not create a commit. Resolve documentation/path and license attribution collisions before copying files.

### Dependencies

- MIG-001

### Deliverables

- docs/reports/consolidation/source-map.md

### Acceptance Criteria

- Every imported file maps to a pinned source/ref or attributed local delta; no local-only work is silently omitted.
- Backups restore into disposable Git repositories; original refs remain unchanged; imported source retains license attribution.
- Public import-path changes and a separately approved history merge commit are explicit boundaries.

### Acceptance Checks

- `git fsck --full`
- `git status --short`

### Execution

- implement

## MIG-003 — Import the public adapter contract, fixtures, and Julia helper

Import only the adapter public `decision/` contract to module-root `decision/`, preserving its exported shapes and its batched question/answer profile; import its tests and test fixtures; retain the Julia helper `tools/julia/` with its offline test. Mechanically rewrite the import path to the unified module. Use the deterministic tools: copy each file byte-exact with `copy_file`, then apply the documented import rewrite with `rewrite_file`, before validation (manifest: docs/reports/consolidation/import-rewrites.md). If the import already exists and every acceptance check passes, return the ALREADY_SATISFIED outcome with verified evidence instead of re-copying. Do not import providers or shadow here. Keep the canonical core `internal/decision` seam distinct, and preserve package-level license notices and legacy source provenance.

### Dependencies

- MIG-002

### Deliverables

- decision/
- tools/julia/
- decision test fixtures (testdata/)

### Acceptance Criteria

- `decision/` builds at module root with exported shapes preserved and imports rewritten to the unified module path.
- The adapter batched question/answer contract stays separate from core `internal/decision`; no provider coupling is added.
- The Julia helper is present and its offline test `python3 tools/julia/test_infer.py` passes.
- The adapter decision package's tests/fixtures are imported and accounted for; none silently excluded.

### Acceptance Checks

- `go build ./decision/...`
- `go test ./decision/...`
- `go vet ./decision/...`
- `python3 tools/julia/test_infer.py`

### Execution

- implement

## MIG-003A — Import adapter providers and shadow evaluation

Import the adapter providers `internal/providers/{clef,nimble,julia}` and `internal/shadow` into `internal/decisionadapter/providers/{clef,nimble,julia}/` and `internal/decisionadapter/shadow/`, rewriting imports to the unified module and the module-root `decision/` contract. Use the deterministic tools: copy each file byte-exact with `copy_file`, then apply the documented import rewrite with `rewrite_file`, before validation (manifest: docs/reports/consolidation/import-rewrites.md). If the import already exists and every acceptance check passes, return the ALREADY_SATISFIED outcome with verified evidence instead of re-copying. Preserve provider tests/fixtures and license notices. Keep the Julia provider consistent with the retained `tools/julia/` helper location. Do not add provider coupling to core.

### Dependencies

- MIG-003

### Deliverables

- internal/decisionadapter/providers/clef/
- internal/decisionadapter/providers/nimble/
- internal/decisionadapter/providers/julia/
- internal/decisionadapter/shadow/

### Acceptance Criteria

- Providers and shadow build against module-root `decision/`; exported behavior is unchanged.
- Provider and shadow tests/fixtures are imported and pass; no provider coupling appears in core (`internal/decision`).
- The Julia provider resolves the retained `tools/julia/` helper path.

### Acceptance Checks

- `go build ./internal/decisionadapter/...`
- `go test ./internal/decisionadapter/...`
- `go vet ./internal/decisionadapter/...`

### Execution

- implement

## MIG-003B — Import controller config and SOP client

Import the controller `internal/config` and `internal/sopclient` into `internal/controller/config/` and `internal/controller/sopclient/`. Copy every file byte-exact with the `copy_file` tool, then apply the documented exact import rewrite with `rewrite_file` before any validation: in each imported file that contains it, replace every occurrence of `"sop-controller/internal/config"` with `"github.com/imhttran/agentic-sop/internal/controller/config"`. The rewrite is idempotent, so re-running the import is safe. If the import already exists and every acceptance check passes, return the ALREADY_SATISFIED outcome with verified evidence instead of re-copying. Do not modify any other file. Preserve the read-only state database boundary and executable-argv command delegation; do not reimplement lifecycle in-process. Preserve the controller's permissive `.env`/`.env.dev` loading behavior and its tests, and preserve package-level license notices.

### Dependencies

- MIG-002

### Deliverables

- internal/controller/config/
- internal/controller/sopclient/

### Acceptance Criteria

- config and sopclient build under the unified module with the read-only store boundary and command delegation preserved.
- All scoped files match their sibling source except the exact documented import rewrites (established deterministically — see docs/reports/consolidation/import-rewrites.md); no `sop-controller/internal/config` import remains, and the rewrite is idempotent.
- The imported config/sopclient tests/fixtures pass; configuration compatibility behavior is unchanged.
- No in-process lifecycle reimplementation is introduced.

### Acceptance Checks

- `go build ./internal/controller/...`
- `go test ./internal/controller/config/... ./internal/controller/sopclient/...`
- `go vet ./internal/controller/...`

### Execution

- implement

## MIG-003C — Import controller web packages and embedded assets

Import the controller `internal/web` into `internal/controller/web/`, and relocate `assets.go` with `templates/` and `static/` together into `internal/controller/assets/` (assets.go beside templates/ and static/, embed directives intact). Rewrite imports using the deterministic tools: copy each file byte-exact with `copy_file`, then apply the documented import rewrite with `rewrite_file`, before validation (manifest: docs/reports/consolidation/import-rewrites.md). If the import already exists and every acceptance check passes, return the ALREADY_SATISFIED outcome with verified evidence instead of re-copying. Preserve discovery allowlists/depth, approvals, model display, auth, CSRF, sessions and streaming behavior, and keep the web layer a presentation boundary.

### Dependencies

- MIG-003B

### Deliverables

- internal/controller/web/
- internal/controller/assets/

### Acceptance Criteria

- web packages build and the embedded templates/static resolve from the relocated embed file.
- The imported web tests/fixtures pass; discovery, approvals, auth, CSRF and session behavior are preserved.
- No change to workflow authority; the web layer remains a presentation boundary.

### Acceptance Checks

- `go build ./internal/controller/...`
- `go test ./internal/controller/web/...`
- `go vet ./internal/controller/...`

### Execution

- implement

## MIG-003D — Reconcile module dependencies and verify all imported packages

Reconcile `go.mod`/`go.sum` for the imported controller and adapter code under the existing core toolchain (go 1.27.1) without introducing a framework, so every imported package builds in one module. Update the architecture guard so transport code is permitted only in the designated adapter subtree. Verify every imported package and record fixture accounting.

### Dependencies

- MIG-003
- MIG-003A
- MIG-003B
- MIG-003C

### Deliverables

- go.mod
- go.sum
- internal/archtest/
- docs/reports/consolidation/import-verification.md

### Acceptance Criteria

- `go build ./...` succeeds under one module; go.mod/go.sum are consistent and no nested module silently excludes migrated tests.
- All imported packages' tests pass; the architecture guard permits transport code only in the designated adapter subtree, never provider coupling in core.
- Fixture accounting enumerates every imported component's tests/fixtures (no silent exclusion).

### Acceptance Checks

- `go build ./...`
- `go test ./decision/... ./internal/controller/... ./internal/decisionadapter/... ./internal/decision/... ./internal/archtest/...`
- `go vet ./decision/... ./internal/controller/... ./internal/decisionadapter/...`

### Execution

- implement

## MIG-004 — Add unified CLI and compatible wrappers

Add sop controller serve and sop decision decide using the existing CLI dispatch and shared component entry points. Bare sop decision displays command help; preserve direct legacy decide semantics, flags, stdout JSON, stderr diagnostics and exit codes. Retain cmd/sop-controller and cmd/sop-decision-adapter wrappers, all current SOP verbs and helper commands. No lifecycle initialization merely to request help or a decision.

### Dependencies

- MIG-003D

### Deliverables

- internal/cli/
- cmd/sop/
- cmd/sop-controller/
- cmd/sop-decision-adapter/

### Acceptance Criteria

- Unified and legacy commands use shared implementations, with regression assertions for flags/help/exit codes and JSON-only stdout.
- Existing sop run behavior and other verbs remain compatible; controller errors return through the command layer rather than unexpectedly exiting tests.
- The connector's appended decide token works with argv [sop, decision] without recursion.

### Acceptance Checks

- `go test ./internal/cli/... ./internal/decisionadapter/... ./cmd/...`
- `go build ./cmd/...`
- `go vet ./internal/cli/... ./cmd/...`

### Execution

- implement

## MIG-005 — Integrate controller without changing authority

Wire project discovery, SOP command delegation, read-only stores, assets, auth/session/CSRF, approvals, streaming and model display into unified startup. Default SOP command path to the current binary; respect SOP_BIN. Preserve launch and HTTPS conveniences through explicit namespaced targets rather than overwriting core Makefile targets.

### Dependencies

- MIG-004

### Deliverables

- internal/controller/
- scripts/controller/
- integrations/controller/
- Makefile

### Acceptance Criteria

- Startup from another working directory finds embedded assets and configured projects; discovery remains allowlisted and bounded.
- Every action delegates to SOP; views do not mutate state; auth, CSRF, session persistence and approval attribution remain covered.
- Tests use ephemeral listeners/projects/session stores and never interact with the running user controller or Caddy instance.

### Acceptance Checks

- `go test ./internal/controller/... ./internal/cli/...`
- `go test -race ./internal/controller/...`
- `go vet ./internal/controller/...`

### Execution

- implement

## MIG-006 — Preserve local advisory integration and fallback

Migrate the reconciled CLEF/oMLX and Bespoke-Nimble implementations. Use existing normalized providers, profile bridge and evidence fields. If automatic failover is absent, implement only the bounded chain needed for CLEF primary, Nimble backup and deterministic SOP fallback; keep deterministic authority in SOP. Preserve Julia, existing SystemOne compatibility and shadow evaluation.

### Dependencies

- MIG-004

### Deliverables

- internal/decisionadapter/
- internal/decision/
- internal/cli/decision_evidence.go
- docs/reports/consolidation/decision-compatibility.md

### Acceptance Criteria

- Fixture tests cover primary success, primary timeout/failure and successful backup, both unavailable, malformed payloads, invalid confidence and cancellation.
- Actual successful provider and secret-free fallback reason are attributed; all deadlines fit a bounded overall consultation budget.
- Advisory results cannot lower risk, approve work, bypass verification or alter execution-agent routing/JEV/model-picker behavior.
- Bounded local live smoke verifies oMLX and Bespoke contracts without stopping or restarting either service; unavailable live infrastructure is reported as blocked.

### Acceptance Checks

- `go test ./internal/decisionadapter/... ./internal/decision/... ./internal/cli/... ./internal/archtest/...`
- `go vet ./internal/decisionadapter/... ./internal/decision/... ./internal/cli/...`

### Execution

- implement

## MIG-007 — Consolidate configuration with explicit precedence

Combine examples, not secrets. Preserve existing environment names, explicit project decision disablement and command overrides. Keep controller .env.dev compatibility scoped to controller mode. Avoid changing strict core parsing through reuse of the permissive controller loader. Document the unified argv and optional provider/helper assets.

### Dependencies

- MIG-005
- MIG-006

### Deliverables

- .env.example
- internal/config/
- internal/dotenv/
- internal/controller/config/
- docs/reference/CONFIGURATION.md

### Acceptance Criteria

- Tests pin project decision config > environment > deterministic default and exported env > .env; unrelated routing precedence remains unchanged.
- No real .env, access token, weights, session state or provider secret enters tracked source or decision evidence.
- Exact working local model variables and YAML are based on the reconciled implementation, not invented endpoint settings.

### Acceptance Checks

- `go test ./internal/config/... ./internal/dotenv/... ./internal/controller/config/... ./internal/decisionadapter/...`
- `go vet ./internal/config/... ./internal/dotenv/... ./internal/controller/config/...`

### Execution

- implement

## MIG-008 — Update native installers and support assets

Update shell and PowerShell installers for unified commands and compatible executables; preserve installer options, ownership markers and skills/plugin behavior. Verify Julia helper resolution from arbitrary project roots. Keep Python/ONNX optional and Caddy external. Test installation in temporary destinations without replacing the running SOP harness or touching personal skill settings.

### Dependencies

- MIG-007

### Deliverables

- install.sh
- install.ps1
- scripts/install/
- internal/dist/

### Acceptance Criteria

- Existing installer interfaces work; unified CLI operates without a checkout or Node runtime.
- Legacy wrappers and required optional support assets have a documented install strategy; unavailable Julia dependencies remain an explicit optional capability.
- Zed/Claude skills names, managed ownership and packaged plugin consistency remain covered by existing native distribution tests.

### Acceptance Checks

- `go test ./internal/dist/... ./internal/skill/...`
- `go build ./cmd/...`
- `go vet ./internal/dist/...`

### Validation

Run existing shell/PowerShell dry-run and isolated installation smoke paths; do not execute personal install, token rotation or service restart commands. Cross-platform checks run in their native CI environments.

### Execution

- implement

## MIG-009 — Extend CI and preserve regression suites

Extend existing core workflows to migrated Go packages, CLI compatibility, controller security/discovery, decision/fallback fixtures and optional support assets. Preserve race tests, tagged JEV evidence, Windows installation, canonical skill enumeration, plugin checks and doc-link checks. Keep controller Playwright in a namespaced development suite with explicit setup; do not require Node for Go build/test or live models in CI.

### Dependencies

- MIG-008

### Deliverables

- .github/workflows/ci.yml
- tests/controller/
- docs/reports/consolidation/ci-coverage.md

### Acceptance Criteria

- Every original suite has a retained destination or a documented equivalence; fixture references survive relocation without rewriting frozen historical evidence.
- Linux/macOS compatibility and Windows builds/installers have a clear check matrix; unavailable native checks are not labeled passing.
- Failure injection uses test servers/dependency injection, not live service manipulation.

### Acceptance Checks

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`

### Validation

Retain existing tagged JEV acceptance commands and packaging checks. Run browser suite after its development dependencies are available and record the separate result.

### Execution

- implement

## MIG-010 — Update human-facing README and references

Explain SOP as a native-toolchain SDLC harness with three command surfaces, ownership boundaries, quick starts, local configuration, migration compatibility and optional integrations. Namespace imported documentation, fix links and keep operational source paths stable. Historical reports remain point-in-time; use errata or redirect docs rather than rewriting evidence.

### Dependencies

- MIG-009

### Deliverables

- README.md
- docs/README.md
- docs/guides/REPOSITORY-CONSOLIDATION.md
- docs/reports/consolidation/documentation.md

### Acceptance Criteria

- Reader can install, run SOP, serve controller and consult decisions from the documented unified commands.
- CLI/API compatibility limits and optional Python/Node/Caddy requirements are explicit.
- Active plan identities and archived evidence remain traceable; internal link checks pass.

### Acceptance Checks

- `go test ./internal/dist/... ./internal/planner/...`

### Validation

Run `bash scripts/checks/check-doc-links.sh` using the established documentation checker; record its result. Do not add shell-based acceptance checks that the safe argv policy does not permit.

### Execution

- implement

## MIG-011 — Execute the end-to-end dogfood regression

Use a candidate binary and disposable project to discover projects, launch real SOP execution through the controller, complete a small native Go behavior change with targeted validation, display results/approvals and inspect run evidence. Run local CLEF consultation plus simulated failover cases and installation startup smoke. Preserve the harness executing this migration; do not self-replace mid-run.

### Dependencies

- MIG-010

### Deliverables

- docs/reports/consolidation/dogfood.md
- docs/reports/consolidation/performance.md

### Acceptance Criteria

- Controller launches the candidate SOP process against a disposable project; real governed execution produces trace, validation, review and outcome evidence.
- A protected action still stops at the existing approval boundary; no task declares acceptance merely from an agent's completion claim.
- All ten Phase 4 checks are mapped to evidence, including actual local provider smoke and injected failures; blocked live checks remain blocked.
- Compare baseline/candidate binary size, startup latency, discovery behavior and decision failover latency with toolchain/hardware/fixture details; explain operational regressions before completion.

### Acceptance Checks

- `go build ./...`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`

### Validation

Run migrated CLI/controller integration tests and existing tagged evidence tests. Real model and real-agent dogfood are supplemental operator-local runs, never inferred from mocked CI passes. Failure injection uses disposable servers and command/config fixtures only.

### Execution

- implement

## MIG-012 — Report readiness and retirement recommendations

Produce the final task/test/architecture/compatibility/risk report and recommended legacy-repository transition. Document ancestry integration readiness and pending commit approval; do not commit, push, archive, retire or delete anything. Original repos remain available for rollback until a separately authorized transition after accepted usage.

### Dependencies

- MIG-011

### Deliverables

- docs/reports/consolidation/completion.md
- docs/guides/LEGACY-REPOSITORY-TRANSITION.md

### Acceptance Criteria

- Every task and validation is PASS, FAIL or BLOCKED with evidence; unknown checks are not treated as completed.
- History manifest, restored-backup evidence, source mappings and approved/pending ancestry commit status are explicit; do not declare Git-history integration complete before ancestor checks pass.
- Recommend later deprecation notices, release/issue redirection and eventual archival only after approval; no automatic retirement action.
- All remaining live checks, performance regressions and external public-package consumers are reported.

### Acceptance Checks

- `git diff --check`
- `git status --short`
- `go test ./internal/cli/... ./internal/dist/... ./internal/archtest/...`

### Execution

- implement

## Current findings and approval decision

Recommendation: approve this phased direction, conditional on MIG-001 local reconciliation. One module and a unified CLI are reasonable; preserve independent controller/adapter boundaries, compatibility wrappers and SOP's canonical decision seam. No core package reorganization is needed.

Phase 1 is complete for the inspected committed snapshots, but local uncommitted work, active plan state, installed binaries and live contracts remain unassessed. Phase 2 has a compiler-shaped review draft with 12 sequential tasks, executable acceptance commands and rollback boundaries. Go/SOP compilation, native baseline tests and runtime checks were not run here. No implementation change, history mutation, commit, push, service change or legacy retirement is authorized by this draft.
