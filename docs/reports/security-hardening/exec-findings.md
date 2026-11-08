# SEC-H-002/SEC-H-003 — SAST coverage and exec-finding adjudication

Status: evidence record. No production code changed in SEC-H-002 or SEC-H-003.

## Coverage change (SEC-H-002)

- Before: semgrep `PARTIAL`; 746 in-scope files skipped, all `*_test.go`, by
  Semgrep's built-in default `.semgrepignore`.
- Change: repository `.semgrepignore` containing only `:include .gitignore`.
  This overrides the scanner default without adding `security.exclude` scope.
- After: semgrep `COMPLETE`; coverage `sast: covered`, `secrets: covered`.
  `dependencies` was `failed` at that point because the OSV database was absent;
  it became `covered` after authorized provisioning (see `final.md`).
- Scan duration: 7332 ms test-inclusive versus 5917 ms before (+1.4 s), inside the
  `<5%` target for multi-minute tasks.

## Trust-boundary adjudication (SEC-H-003)

Production sites (verified by reading the call path):

- `internal/cli/outcome.go:720` — `runAcceptanceArgv` executes an argv that has
  already passed `authorizeAcceptanceCommand` (`commandpolicy.Classify == SAFE`)
  and `rejectDangerousForm` (no `go -exec/-toolexec/-vettool/-o/-overlay/-modfile`,
  no `go fmt`, no `git -c/--config-env`). No shell is invoked; output is redacted.
- `internal/decisionadapter/providers/julia/runner.go:231` — the command is
  operator-configured local-runner argv, not model output. Model input is JSON on
  stdin and `childEnv()` scrubs the environment. No shell.
- `internal/dqharness/replay_exec.go:273` — this is the sandbox launcher itself:
  `iso.Tool` is the detected bwrap/sandbox-exec backend and args come from
  `isolationCommand`, with a scrubbed environment and fail-closed behavior when no
  backend is available.

Test sites: every finding listed below is test-only harness code (re-exec of
`os.Args[0]`, a freshly built test binary, `go build/test`, `psql` with test SQL, or a
shell-script wrapper). argv is test-controlled, no external input is
shell-interpolated, and none of these paths ships in the product.

## Reviewed baseline entries added

All 23 entries use owner `imhttran` and `expires_at 2027-06-30T00:00:00Z`.
Fingerprints were copied from the scan artifact; none was computed or guessed.

