# Performance measurements (MIG-011)

Toolchain/hardware: `go1.27.1 darwin/arm64`, Apple-silicon laptop, local SSD.
Fixture: the current checkout; measurements taken at HEAD `54aa6a1a` + the
consolidation working tree.

## Binary size

| Binary | Bytes | Notes |
| --- | --- | --- |
| Baseline `sop` (pre-consolidation, sha256 `c5551414…`, `sop dev`) | 20,018,530 | core CLI only |
| Candidate unified `sop` (sha256 `6c4febfa…`) | 25,404,114 | **+5,385,584 B (+26.9 %)** — now embeds the controller (web + assets + templates/static) and the decision adapter |
| Candidate `sop-controller` wrapper | 20,002,530 | legacy wrapper |
| Candidate `sop-decision-adapter` wrapper | 9,858,082 | legacy wrapper |

Explanation: consolidation trades a larger unified binary for a single artifact that
serves `sop run`, `sop controller serve` and `sop decision`. A project that ships only
one of the components can install the smaller wrapper instead. This is an intentional,
explained operational tradeoff, not an unexplained regression.

## Startup latency

`sop version` wall time: baseline **< 10 ms**, candidate **< 10 ms** (three runs each;
the shell `time` resolution of `0.00 s` is the limit — no measurable difference).

## Discovery behaviour

Unchanged: the controller's project discovery is an explicit allowlist
(`SOP_CONTROLLER_PROJECTS` + `SOP_CONTROLLER_WORKSPACES` with a bounded
`SOP_CONTROLLER_DISCOVERY_DEPTH`), never the wider filesystem. Observed: 1 project
discovered from the disposable fixture.

## Decision failover latency

- Fixture behaviour: **PASS** (primary serves; backup on primary failure; a
  cancelled/deadline primary does not attempt the backup, so the whole consultation
  stays within the bounded per-provider timeout).
- **Wall-clock failover latency against live providers: BLOCKED** — not measured;
  the live oMLX/Bespoke services were not probed.

## Summary

Size regression: **+27 % on the unified binary** (explained). Startup, discovery and
fixture failover: no measured regression. Live provider latency: **BLOCKED** (not
measured) — not claimed as a pass.
