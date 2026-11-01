# CLOSE-001 — Current Repository Baseline

Secrets-redacted baseline snapshot for the pre-performance-closure plan. This Markdown
report is the sole permitted intentional repository mutation created by CLOSE-001.

- Captured at (UTC): 2026-10-04T17:29:23.527512+00:00
- Capture discipline: read-only for all repositories except this report file.
- Secret handling: only allow-listed public `SOP_MODEL_*` mapping fields and provenance
  identifiers are quoted. No credential or endpoint values are recorded.

---

## 1. agentic-sop

| Field | Value |
| --- | --- |
| Repository name | agentic-sop |
| Repository path | /Users/imhttran/agentic-workspace/agentic-sop |
| Branch | fix/verify-repository-mutations |
| HEAD SHA | 79634dc60bea1c80426eac395a6efc5698972c5e |
| Local main HEAD | d46bc1674e6f25e6c57219b0e49e0853a94f40e5 |
| Divergence from local main (ahead / behind) | 7 / 0 |
| Tracked dirty state | true |
| Uncommitted patch exists | true |
| Tracked patch SHA-256 | a10f0df7a2b00d07a29f68d903fd13bb3e9b82a7c52fa2368a7366b49745583f |

### Verbatim sanitized `git status --short` (captured before writing this report)

```
 M docs/reference/CLI.md
 M internal/cli/cli.go
 M internal/cli/drive.go
 M internal/cli/jev.go
 M internal/cli/mutation.go
 M internal/cli/run.go
 M internal/git/git.go
 M internal/ollamaagent/prompt.go
 M internal/taskfile/taskfile.go
?? internal/cli/report_deliverable.go
?? internal/cli/report_deliverable_test.go
?? internal/cli/task_input.go
```

### Tracked dirty state (pre-existing, user-owned)

```
 M docs/reference/CLI.md
 M internal/cli/cli.go
 M internal/cli/drive.go
 M internal/cli/jev.go
 M internal/cli/mutation.go
 M internal/cli/run.go
 M internal/git/git.go
 M internal/ollamaagent/prompt.go
 M internal/taskfile/taskfile.go
```

### Untracked inventory (pre-existing, user-owned)

- internal/cli/report_deliverable.go
- internal/cli/report_deliverable_test.go
- internal/cli/task_input.go

### Divergence note

Divergence from local main is available: the branch is 7 commits ahead and 0 commits
behind `main`. `git rev-list --left-right --count main...HEAD` returned `0\t7`:
the left count is main-only and the right count is HEAD-only. These HEAD values
are the starting facts observed at CLOSE-001
run time; they are not evergreen SHAs and may be re-derived after later authorized
closure edits (CLOSE-005 owns any repinning).

---

## 2. sop-controller

| Field | Value |
| --- | --- |
| Repository name | sop-controller |
| Repository path | /Users/imhttran/agentic-workspace/projects/sop-controller |
| Branch | main |
| HEAD SHA | a51b0c6a6563033821ed4ae1e51890ab10479bbd |
| Local main HEAD | a51b0c6a6563033821ed4ae1e51890ab10479bbd |
| Divergence from local main (ahead\tbehind) | 0\t0 |
| Tracked dirty state | false |
| Uncommitted patch exists | false |
| Tracked patch SHA-256 | e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 (empty) |

### Verbatim sanitized `git status --short` (captured before writing this report)

```
?? c2-009-dogfood.4a0Q7q/
?? c2-009-dogfood.QnUK2o/
?? c2-009-dogfood.th8DCS/
?? c2-009-dogfood.vt4eoy/
```

### Tracked dirty state (pre-existing)

None — no tracked files modified, staged or deleted.

### Untracked inventory (pre-existing)

