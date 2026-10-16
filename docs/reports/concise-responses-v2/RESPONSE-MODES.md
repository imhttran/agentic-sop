# Response modes: concise default, detailed override

This report captures the **actual** `sop report` output for the five representative
cases in both response shapes, and reviews completeness. It is a presentation-shape
comparison only: it makes **no** claim about model compliance, token counts, or time
savings (those were not measured).

## What changed

- One shared writing rule lives in `internal/writing` (`writing.Rule`) and is carried
  into generated technical prompts from that single definition (the tool harness
  `systemPrompt` and the text-provider `renderPrompt`).
- `output.responses` selects the default shape of the human-facing summaries:
  `concise` (default) or `detailed`.
- `sop status --detailed` and `sop report --detailed` override the configured default
  for one invocation. Machine-readable (JSON) output is never affected.
- The concise `sop report` now retains the objective outcome verdict (the recorded
  acceptance evidence) and, in both shapes, lists the required criteria that were not
  met. Detailed adds the met criteria and the per-finding detail.

## Captured outputs

Commands were run against fixture projects with the real binary
(`sop report <id>` for concise, `sop report <id> --detailed` for detailed).

### Case 1 — success

Concise:

```text
Run: T001
Stage: PASSED
Provider: ollama
Review: self
Fix cycles: 0
Gate: PASS

Validation:
  PASS   BUILD        go build ./...
  PASS   TEST         go test ./...

Review findings: 0

Outcome:
  Status:      VERIFIED

Next action: commit when ready (`sop commit`); no further validation is required
```

Detailed:

```text
Outcome:
  Status:      VERIFIED
  met      deterministic-checks — the configured build/test checks passed (met)
```

**Difference:** detailed adds the met criterion under `Outcome:`. No failure, evidence,
or approval content differs (there is none to preserve here).

### Case 2 — test failure

Concise:

```text
Gate: FAIL

Validation:
  PASS   BUILD        go build ./...
  FAIL   TEST         go test ./...

Review findings: 1
  HIGH     1

Classification:
  Kind:        test_failure
  Disposition: retry
  Confidence:  high
  Reason:      the deterministic test check failed after the fix loop

Outcome:
  Status:      PARTIAL
  not met  criterion-1 — Targeted tests pass. (unverifiable: no deterministic check supplied)
  - required criterion criterion-1 not met

Next action: review the blocking findings and retry (`sop retry T002`)
```

Detailed adds, under `Outcome:`/`Findings (detail):`:

```text
Findings (detail):
  HIGH     internal/api/handler.go:88     nil dereference in handler
```

**Difference:** detailed adds the per-finding location line. The failed check (`FAIL
TEST`), the blocking finding count, the classification, the unmet required criterion,
and the next action are all present in the concise shape.

### Case 3 — blocked task

Concise:

```text
Gate: NEEDS_HUMAN

Classification:
  Kind:        implementation_blocked
  Disposition: needs_human
  Confidence:  medium
  Reason:      the implementation agent reported it cannot complete the task

Outcome:
  Status:      HOLD
  - no executed acceptance check

Next action: resolve the human gate (`sop approvals --json`, then `sop approve`/`sop decline T003`)
```

**Difference:** none. There is no additional detail to add (no findings, no criteria),
so concise and detailed are byte-identical. The concise shape still states the gate,
the classification reason, the HOLD verdict, and the exact next action.

### Case 4 — approval request

Concise:

```text
Gate: NEEDS_HUMAN

Next action: resolve the human gate (`sop approvals --json`, then `sop approve`/`sop decline T004`)

Approval:
  Status:      PENDING
  Kind:        NEEDS_HUMAN
  Reason:      fix-loop limit reached (3/3)
```

**Difference:** none. The pending approval request is retained in the concise shape
(it is required evidence, never treated as "detail").

### Case 5 — detailed explanation

Concise:

```text
Review findings: 3
  MEDIUM   1
  LOW      1
  INFO     1

Outcome:
  Status:      VERIFIED
```

Detailed adds:

```text
Outcome:
  Status:      VERIFIED
  met      deterministic-checks — the configured checks passed (met)

Findings (detail):
  MEDIUM   c.go:42                        error message omits the path
  LOW      a.go:10                        comment could be clearer
  INFO     b_test.go:3                    consider table-driven test
```

The same detailed output was produced with **no flag** when the project configured
`output.responses: detailed`, proving the configured default and the flag select the
same shape.

**Difference:** detailed adds the met criterion and every finding's location. The
severity counts, the validation rows, the gate, and the next action are unchanged.

## Completeness review

| Case | Concise retains | Detailed adds |
| --- | --- | --- |
| success | gate, validation rows, VERIFIED outcome, next action | met criterion |
| test failure | failing check, blocking-finding count, classification, unmet required criterion, next action | finding location |
| blocked task | gate, classification + reason, HOLD outcome, next action | (nothing) |
| approval request | gate, **pending approval request**, next action | (nothing) |
| detailed explanation | validation rows, severity counts, VERIFIED outcome, next action | met criterion + finding locations |

- **Failures, evidence, approval requests:** preserved in the concise shape for every
  case. The concise shape never drops a failed check, an unmet required criterion, the
  classification reason, or a pending approval request.
- **Structured output:** unaffected. `sop report --decisions --format json` still emits
  valid JSON with `output.responses: detailed` (covered by a focused test).
- **Not measured:** model compliance with the shared writing rule, input/output token
  counts, and elapsed time. The shared rule is a style target (roughly 80% ASD-STE100),
  not a compliance claim, and prompt-instruction presence does not prove model
  behavior. No time savings are claimed.
