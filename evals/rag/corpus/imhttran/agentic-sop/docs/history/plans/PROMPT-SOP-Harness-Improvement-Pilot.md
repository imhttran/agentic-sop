# SOP Harness Improvement Pilot

Implement this pilot in `https://github.com/imhttran/agentic-sop` and open a PR. Continue through implementation, targeted verification, evidence capture, and PR creation. Stop only for a real blocker. Do not merge the PR.

## Objective

Test whether one evidence-backed prompt or skill revision reduces agent cost or human intervention without reducing correctness. Borrow the controlled comparison approach from Harness². Keep this pilot outside the default SOP execution path.

This is a small SOP-native experiment, not a full Harness² integration or a reproduction of its benchmarks.

## 1. Inspect and reuse

- Inspect the current repository instructions, runner, skills, advisory memory, telemetry, and evidence conventions.
- Check for an existing implementation or active plan before adding code.
- Reuse current execution adapters, budgets, result records, and verification mechanisms. Do not create a second orchestrator or memory database.
- Use an isolated branch or worktree. Preserve unrelated changes.
- Record the starting commit and the implementation choices in a short plan using repository conventions.

## 2. Implement the smallest experiment

Add an opt-in script or command using the repository's existing extension pattern. Avoid a new public CLI command unless the current design requires one.

The experiment must:

1. Accept a task manifest and two versioned instruction sets: baseline and candidate.
2. Run each pair from the same starting commit and task inputs in separate disposable worktrees.
3. Use the same executor, model, settings, tool permissions, and verification commands for both arms.
4. Apply the candidate only to explicitly allowed prompt or skill files.
5. Capture completion, acceptance results, human interventions, tokens, elapsed time, failures, and evidence paths.
6. Write machine-readable results and a short Markdown comparison.
7. Resume completed comparisons only when all relevant input fingerprints match.

Keep credentials out of artifacts. Use existing sandbox controls and restrict network access to required services. Do not inherit Harness²'s sandbox-disabled execution settings. Experimental agents must not push, merge, publish, or change external state.

## 3. Run a bounded pilot

Select three completed SOP tasks that can be replayed from their pre-change commits. Choose tasks with clear acceptance checks and no required external writes. Record task selection before candidate execution.

Use prior execution evidence to propose exactly one prompt or skill revision. Keep that revision fixed across all three comparisons. Keep reference solutions and prior implementation patches outside the agent context. Acceptance checks must remain fixed and run from trusted copies.

Default budget:

- Three task pairs: six task executions total.
- One additional held-out pair only if the initial result meets promotion criteria: eight task executions maximum.
- No automatic retries or budget increases. Record failed or timed-out runs as such.
- Set explicit per-run time and token limits before execution using existing SOP controls. Do not exceed existing limits. If token enforcement is unavailable, document that limitation and enforce the execution-count and time limits.

A reuse check or dry run must not launch model calls. If supported credentials or a safe executor are unavailable, finish the implementation and open a draft PR with the exact blocker and run command. Do not fabricate results.

## 4. Decide whether to retain the candidate

Retain the candidate for review only when:

- It passes every task's fixed acceptance checks and causes no policy violations.
- Total human interventions decrease without increasing total tokens or elapsed time by more than 10%; OR total tokens or elapsed time decrease by at least 10%, with no increase in interventions and no increase above 10% in the other cost metric.
- The held-out task passes, introduces no additional human intervention, and stays within 10% of baseline tokens and elapsed time.

These are pilot decision thresholds, not proof of statistical significance. Missing telemetry makes the affected comparison inconclusive. Do not substitute missing values with zero. If no candidate qualifies, preserve the baseline and report the negative or inconclusive result. A useful pilot does not require a positive result.

Store lessons with their source evidence, input fingerprints, and outcome. Use existing advisory-memory facilities only if they support this provenance. Otherwise retain the lessons in the report. Do not automatically activate the candidate or promote unverified lessons into shared memory.

## Boundaries

- Do not modify approval rules, risk classification, acceptance criteria, security policy, or required gates.
- Do not modify model routing or add model providers for this pilot.
- Do not add a default recursive execution loop, new service, database, or full Harness² dependency.
- Do not run public benchmark suites or repeat the complete SOP CLI suite solely for this experiment.

## Verification and completion

Use focused tests for input-fingerprint invalidation, budget enforcement, prohibited edit rejection, and failure reporting. Exercise one deterministic fixture to verify report generation without model calls. Run the affected repository checks and any mandatory integration gates. Reuse passing evidence when its inputs are unchanged.

Completion requires:

- An opt-in implementation with a documented invocation and fixed limits.
- Isolated baseline/candidate execution with unchanged policy and acceptance checks.
- Results that distinguish success, failure, timeout, and inconclusive comparison.
- A report with task commits, instruction hashes, executor/model settings, limits, metrics, evidence references, and the adoption decision or blocker.
- A PR containing the implementation, focused verification results, and relevant documentation. Include a candidate instruction change only if it meets the promotion criteria; identify it explicitly for review.

Return the PR URL, a concise result, checks performed, and any remaining blocker. Do not stop after planning or scaffold creation.

## Reference

- Harness²: https://github.com/google-research/harness2
- Reviewed reference commit: `1cfe254e8d25b15f37048c3730134686abe60335`
- Relevant source: `harness2/core/recursion.py`, `harness2/substrate/base.py`, and `tests/test_workflow.py`.
- The source review did not run Harness² benchmarks. Verify current SOP integration points from the repository rather than assuming earlier project descriptions remain current.
