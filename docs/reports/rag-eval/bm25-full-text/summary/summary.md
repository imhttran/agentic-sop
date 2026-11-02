# RAG evaluation summary

## Mean recall@5 per run, arm, and subset

### run docs/reports/rag-eval/bm25-full-text/20261102T204448Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.5865 | 0.5865 | 0.0000 | 0.6346 |
| code | 0.5000 | 0.5000 | 0.0000 | 0.5000 |
| multi_hop | 0.4643 | 0.4643 | 0.0000 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.0000 | 0.2500 |

### run docs/reports/rag-eval/bm25-full-text/20261102T204453Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.5865 | 0.5865 | 0.0000 | 0.6346 |
| code | 0.5000 | 0.5000 | 0.0000 | 0.5000 |
| multi_hop | 0.4643 | 0.4643 | 0.0000 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.0000 | 0.2500 |

### run docs/reports/rag-eval/bm25-full-text/20261102T204459Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.5865 | 0.5865 | 0.0000 | 0.6346 |
| code | 0.5000 | 0.5000 | 0.0000 | 0.5000 |
| multi_hop | 0.4643 | 0.4643 | 0.0000 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.0000 | 0.2500 |

## §4.1 checks (run docs/reports/rag-eval/bm25-full-text/20261102T204448Z, status: pass)

- value (mean B - mean A, historical): 0.0000 (threshold >= 0.10)
- breadth (historical questions where B > A): 0 (threshold >= 5)
- code regressions (code questions where B < A): 0 (threshold <= 1)
- latency p95 ms (arm B, run docs/reports/rag-eval/bm25-full-text/20261102T204448Z): 32 (threshold <= 500)
- arm_a_identical across runs: true

## POST-HOC: not part of the §4.1 decision rule

Post-hoc full-context score (run docs/reports/rag-eval/bm25-full-text/20261102T204448Z):

| subset | mean full-context recall (arm B) |
| --- | --- |
| historical | 0.5865 |
| code | 0.5000 |
| multi_hop | 0.4643 |
| reworded | 0.2500 |

- mean distinct documents in arm B context: 5.0000
- historical improved vs arm A recall@5: 0
- historical regressed vs arm A recall@5: 0

## POST-HOC: not part of the §4.1 decision rule

Controls (run docs/reports/rag-eval/bm25-full-text/20261102T204504Z, top-k 10), mean RecallAtK(..., 10):

| subset | A | D |
| --- | --- |
| historical | 0.6731 | 0.7212 |
| code | 0.6818 | 0.6818 |
| multi_hop | 0.5000 | 0.5357 |
| reworded | 0.2500 | 0.2500 |

