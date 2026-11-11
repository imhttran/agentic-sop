# Proposal — dollar budget for model usage (OWASP gap 17)

**Status:** PROPOSAL, not approved. **Gap:** OWASP gap 17 in
[OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md). This gap is optional: no code is
written until you confirm a dollar cap is wanted.

## What exists today

- `SOP_AGENT_MAX_TOTAL_TOKENS` (`internal/perf/budget.go`): an opt-in, per-run token
  budget checked before each request. It is off by default, fails closed when usage cannot
  be measured, and counts tokens, not money.
- Per-model usage is already recorded: `perf.AddUsageWithReport(ctx, model, input, output,
  reported)` keeps input and output tokens by model name. A dollar figure can be built from
  this without new instrumentation.
- SOP has **no pricing data** anywhere in the codebase.

## Proposed design (if approved)

1. **Pricing comes from the operator, never from SOP.** A configuration table keyed by
   model name, with input and output price per million tokens. Nothing is fetched and no
   prices are hardcoded, because they change.
2. **Dollar budget** computed from the recorded per-model usage and that table. A new
   setting, for example `SOP_AGENT_MAX_TOTAL_USD`, opt-in and off by default.
3. **Fail closed.** A model with no price in the table is refused when the dollar budget is
   set, the same rule the token budget applies to unmeasured usage.
4. **Overshoot is bounded and stated.** As with the token budget, the request that crosses the
   budget still completes. The bound is one request's input plus `SOP_AGENT_MAX_OUTPUT_TOKENS`,
   priced at the model's rates.

## Options

- **A. Keep the token budget only.** No dollar figure. Matches the current evidence and avoids
  maintaining prices.
- **B. Add the opt-in dollar budget** with an operator-supplied price table, as above.
- **C. Also set a default token budget.** Needs a measured typical run first. This repository
  has no recorded per-run token figures to base a default on, so the default cannot be chosen
  yet.

## Recommendation

Choose **A** unless a dollar limit is needed for a specific reason. If it is, choose **B**
with a price table you maintain. Defer **C** until a typical run has been measured.

## Decision for the operator

1. Is a dollar cap wanted, or is the token budget enough?
2. If wanted: who maintains the price table, and which models must it cover?
3. Should a default token budget be set, and if so, what typical run size should it use?
