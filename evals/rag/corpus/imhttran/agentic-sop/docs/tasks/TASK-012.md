# T012 --- Open Code Review Adapter

## Status

DONE

## Objective

Add an independent, **optional** review provider so a project can use Open Code
Review (or any external reviewer) in addition to the internal/Ponytail review,
without making it mandatory.

``` text
ReviewProvider
  ├── Internal / Ponytail   (T011, agent REVIEW capability)
  └── OpenCodeReview        (configured command)
```

The adapter captures structured findings and hands them to the same T011
policy (`Report.Blocking` + bounded `Loop`). SOP still decides pass/fail.

## Dependencies

- T011 --- structured review boundary

## Scope

Extend `internal/review`:

- make `Request` JSON-serialisable;
- add a command-backed `OCRProvider` that runs a trusted configured command
  (`SOP_REVIEW_COMMAND`), sends the review request as JSON on stdin, and parses
  the findings JSON on stdout with the existing deterministic `parseReport`;
- add `ProviderFromEnv` that returns the configured external provider, or an
  error when unset, so an orchestrator can fall back to the internal provider.

## Rules

- The command is **trusted local configuration**; request data goes through
  stdin, never command interpolation.
- Missing configuration is not an error for the orchestrator: it simply means
  "no external provider configured".
- Malformed external output is an error, never a silent pass.
- No network/model/ocr binary is required by tests.

## Tests

Use a fake local command (e.g. `printf`). Cover: valid findings parsed;
malformed output → error; command failure → error; missing configuration →
error from `ProviderFromEnv`; configured `ProviderFromEnv` returns a provider;
the report integrates with the T011 `Blocking` policy.

## Acceptance Criteria

- [x] An external (OCR) review provider exists behind `review.Provider`.
- [x] It is configured via a trusted command environment variable.
- [x] Findings are parsed deterministically; malformed output errors.
- [x] OCR is optional (unset configuration is a normal, non-fatal state).
- [x] Request data is passed via stdin, not shell interpolation.
- [x] Tests require no network or real ocr binary.
- [x] `make check` passes.

## Git

Branch: `task/T012-ocr-adapter`
Commit: `task(T012): add open code review adapter`
PR: `[Task T012] Add Open Code Review adapter`

## Out of Scope

CI/PR/GitHub integration; changing the internal review semantics.
