# RAG evaluation summary

## Mean recall@5 per run, arm, and subset

### run docs/reports/rag-eval/20261102T021652Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.3846 | 0.3846 | 0.4679 | 0.6346 |
| code | 0.0909 | 0.0909 | 0.5000 | 0.5000 |
| multi_hop | 0.2143 | 0.2143 | 0.3810 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.2500 | 0.2500 |

### run docs/reports/rag-eval/20261102T021737Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.3846 | 0.3846 | 0.4679 | 0.6346 |
| code | 0.0909 | 0.0909 | 0.5000 | 0.5000 |
| multi_hop | 0.2143 | 0.2143 | 0.3810 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.2500 | 0.2500 |

### run docs/reports/rag-eval/20261102T021822Z (status: pass)

| subset | A | B | C | D |
| --- | --- | --- | --- | --- |
| historical | 0.3846 | 0.3846 | 0.4679 | 0.6346 |
| code | 0.0909 | 0.0909 | 0.5000 | 0.5000 |
| multi_hop | 0.2143 | 0.2143 | 0.3810 | 0.5000 |
| reworded | 0.2500 | 0.2500 | 0.2500 | 0.2500 |

## §4.1 checks (run docs/reports/rag-eval/20261102T021652Z, status: pass)

- value (mean B - mean A, historical): 0.0000 (threshold >= 0.10)
- breadth (historical questions where B > A): 0 (threshold >= 5)
- code regressions (code questions where B < A): 0 (threshold <= 1)
- latency p95 ms (arm B, run docs/reports/rag-eval/20261102T021652Z): 306 (threshold <= 500)
- arm_a_identical across runs: true

## POST-HOC: not part of the §4.1 decision rule

Post-hoc full-context score (run docs/reports/rag-eval/20261102T021652Z):

| subset | mean full-context recall (arm B) |
| --- | --- |
| historical | 0.6506 |
| code | 0.5909 |
| multi_hop | 0.4881 |
| reworded | 0.4167 |

- mean distinct documents in arm B context: 8.4627
- historical improved vs arm A recall@5: 17
- historical regressed vs arm A recall@5: 0

## POST-HOC: not part of the §4.1 decision rule

Controls (run docs/reports/rag-eval/20261102T160658Z, top-k 10), mean RecallAtK(..., 10):

| subset | A | D |
| --- | --- |
| historical | 0.4327 | 0.7212 |
| code | 0.1818 | 0.6818 |
| multi_hop | 0.3214 | 0.5357 |
| reworded | 0.2500 | 0.2500 |

