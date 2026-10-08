# EV-001 — Live-Path Baseline and State Verification

Read-only baseline record for the EV-CONTEXT-RUN evaluation. This report captures the
exact repository/lifecycle state and the current live context-building path so the
evaluation baseline is unambiguous. **No production change is made.**

## 1. Repository state (verbatim)

| Fact | Value | Source |
| --- | --- | --- |
| HEAD commit | `388ea56aef4b8e2976c73dc0e211069c695b620d` | `git rev-parse HEAD` |
| Branch | `main` | `git rev-parse --abbrev-ref HEAD` (tracking `origin/main`) |
| Working tree | dirty (see verbatim `git status --short --branch` below) | `git status --short --branch` |

### Working tree — `git status --short --branch` (verbatim)

```text
## main...origin/main
 M docs/specs/AGENT-PROVIDER.md
?? docs/plans/PLAN-EV-Context-Run-Evaluation.md
?? docs/reports/CONV-001-convergence-baseline.md
```

These are pre-existing, user-owned changes. They are preserved unchanged and are
recorded here only as part of the verbatim baseline. They are not reverted, discarded,
or treated as blocking. This task adds only its own report artifact.

## 2. Active SOP plan state (verbatim)

Source: `.agent-sdlc/plan.meta.json` (read-only; SOP state is never modified by this task).

```json
{
  "source": "docs/plans/PLAN-EV-Context-Run-Evaluation.md",
  "source_kind": "plan",
  "source_sha256": "e6f8a3c67d4ea5d10835de8c48260976c1bb85512763eeb959000a0b3bae5865",
  "plan_id": "plan-ev-context-run-evaluation",
  "generated_at": "2026-10-08T02:44:07.191755Z"
}
```

| Fact | Value |
| --- | --- |
| Active plan id | `plan-ev-context-run-evaluation` |
| Plan source | `docs/plans/PLAN-EV-Context-Run-Evaluation.md` |
| Source kind | `plan` |
| Source sha256 | `e6f8a3c67d4ea5d10835de8c48260976c1bb85512763eeb959000a0b3bae5865` |
| Generated at | `2026-10-08T02:44:07.191755Z` |
| Lifecycle (plan document) | `PROPOSED` (not activated; not implemented) — per the plan header |

## 3. Live context-building path with file/line anchors

The live IMPLEMENT context path is exactly the PRD-stated sequence
`runStages` → `implementContext` → `FromInputs` → `prompt.Compile`. Every hop is
anchored below. No deviation from the PRD-stated sequence was found.

| Hop | Symbol | File:line (anchor) |
| --- | --- | --- |
| 1 | `runStages` | `internal/cli/run.go:411` (func definition) |
| 2 | `implementContext(...)` call | `internal/cli/run.go:618` (invoked during the Implementing stage) |
| 2 | `implementContext` definition | `internal/cli/run.go:1787` |
| 3 | `sopctx.FromInputs(...)` call | `internal/cli/run.go:1829` (inside `implementContext`) |
| 3 | `FromInputs` definition | `internal/context/context.go:305` |
| 4 | `prompt.Compile(...)` call | `internal/cli/run.go:620` |
| 4 | `Compile` definition | `internal/prompt/compile.go:132` |

### Anchored trace (verbatim excerpts)

`internal/cli/run.go` — `runStages` definition (hop 1):

```go
// runStages performs the lifecycle for one task. ...
func runStages(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, approval failure.ApprovalBoundary, tri earlyGateResult, stdout io.Writer) (res lifeResult, err error) {
```

`internal/cli/run.go` — the context build + compile site (hops 2–4):

```go
ctxSummary = implementContext(spec, plan, rn, cfg, trouting, failureCtx, d)
...
compiled := prompt.Compile(prompt.Input{
```

`internal/cli/run.go` — `implementContext` definition and the `FromInputs` call (hops 2–3):

