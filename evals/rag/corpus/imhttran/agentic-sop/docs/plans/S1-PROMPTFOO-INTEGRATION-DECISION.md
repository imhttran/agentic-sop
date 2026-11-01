# S1 — Promptfoo Integration Decision

Status: decision record. Stage S1 of the Promptfoo security-evals work.

Scope guard: this record selects one optional Promptfoo adapter and no second
workflow runner. Garak, PyRIT, Guardrails AI, scheduling, and Tran-specific
adapters are explicitly out of scope.

## 1. Branch state and repository instructions

- Branch state at S1: `feat/promptfoo-security-evals`, tracking
  `origin/feat/promptfoo-security-evals`. This is the isolated feature branch for
  the Promptfoo work; no worktree switch was needed.
- Repository instructions: `CLAUDE.local.md` (local lessons) and `.claude/`
  (agent command definitions). `README.md` points to both.
- Existing SOP workflow: `README.md` (Basic Workflow section) and
  `docs/specs/WORKFLOW.md`, `docs/specs/TASK-LIFECYCLE.md`,
  `docs/specs/EXECUTION.md`. The boundary is owned by
  `docs/architecture/SOP-BOUNDARY.md`.
- Unrelated change present in the tree: `docs/plans/PLAN-Executore-Integration.md`
  (untracked). It is preserved unchanged, not reverted, cleaned, or amended.

## 2. Phase 1 inspection targets

The four Phase 1 inspection targets are recorded with a concrete path or
command.

### Target 1 — Scanner adapters, command execution, version pinning

- Adapter contract: `internal/security/contract.go` (`Adapter` interface,
  `Descriptor`, `Request`, `Execution`, `ProtocolVersion = security-scan/v1`).
- Adapter construction and pinning: `internal/security/adapter.go`
  (`BuildAdapters`, `BuildAdapter`, `adapterConfigDigest`, `descriptorLanguages`).
  Version pinning is the `version` field in `config.SecurityAdapterConfig`,
  surfaced as `Descriptor.AdapterVersion` and `Provenance.ScannerVersion`.
- Command execution boundary: `internal/security/exec.go` and
  `internal/security/runner.go`; `scanContext` in `internal/security/adapter.go`
  derives the bounded per-adapter context.
- External process adapter: `internal/security/external.go`
  (`SecurityProtocolExternal`).
- Built-in adapters already wired: `internal/security/semgrep.go`,
  `internal/security/gitleaks.go`, `internal/security/cargoaudit.go`,
  `internal/security/osv.go`.
- Configuration schema: `internal/config/security.go`
  (`SecurityAdapterConfig`, `SecurityProtocolBuiltin`, `SecurityProtocolExternal`).

### Target 2 — Reporting and artifact storage

- Normalized report assembly and verdict: `internal/security/report.go` and
  `internal/security/policy.go` (`Verdict`, `ExecutionStatus`, `Coverage`).
- Sanitization and identity: `internal/security/normalize.go`,
  `internal/security/identity.go`.
- Artifact storage and evidence: `internal/outcomefile/`, `internal/jev/`, and
  the check-evidence path in `internal/cli/` (`sop check`). Evidence follows the
  existing secret-redaction and retention rules; no separate storage is added.

### Target 3 — Approval enforcement and verification-evidence checks

- Approval enforcement: `internal/approval/` (requirement resolution, gates).
- Verification-evidence checks: `internal/verify/` and the `sop check` evidence
  path in `internal/cli/`. Missing required evidence blocks through the existing
  mechanism; evaluation results never satisfy approval.

### Target 4 — RAG, advisory memory, and tool-execution boundaries

- RAG retrieval: `internal/ragclient/`, `internal/retrieval/`,
  `internal/retrievalboundary/`, `internal/retrievalgate/`. Retrieved content is
  untrusted and can never satisfy approvals.
- Advisory decision memory: `internal/decisionmemory/` (disabled by default).
- Tool-execution boundaries: `internal/toolharness/`, `internal/commandpolicy/`,
  and the IMPLEMENT capability tool allow-list enforced by SOP.

Concrete commands used for this inspection target set:

```bash
ls internal/security internal/config internal/approval internal/verify \
   internal/ragclient internal/retrieval internal/toolharness internal/commandpolicy
grep -n "SecurityAdapterConfig" internal/config/security.go
grep -n "type Adapter interface" internal/security/contract.go
```

## 3. Promptfoo pin and verification findings

- Pin: `promptfoo@0.118.0` (npm distribution), invoked through the existing
  command-execution layer. The pin is recorded in adapter configuration; the
  adapter never installs a version at run time.
- License: MIT. Provider interface: the OpenAI-compatible HTTP provider
  (`http` provider with `apiBaseUrl`) and the `exec` provider; no SOP-specific
  provider plugin is added. Output schema: the `results.json` written by
  `promptfoo eval --output`, consumed as a native report.
- Data handling: evaluation runs locally; telemetry is disabled through the
  documented setting so no result leaves the host.
- Provisional marker: this record could not re-fetch the official release
  artifacts from `https://github.com/promptfoo/promptfoo` and
  `https://www.promptfoo.dev/docs/` inside this environment. The license,
  provider, output-schema, and data-handling findings above are therefore
  PROVISIONAL and must be re-verified in S4 before implementation claims
  verified behavior.

## 4. Selected integration

Smallest integration: one optional adapter that runs the pinned Promptfoo CLI
through the existing command-execution layer, normalizes `results.json` into an
`Execution` under a new evaluation category, and lets SOP policy decide the
verdict.

- It adds no second workflow runner: execution, retries, artifacts, approvals,
  and the final verdict stay SOP-owned.
- No new production endpoint is added solely for evaluations.
- The adapter is disabled unless configured.
- Garak, PyRIT, Guardrails AI, scheduling, and Tran-specific adapters are out of
  scope and remain unimplemented.

### Selected command

```bash
npx --yes promptfoo@0.118.0 eval -c <config> --output <results.json> --no-progress-bar
```

### Selected configuration

An `external`-protocol adapter entry with ID `promptfoo`, the pinned `version`,
and the Promptfoo config path passed as an input. The target and provider are
supplied by operator configuration outside the committed file.

### Target contract

The target is an external HTTP endpoint or model served through an
OpenAI-compatible API base URL. It is configured by the operator; credentials
never enter committed configuration and never enter the adapter config digest.
If the target is unavailable, the run is reported as blocked with the exact
missing configuration key. No evaluation endpoint is added to production code.

### Evidence format

The normalized evidence is a `security.Execution` with:

- `AdapterID = "promptfoo"`, `AdapterVersion = "0.118.0"`.
- `Provenance.ScannerVersion`, `Provenance.ConfigurationDigest`,
  `Provenance.ContentIdentity`, `Provenance.PolicyIdentity` as for the other
  adapters.
- `NativeReportDigest` plus a sanitized native report retained for debugging.
- One `Finding` per failing Promptfoo assertion, with the red-team category as
  `RuleID`, normalized `Severity`, and a stable `Fingerprint`.

## 5. Check-command set

The check-command set reuses the existing verification commands. Each entry has
a concrete file path or command.

| Check | Path or command |
| --- | --- |
| Build | `go build ./...` (`Makefile`) |
| Unit and integration tests | `go test ./...` (`Makefile`) |
| Static analysis | `go vet ./...` (`Makefile`) |
| Security package tests | `go test ./internal/security/...` |
| CLI evidence tests | `go test ./internal/cli/...` |

## 6. Out of scope

Garak, PyRIT, Guardrails AI, scheduling, and Tran-specific adapters are out of
scope for this integration and for the following stages.
