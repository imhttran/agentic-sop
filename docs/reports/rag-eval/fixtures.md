# RAG-EVAL-003 — End-to-end failure fixtures

Operational matrix from `docs/specs/RAG-EVALUATION.md` §5 and the decision report
§4, recorded at the RAG layer of SOP's real run path. Each fixture runs SOP's
`run` command in-process through the `internal/cli` test harness
(`runInjectedCLI`), with the real `ragclient` executing a producer executable.
Only the agent is faked. Producers are `#!/bin/sh` scripts that read one request
line and print one `rag-retrieval/v1` JSON response or exit non-zero.

## Cases

| Case | Failure | Scripted or real | Asserted outcome |
| --- | --- | --- | --- |
| `TestRAGFixtureColdStart` | None; producer returns status OK with no evidence | scripted | Optional task exits OK; `used_in_context` false |
| `TestRAGFixtureStaleIndex` | Producer returns status ERROR, error_code STALE_INDEX | scripted | Exit OK; telemetry status failed, reason_code OPTIONAL_FALLBACK, `used_in_context` false |
| `TestRAGFixtureEmbeddingProviderUnavailable` | Producer returns status ERROR, error_code UNAVAILABLE (stands in for Ollama down) | scripted | Same as StaleIndex |
| `TestRAGFixturePostgresUnavailable` | Producer really runs `psql` against `postgres://rag:rag@127.0.0.1:1/rag?sslmode=disable` (nothing listening) and exits with psql's status | real | Same as StaleIndex |
| `TestRAGFixtureMalformedProducerResponse` | Producer prints `not json` | scripted | Same as StaleIndex |
| `TestRAGFixtureCrossRepositoryScope` | Producer returns one evidence item whose corpus is `other` (outside the allowlist) | scripted | Same as StaleIndex; no `other` item reaches the agent's prompt |
| `TestRAGFixtureRequiredEvidenceBlocks` | Producer returns status ERROR, error_code UNAVAILABLE on a required-evidence task | scripted | Task blocks with `REQUIRED_EVIDENCE_MISSING`; the agent is not invoked |
| `TestRAGFixtureResumedRun` | StaleIndex producer run twice in the same project directory | scripted | Both runs exit OK and record the same telemetry status and reason_code |

All eight tests live in `internal/cli/rag_fixtures_test.go` under the
`ragacceptance` build tag and pass with
`go test -count=1 -tags ragacceptance -run TestRAGFixture ./internal/cli/`.