- c2-009-dogfood.4a0Q7q/controller.log
- c2-009-dogfood.4a0Q7q/project/.agent-sdlc/config.yaml
- c2-009-dogfood.4a0Q7q/project/PLAN.md
- c2-009-dogfood.4a0Q7q/transcript.log
- c2-009-dogfood.QnUK2o/controller.log
- c2-009-dogfood.QnUK2o/project/.agent-sdlc/config.yaml
- c2-009-dogfood.QnUK2o/project/PLAN.md
- c2-009-dogfood.QnUK2o/transcript.log
- c2-009-dogfood.th8DCS/controller.log
- c2-009-dogfood.th8DCS/project/.agent-sdlc/config.yaml
- c2-009-dogfood.th8DCS/project/PLAN.md
- c2-009-dogfood.th8DCS/transcript.log
- c2-009-dogfood.vt4eoy/controller.log
- c2-009-dogfood.vt4eoy/project/.agent-sdlc/config.yaml
- c2-009-dogfood.vt4eoy/project/PLAN.md
- c2-009-dogfood.vt4eoy/transcript.log

### Read-only statement

The sop-controller sibling checkout is strictly read-only during CLOSE-001. No file,
configuration, branch or commit in that repository was created, modified or deleted by
the CLOSE-001 capture.

---

## 3. Toolchain/runtime

- Go version: `go version go1.27.1 darwin/arm64`
- SOP binary: `/Users/imhttran/go/bin/sop`
- `sop version`: `sop dev`
- Build/VCS revision (`vcs.revision`): `79634dc60bea1c80426eac395a6efc5698972c5e`
- Build VCS time (`vcs.time`): `2026-10-03T20:37:14Z`
- VCS modified state (`vcs.modified`): `true`
- SHA-256 of actual SOP binary: `172778e5c45b5ec3f46f7e2b0caeeb34fefee2f596b20a8440674afb7c7979d1`
- Unavailable build fields: `sop version` reports only `sop dev` (no semantic release
  version is embedded), and no separate build-id/commit-date field is exposed beyond
  the VCS settings above; these are marked unavailable rather than fabricated.

### Binary/source identity (repair installed before CLOSE-001)

The recorded `vcs.revision` equals the current agentic-sop HEAD
(`79634dc60bea1c80426eac395a6efc5698972c5e`), but `vcs.modified` is `true`: the
installed binary was built from the repaired working tree before CLOSE-001 and is
not a clean build of HEAD alone. Its revision matches HEAD and its source includes
the captured uncommitted repair; this does not establish a mismatch with the
working tree. No historical SHA is asserted to be current.
CLOSE-001 does not rebuild, align, install, repin or otherwise change the SOP binary.
Remediation (rebuild/align/install/repin a reproducible binary) is owned by CLOSE-005,
which will update the pinned source SHA and binary hash recorded here.

---

## 4. Model configuration

### Configured mappings (configuration only — not selection, not availability)

#### SMALL

- Provider: `ollama`
- Model: `qwen3:4b`
- Locality: local
- Fallback: provider `ollama`, model `nemotron-3-nano:30b-cloud`, locality cloud

#### MEDIUM

- Provider: `ollama`
- Model: `nemotron-3-super:cloud`
- Locality: cloud
- Fallback: none configured for this cloud class

#### LARGE

- Provider: `ollama`
- Model: `deepseek-v4.1-flash:cloud`
- Locality: cloud
- Fallback: none configured for this cloud class

### Routing and related switches

- Automatic routing enabled/disabled: disabled (`false`; built-in default)
- Configured default class: `large`
- Configured completeness fallback class: `medium`
- Allow cloud fallback for local: `false`
- Bounded escalation enabled/disabled: disabled (`false`; built-in default)

### Redacted configuration provenance

- Project config: `.agent-sdlc/config.yaml` (agent harness/provider/model configured;
  no `models` block present)
- Environment config: `.env` (only public `SOP_MODEL_*` mapping fields collected; all
  credentials and endpoints omitted)
- Public model fields SHA-256: `e7f6ba5eb13732e88f368b70ded1a4d6e24917227020ae365d265e9e7cc3964a`

### Selected target for CLOSE-001 (distinct from configuration)