| # | Path:line | Class | Fingerprint |
| --- | --- | --- | --- |
| 1 | `internal/cli/outcome.go:720` | production | `semgrep:82f8cbd035e31b4adffe1f201507e29781b9626f8acb040142ad16ee46216576` |
| 2 | `internal/cli/security_integration_test.go:38` | test | `semgrep:646030f7bae4ee14cc2e73d33132185357a410680765da8cd26af4ff44dd9672` |
| 3 | `internal/controller/web/activity_isolation_test.go:134` | test | `semgrep:2a0c308fff65f585f0c87f70dd8cc18e5cfac74b3aae96f8081cd5f561594877` |
| 4 | `internal/decision/commandprofile/connector_test.go:211` | test | `semgrep:d08a5b11bcb6fcfe0d5d9392b5838974435ef6ab0475fe2edf94647cb33e3ca8` |
| 5 | `internal/decisionadapter/providers/julia/runner.go:231` | production | `semgrep:11fb9bdc2aa5607323d07d686df44459f31e57e5e91c0745e3d21a8fb779672d` |
| 6 | `internal/dist/install_test.go:162` | test | `semgrep:9d2c56df8e0cf756aa641c7dfb0f994233532e3bf28e38115b67c232544a910c` |
| 7 | `internal/dist/install_test.go:287` | test | `semgrep:dfb5a41e147db8e1f06ce68a644f3dc7b041438cb682e81b388792bac43a2f68` |
| 8 | `internal/dist/install_windows_test.go:120` | test | `semgrep:e7e2004595afb68157dc5b550c0618c680f74252ecb443a7b4ddd6cdbf54b950` |
| 9 | `internal/dist/install_windows_test.go:138` | test | `semgrep:3d0b4685d096087b3b59ab23541c5e5f9dcd48f23fc93ed8d570486d15a9ae7a` |
| 10 | `internal/dist/install_windows_test.go:217` | test | `semgrep:8d96015da9bb46c6ab5e6aa227b43989bfb723a9f68f330a27cb435c008e4d17` |
| 11 | `internal/dist/install_windows_test.go:256` | test | `semgrep:96c2e71d40648d498bbac6bcaa7f9a08231eae5655ca11c427fbc33c75a54361` |
| 12 | `internal/dist/install_windows_test.go:503` | test | `semgrep:15d3468a59632f5e1885a803ba895f555f3a02593613e805fa85b3319d17ad26` |
| 13 | `internal/dqharness/replay_exec.go:273` | production | `semgrep:90576e4e8d83cebe41710b35562e65f7d78cccbd10cccf1bc41c38c47f66c8ef` |
| 14 | `internal/projectdetect/exec_test.go:56` | test | `semgrep:a88ec564a5cc7c03305f1564e87c3603301621f83bdcef160145a70865ccf67a` |
| 15 | `internal/projectlock/lock_test.go:133` | test | `semgrep:095dbc1e0b93a34d33731af8de20e0507b66a0de5290cd8ac28d132b801ccdb0` |
| 16 | `internal/question/atomic_test.go:178` | test | `semgrep:33d5b001c8fb68fc1ccbe4c9fa0c00da8d65c51bb4e03fe7208d5fc845be98ab` |
| 17 | `internal/rageval/ragacceptance_test.go:129` | test | `semgrep:5c2bcef653cbc3f55129d8989f9a7db8ed1a39dfaab3b6a28c36bbf192bbc832` |
| 18 | `internal/security/integration_test.go:55` | test | `semgrep:15463d4514fdc179b3802cdeace17427bd3c223f8c52f0a353fce9f954afd7f1` |
| 19 | `internal/security/integration_test.go:541` | test | `semgrep:cc82e3fddd89b24940d41d6e1a2ae05fe6060ce667ac8438d489b7d7170c897d` |
| 20 | `internal/security/integration_test.go:677` | test | `semgrep:b320f0fa98c26b04a6b038147f28f974d7cc18fa398c5e49a32592de66444aee` |
| 21 | `internal/testfast/testfast_test.go:72` | test | `semgrep:b3e531be8cb9a677dfb2276bd87f3d0b9a6167adca74bd8028ae6d7b1b012665` |
| 22 | `internal/testfast/testfast_test.go:355` | test | `semgrep:d44ed3ba4e2bfde9b9b9f45c247f14ab716a3064027264c6c23cf79c623055d8` |
| 23 | `internal/toolharness/import_sequence_test.go:50` | test | `semgrep:4e5da4d6d0084bfed3bd97702df35aced7457bbd2f987e1771ece46e29c1cae6` |

## Verification

- Post-change scan `security-20261108T172156Z`: findings 67, suppressed 53, unsuppressed 14, blocking 0.
- Verdict `INDETERMINATE`, coverage `UNKNOWN` at this stage: driven by
  `osv-scanner FAILED` (dependency prerequisite), not by the exec findings. After
  authorized OSV provisioning the verdict is `PASS` / `COMPLETE` (see `final.md`).
- No severity, coverage, or scope setting changed. No unrelated finding changed
  suppression state; the suppression is exact-fingerprint.

## Remaining advisory findings (not suppressed)

14 MEDIUM findings remain unsuppressed and advisory because
`quality.fail_on` is `[critical, high]`:

| Rule | Path:line |
| --- | --- |
| `no-direct-write-to-responsewriter` | `internal/agent/openaicompatible_execution_test.go:44` |
| `no-direct-write-to-responsewriter` | `internal/agent/provider_test.go:199` |
| `no-direct-write-to-responsewriter` | `internal/agent/provider_test.go:291` |
| `no-direct-write-to-responsewriter` | `internal/cli/ollama_e2e_test.go:41` |
| `no-direct-write-to-responsewriter` | `internal/cli/provider_validation_task_test.go:39` |
| `unsafe-reflect-by-name` | `internal/cli/zz_operator_acceptance_acc008_test.go:158` |
| `cookie-missing-httponly` | `internal/controller/web/recovery_test.go:474` |
| `cookie-missing-secure` | `internal/controller/web/recovery_test.go:474` |
| `no-fprintf-to-responsewriter` | `internal/decisionadapter/providers/clef/chat_test.go:39` |
| `no-io-writestring-to-responsewriter` | `internal/decisionadapter/providers/nimble/score_test.go:77` |
| `no-io-writestring-to-responsewriter` | `internal/ollamaagent/ollama_retry_test.go:70` |
| `no-io-writestring-to-responsewriter` | `internal/ollamaagent/ollama_retry_test.go:73` |
| `no-io-writestring-to-responsewriter` | `internal/provider/openai/provider_locality_test.go:24` |
| `string-formatted-query` | `internal/store/notifications_test.go:40` |