```go
func implementContext(spec *taskfile.Spec, plan *planner.Plan, rn *runpkg.Run, cfg config.Config, trouting *taskRouting, failureCtx string, d deps) sopctx.Context {
	...
	return sopctx.FromInputs(sopctx.Inputs{
		TaskID:       spec.ID,
		Plan:         plan.RenderMarkdown(),
		ChangedFiles: changed,
		Execution:    []sopctx.Item{execution},
		Recovery:     recovery,
		Memory:       memory,
	}, sopctx.DefaultLimits())
}
```

### Context sources populated on the live path

| Context source | Populated by `sop run`? | Anchor |
| --- | --- | --- |
| `SourceTask` (task + plan) | Yes | `run.go:1829` → `FromInputs` |
| `SourceRepository` (changed-file paths only) | Yes | `run.go:1816-1819`, `context.go:338-351` |
| `SourceExecution` (provider/model/class) | Yes | `run.go:1788-1794` (`executionContextText`) |
| `SourceRecovery` (prior attempt + validation failure) | Yes | `run.go:1796-1814` |
| `SourceMemory` (decision memory) | Opt-in | `run.go:1825-1828` (gated by `cfg.ContextEfficiency.DecisionMemory`) |
| Retrieval / index evidence (CTX-002/CTX-003) | **No** | no import; no `sopctx.Inputs` field |

## 4. Retrieval is absent from the live path (import/reference evidence)

Inspection method: (a) **import scan** of `internal/cli/run.go`'s import block, and
(b) **reference search** for `repoindex` / `retrieval` across `internal/cli/run.go`
and the files it reaches along the context path (`internal/context/context.go`,
`internal/prompt/compile.go`).

### Inspected files (with method)

| File | Inspection method | Result |
| --- | --- | --- |
| `internal/cli/run.go` | import scan + reference search | imports neither package; no reference |
| `internal/context/context.go` | import scan + reference search (reached via `sopctx.FromInputs`) | imports neither package |
| `internal/prompt/compile.go` | import scan + reference search (reached via `prompt.Compile`) | imports neither package |

### Import block of `internal/cli/run.go` (verbatim, non-context imports elided)

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/decisionmemory"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/prompt"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
	"github.com/imhttran/agentic-sop/internal/verifcache"
)
```

Neither `internal/repoindex` nor `internal/retrieval` appears in this import block.

### Reference search results

- `repoindex` in `internal/cli/run.go`: **0 matches**.
- `retrieval` in `internal/cli/run.go`: **0 matches**.
- `internal/context/context.go` (`FromInputs`, `Inputs`): **0** references to either
  package; `sopctx.Inputs` has no retrieval field.
- `internal/prompt/compile.go` (`Compile`): **0** references to either package.

### Where the retrieval packages are reached instead

The retrieval facilities are imported only by the operator commands, not by the live
run path:

- `internal/cli/index.go` — `sop index` (structural repository index, CTX-002).
- `internal/cli/retrieve.go` — `sop retrieve` (deterministic BM25, CTX-003).
- `internal/cli/gate.go` (transitively) — `sop gate retrieve|vector` (CTX-004).

The only retrieval-aware context builder is the Phase 9
`internal/orchestration/context_routing.go` (`Retrieved`), reachable via
`sop orchestrate`, not via `sop run`.

**Conclusion.** By import scan and reference search, `internal/repoindex` and
`internal/retrieval` are absent from `internal/cli/run.go`'s live context path
(`runStages` → `implementContext` → `FromInputs` → `prompt.Compile`): there is no
direct import, no reference, and no indirect reachability through the files the path
reaches.

## 5. Change scope

- No production file is created, modified, or deleted by this task.
- No `.agent-sdlc` state is created, modified, or deleted by this task.
- The only artifact added is this report (`docs/reports/ev-context-run/EV-001-live-path-baseline.md`).
- Pre-existing user-owned working-tree changes are preserved unchanged (Section 1).

## 6. Path-deviation note

No deviation from the PRD-stated sequence was found; the live path is exactly
`runStages` → `implementContext` → `FromInputs` → `prompt.Compile`.