- At the initial snapshot, the new run's selection had not yet been observed.
- Post-run observation: SOP recorded class `large`, provider `ollama`, model
  `deepseek-v4.1-flash:cloud`, locality `cloud`, source `env`, fallback `false`,
  reason `environment default class` in
  `.agent-sdlc/runs/CLOSE-001/model-selection.json`. The run's startup output
  reported the same selection. Configured mappings alone do not prove selection.

### Runtime availability (distinct from configuration and selection)

Recorded only where actually observed; never inferred from configuration.

| Target | Runtime availability | Fallback availability |
| --- | --- | --- |
| SMALL (ollama / qwen3:4b) | NOT OBSERVED / UNAVAILABLE | NOT OBSERVED / UNAVAILABLE |
| MEDIUM (ollama / nemotron-3-super:cloud) | NOT OBSERVED / UNAVAILABLE | NOT OBSERVED / UNAVAILABLE (none configured) |
| LARGE (ollama / deepseek-v4.1-flash:cloud) | The real CLOSE-001 workflow returned PLAN, IMPLEMENT and REVIEW results; no independent availability probe ran | NOT OBSERVED / UNAVAILABLE (none configured) |
| SMALL fallback (ollama / nemotron-3-nano:30b-cloud) | NOT OBSERVED / UNAVAILABLE | NOT OBSERVED / UNAVAILABLE |

The initial snapshot made no provider probe or generation call. The subsequent
SOP lifecycle used its configured provider; that observation does not establish
availability for SMALL, MEDIUM, or any fallback.

---

## 5. Mutation record and preservation statement

### Initial state captured before writing

The agentic-sop and sop-controller sections above record the exact repository state as
captured before this report was written. Those pre-existing tracked modifications and
untracked files were present in the working tree prior to CLOSE-001 and are user-owned.

### Authorized report delta (sole intentional mutation)

- Added: `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` (this file)
- Added parent directory: `docs/reports/pre-performance-closure/`

This is the only intentional repository mutation created or updated by CLOSE-001. No
separate `.json`, patch, configuration or other second artifact is produced; all
evidence lives in this Markdown report.

### Preservation verification (after writing)

- The pre-existing tracked modifications (`docs/reference/CLI.md`, `internal/cli/cli.go`,
  `internal/cli/drive.go`, `internal/cli/jev.go`, `internal/cli/mutation.go`,
  `internal/cli/run.go`, `internal/git/git.go`, `internal/ollamaagent/prompt.go`,
  `internal/taskfile/taskfile.go`) remain present and unmodified.
- The pre-existing untracked files (`internal/cli/report_deliverable.go`,
  `internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go`) remain
  present and unmodified.
- The sop-controller checkout shows no new modifications attributable to CLOSE-001.
- No reset, clean, amend or overwrite of unrelated work occurred.

### Non-mutation statement

CLOSE-001 did not mutate application/source code, tests, runtime configuration, SOP
configuration, any Git branch, any existing user change, the sibling sop-controller
repository content, or any installed binary. The SOP binary was not rebuilt, aligned
or reinstalled. sop-controller was strictly read-only.

### Pinned snapshot (re-validated/repinned only by CLOSE-005)

- agentic-sop source revision: `79634dc60bea1c80426eac395a6efc5698972c5e`
- agentic-sop binary hash: `172778e5c45b5ec3f46f7e2b0caeeb34fefee2f596b20a8440674afb7c7979d1`
- sop-controller source revision: `a51b0c6a6563033821ed4ae1e51890ab10479bbd`

Re-derivable by re-running `git rev-parse HEAD`, `git status --short`, `go version`,
`sop version` and a SHA-256 hash of the SOP binary path. These pinned values are
updated only by later authorized bounded remediation under CLOSE-005.

### Artifact review after the SOP run

SOP created this report and recorded CLOSE-001 `LOCAL_DONE` with a PASS gate.
The operator's artifact review corrected the reversed ahead/behind labels,
clarified the repaired binary's provenance, and added the observed SOP model
selection above. These changes affect only this authorized report; they were
not a second SOP execution or retry. The original generated report and the
single run's transcript are preserved in the operator's external scratch
directory, not claimed as committed repository evidence.
