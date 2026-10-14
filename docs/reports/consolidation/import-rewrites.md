# Import rewrites — migration-owned controller files (MIG-003B)

Copying and rewriting are separate steps. SOP's `copy_file` imported these files
byte-for-byte; this manifest records the deterministic rewrite applied afterwards.

Approved source mapping (MIG-002 `source-map.md`): controller `internal/config` →
`internal/controller/config/` in the unified module `github.com/imhttran/agentic-sop`.

Exact rewrite (applied to every occurrence in the files below):

    "sop-controller/internal/config"  ->  "github.com/imhttran/agentic-sop/internal/controller/config"

| File                                                   | Before                             | After                                                          |
| ------------------------------------------------------ | ---------------------------------- | -------------------------------------------------------------- |
| internal/controller/sopclient/approvals_wiring_test.go | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/commands_test.go         | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/boundary_test.go         | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/discovery_test.go        | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/changed_task_test.go     | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/decision_test.go         | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/store.go                 | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/accept_changed_test.go   | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |
| internal/controller/sopclient/approvals_client_test.go | `"sop-controller/internal/config"` | `"github.com/imhttran/agentic-sop/internal/controller/config"` |

Verification: every imported controller file matches its sibling source byte-for-byte
except the single import rewrite above. `internal/controller/config` (4 files) and 28 of
37 `sopclient` files are byte-identical; 9 `sopclient` files differ only by this rewrite.
No other module-path references exist.

Deterministic check (operator, not an agent assertion): applying this exact rewrite to
each sibling source file and comparing byte-for-byte to the destination yields
**41/41 equal, 0 mismatches** (4 `config` + 37 `sopclient`), against a sibling source of
41 files. The import is therefore source-equivalent modulo the documented rewrite.

---

## Deterministic staging of the remaining imports (operator, verified by SOP)

Copied from the read-only sibling working trees (local deltas preserved) with the
approved source mapping; only the exact documented import rewrites applied.

| Source                                                                  | Destination                            | Files | Rewrite                                                                                                                                                                                                   |
| ----------------------------------------------------------------------- | -------------------------------------- | ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| sop-decision-adapters `decision/`                                       | `decision/`                            | 7     | none (stdlib-only, byte-exact)                                                                                                                                                                            |
| sop-decision-adapters `tools/julia/`                                    | `tools/julia/`                         | 12    | none (byte-exact)                                                                                                                                                                                         |
| sop-decision-adapters `internal/providers/{clef,nimble,julia,fallback}` | `internal/decisionadapter/providers/…` | 38    | `github.com/imhttran/sop-decision-adapters/decision` → `github.com/imhttran/agentic-sop/decision` (30×)                                                                                                   |
| sop-decision-adapters `internal/shadow/`                                | `internal/decisionadapter/shadow/`     | 5     | same (included in the 30)                                                                                                                                                                                 |
| sop-decision-adapters `tests/fixtures/`                                 | `internal/tests/fixtures/`             | 12    | none (placed so the unmodified `../../../tests/fixtures/…` path in `providers/nimble/golden_test.go` resolves)                                                                                            |
| sop-controller `internal/web/`                                          | `internal/controller/web/`             | 32    | `sop-controller/internal/config`→`…/internal/controller/config` (9×); `sop-controller/internal/sopclient`→`…/internal/controller/sopclient` (17×); `"sop-controller"`→`…/internal/controller/assets` (8×) |
| sop-controller `assets.go` + `templates/` + `static/`                   | `internal/controller/assets/`          | 34    | none                                                                                                                                                                                                      |

Deterministic check: applying the documented rewrites to each sibling source file and
comparing byte-for-byte to the destination yields **equal for every file** (7 + 12 + 43 +
12 + 32); `assets` (34 files) byte-identical. Sibling working trees unchanged.

### One documented deviation

`internal/decisionadapter/providers/clef/config_test.go` `TestConfigFromEnvDefaults` now
also clears `CLEF_ENABLED` and `CLEF_TRANSPORT`, so the operator `.env` cannot defeat its
"defaults" assertion. Same env-isolation convention as the core fixes; it is the only
staged file that differs from source beyond the import rewrites.

### Recorded as external completions verified by SOP

`sop task complete MIG-003/MIG-003A/MIG-003C --external` — each required passing
validation (BUILD/UNIT_TEST/LINT PASS) and was recorded `LOCAL_DONE`. The controller
import (MIG-003B) reached `LOCAL_DONE` via the `ALREADY_SATISFIED` path; MIG-003A/003B/003C/
003D were verified by SOP, not asserted.

---

## Independent equivalence re-check (operator, 2026-10-14)

Re-derived by applying the documented rewrites to each sibling source file and comparing
byte-for-byte with the destination. Result: **TOTAL=181 EQUAL=177 MISMATCH=4**. The four
mismatches are all deliberate, documented deviations from a byte-exact import (none is an
accidental drift):

| File                                                     | Deviation                                                                                   | Rationale                                                                               |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `internal/decisionadapter/providers/clef/config_test.go` | also clears `CLEF_ENABLED`/`CLEF_TRANSPORT`                                                 | env-isolation convention (see above)                                                    |
| `internal/controller/config/config.go`                   | adds `ResolveSOPBin()`, default `SOPBin` = running executable, `SOP_BIN` wins               | required by MIG-005 ("Default SOP command path to the current binary; respect SOP_BIN") |
| `internal/controller/sopclient/start.go`                 | `StartOrContinue` dispatches `sop run` for a started-but-incomplete plan (was `sop resume`) | MIG-011 fix; `resume` is read-only inspection, never execution                          |
| `internal/controller/web/dogfood_test.go`                | expectations updated to the `run` argv                                                      | pinned to the `start.go` fix above                                                      |

Everything else under `decision/`, `internal/decisionadapter/`, `internal/controller/`,
`internal/tests/fixtures/` and `tools/julia/` matches its sibling source byte-for-byte after
the single documented import-path rewrite per tree.

### License attribution and provenance (verified)

- `sop-decision-adapters` is **Apache-2.0** (`LICENSE` + `README` "## License"). The unified
  repo is also Apache-2.0, so the imported adapter files retain equivalent terms. No per-file
  license header exists in the source, so none is dropped by the import.
- `sop-controller` has **no `LICENSE` file, no per-file header and no README license
  statement** — its license is **UNKNOWN**. Per `source-map.md` §5 it is recorded as
  attribution-pending, not assumed permissive; the operator owns this determination before any
  public distribution. No controller source carries a header that the import could drop.
- Source provenance (repo, path and tip) for every import is pinned in `source-map.md` §2.1 and
  in the import table above.
