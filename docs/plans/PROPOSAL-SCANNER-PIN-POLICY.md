# Proposal — scanner pin policy (OWASP gap 9, policy items)

**Status:** PROPOSAL, not approved. **Gap:** OWASP gap 9 in
[OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md). The hash-then-exec race is closed
in code (see the `pinnedExecutor` change). This proposal covers the policy items that
need an operator decision.

## Items

1. **Committed per-platform binary pins.** `binary_sha256` is opt-in and none are
   committed. CI already pins scanner archives by SHA-256. The binary pin would need one
   digest per platform and version. Proposal: commit pins for the scanners CI already
   pins, on linux and darwin, and update them with the scanner version.
2. **Publisher signatures.** Not checked today. Verifying them would add a signature
   tool or library as a dependency. Proposal: defer. A digest proves the bytes, and the
   pin already records what was approved; signatures add trust in a publisher, which
   needs a separate decision and a new dependency.
3. **Report-only mode and empty `required_categories`.** The repository runs security
   scans in report-only mode with no required categories, so a failing scan does not
   block a change. Proposal: run one baseline with pins committed, record its findings,
   then choose which categories become required. Requiring them is a policy change for
   the operator.

## Recommendation

Do 1 next, after the scanner versions are settled. Defer 2. Make 3 a separate decision
taken on the baseline from 1.

## Decision for the operator

- Approve 1 (committed per-platform pins) and the baseline run it needs.
- Confirm 2 stays deferred, or approve the dependency it would add.
- Choose the categories to require after the baseline, or keep report-only.
