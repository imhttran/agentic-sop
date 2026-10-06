# Phase 8 --- Context & Execution Efficiency

> **Document class:** plan · **Lifecycle:** complete · **Authority:** historical — a record of completed work, not current planning authority.

**Status:** Implemented; all twelve CTX tasks reached a terminal state and the plan was
archived by SOP (`sop plan complete`, `.agent-sdlc/archive/phase-8-context-execution-efficiency/`).

## Status

``` text
PHASE 7 — AGENTIC RELIABILITY & EVALUATION
COMPLETE

PHASE 8 — CONTEXT & EXECUTION EFFICIENCY
COMPLETE
```

Phase 8 builds on the completed reliability foundation:

``` text
AGENT-001  Structured Run Trace
AGENT-002  Progress Signals
AGENT-003  Evaluation Harness
AGENT-004  Agent Budgets
AGENT-005  Replan Strategy
```

The goal is no longer primarily:

> Can SOP execute safely?

The Phase 8 question is:

> Can SOP give each model the smallest useful context needed to complete
> work successfully, with less discovery, latency, repetition, and model
> usage?

------------------------------------------------------------------------

## 1. Design Principle

The deterministic harness continues to own:

-   selection
-   retrieval
-   ranking
-   context limits
-   budgets
-   provenance
-   verification
-   evaluation

The model consumes context. The model does not control the retrieval
system.

``` text
                    TASK
                      │
                      ▼
               CONTEXT ENGINE
                      │
       ┌──────────────┼──────────────┐
       │              │              │
       ▼              ▼              ▼
  Task Context   Repository     Run Evidence
                 Context
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                CONTEXT PACKAGE
                      │
                      ▼
               PROMPT COMPILER
                      │
                      ▼
                    MODEL
                      │
                      ▼
                   EXECUTE
                      │
                      ▼
          TRACE / PROGRESS / EVAL
```

------------------------------------------------------------------------

## 2. Phase 8 Work Breakdown

``` text
CTX-001  Context Engine
CTX-002  Structural Repository Index
CTX-003  BM25 Retrieval
CTX-004  Retrieval Evaluation Gate
CTX-005  Prompt Compiler
CTX-006  Response Normalizer
CTX-007  Verification Cache
CTX-008  Prompt Result Cache
CTX-009  Vector Retrieval Evaluation
CTX-010  Decision Memory
CTX-011  Adaptive Routing
CTX-012  Automatic Prompt Tuning
```

Not all items are automatically authorized. Each major capability must
pass an evaluation gate before the next complexity layer is justified.

------------------------------------------------------------------------

## 3. CTX-001 --- Context Engine

### Objective

Introduce a canonical, deterministic representation of the context
supplied to an agent.

Do **not** implement RAG yet.\
Do **not** implement embeddings.\
Do **not** redesign prompts yet.

The Context Engine should answer:

``` text
What information does this execution need?
Where did it come from?
Why was it included?
How much was included?
```

### Candidate context model

Conceptually:

``` go
type Context struct {
    Task       TaskContext
    Repository RepositoryContext
    Execution  ExecutionContext
    Recovery   RecoveryContext
}
```

Adapt to repository conventions. Do not force this exact type.

### Task Context

Potential inputs:

``` text
task specification
plan
acceptance criteria
dependencies
current lifecycle state
capability
```

### Repository Context

Initially:

``` text
repository root
branch
relevant known files
changed files
package/module metadata
explicit task references
```

Do not perform semantic retrieval yet.

### Execution Context

Potential inputs:

``` text
selected model class
provider
capability
execution budget
attempt number
```

### Recovery Context

When applicable:

``` text
previous attempt
failure classification
verification evidence
replan reason
bounded failure context
```

AGENT-005 currently constructs replan context separately. CTX-001 should
investigate whether that becomes one consumer of the canonical Context
Engine rather than maintaining a parallel context mechanism.

Do not break AGENT-005.

------------------------------------------------------------------------

## 4. Context Provenance

Every context item should have deterministic provenance.

Conceptually:

``` text
ContextItem

source:
  task
  plan
  repository
  changed-file
  validation
  review
  recovery
  explicit-reference

path:
  internal/foo/bar.go

reason:
  task_reference

priority:
  ...

size:
  ...
```

Avoid storing secrets. Do not persist full sensitive environment values.

------------------------------------------------------------------------

## 5. Context Limits

Context must be bounded.

Do not rely on the model's context window as the safety mechanism.

Introduce deterministic limits using measurements SOP can actually own.

Initially prefer:

``` text
bytes
characters
lines
items
files
```

Do **not** introduce token budgets.

Reliable cross-provider token accounting does not currently exist.

Potential concept:

``` text
ContextBudget {
    MaxBytes
    MaxFiles
    MaxItems
}
```

Use repository conventions and avoid overengineering.

------------------------------------------------------------------------

## 6. CTX-001 Observability

Extend trace evidence enough to answer:

``` text
how many context items were supplied?
how many files?
how many bytes?
which source categories?
was anything omitted because of the context limit?
```

Do not necessarily persist full context contents. Prefer
metadata/provenance.

Example conceptual trace:

``` json
{
  "context": {
    "items": 12,
    "files": 6,
    "bytes": 18422,
    "truncated": false,
    "sources": {
      "task": 1,
      "repository": 6,
      "validation": 2,
      "recovery": 3
    }
  }
}
```

If persisted trace structure changes:

``` text
schema 4 → schema 5
```

Trace remains observational. Runtime policy must never read `trace.json`
back to make decisions.

------------------------------------------------------------------------

## 7. CTX-001 Evaluation

Extend AGENT-003 evaluation so fixtures can assert context behavior.

Useful deterministic assertions:

``` text
item count
file count
byte ceiling
truncated
source-category count
```

Add regression cases proving:

``` text
normal execution receives expected context

replan receives recovery evidence

context remains within configured bound

context ordering is deterministic

same inputs produce same context package

missing optional evidence does not fail execution
```

------------------------------------------------------------------------

## 8. CTX-002 --- Structural Repository Index

Only begin after CTX-001 is stable.

### Objective

Build a deterministic repository map that the Context Engine can query.

For Go, prioritize:

``` text
modules
packages
files
types
functions
methods
interfaces
imports
tests
```

Also index:

``` text
Markdown headings
configuration files
SOP specs/plans/docs
```

Avoid embeddings.

Prefer native Go parsing where appropriate:

``` text
go/parser
go/ast
go/token
```

The structural index should be reproducible.

``` text
same repository state
→ same index
→ same ordering
→ same identifiers
```

------------------------------------------------------------------------

## 9. Structural Index Storage

Prefer a repository-local generated artifact under the existing SOP
state hierarchy.

Conceptually:

``` text
.agent-sdlc/
    context/
        index.json
```

or another existing canonical state location.

Do not pollute tracked source files.

The index should identify the repository state it represents.

Potential identity:

``` text
HEAD
working-tree fingerprint
index schema
generated timestamp
```

Do not use timestamp as semantic identity.

------------------------------------------------------------------------

## 10. Index Invalidation

Define deterministic invalidation.

At minimum consider:

``` text
HEAD changed
tracked file changed
relevant untracked file appeared
indexed file changed
schema changed
```

Avoid rebuilding everything unnecessarily if a simple safe invalidation
mechanism exists.

Correctness beats premature incremental complexity. A full deterministic
rebuild is acceptable initially.

------------------------------------------------------------------------

## 11. CTX-003 --- BM25 Retrieval

Once structural context exists, add lexical retrieval.

Start with BM25 or an equivalent deterministic lexical ranker.

Input:

``` text
task
plan
acceptance criteria
failure evidence
```

Candidate corpus:

``` text
source symbols
source chunks
docs
tests
configuration
```

Output:

``` text
ranked context candidates
```

Every retrieval result must expose:

``` text
source
score
reason
rank
```

Tie-breaking must be deterministic. Do not allow filesystem iteration
order to affect ranking.

------------------------------------------------------------------------

## 12. Hybrid Retrieval

The first useful retrieval architecture should be:

``` text
                     TASK
                       │
                       ▼
               CONTEXT ENGINE
                       │
          ┌────────────┴────────────┐
          │                         │
          ▼                         ▼
   STRUCTURAL MATCH              BM25
          │                         │
          └────────────┬────────────┘
                       ▼
               DETERMINISTIC MERGE
                       │
                       ▼
                CONTEXT BUDGET
                       │
                       ▼
                CONTEXT PACKAGE
```

Explicit references should generally outrank inferred retrieval.

Potential priority:

``` text
1. task-explicit references
2. current changed files
3. structural relationships
4. lexical/BM25 results
5. generic repository context
```

Confirm this against repository behavior before encoding it.

------------------------------------------------------------------------

## 13. CTX-004 --- Retrieval Evaluation Gate

This is mandatory.

Do not automatically proceed from BM25 to vector RAG.

Use AGENT-001--005 evidence to compare:

``` text
BASELINE
current discovery behavior

vs

CONTEXT ENGINE
structural + BM25 retrieval
```

Measure what SOP can reliably observe.

Candidate metrics:

``` text
success/failure
model iterations
discovery progress signals
repeated discovery
tool calls
repository mutations
verification passes
replans
attempt count
elapsed wall time if reliable
context bytes
context items
```

Do not use token counts until reliable accounting exists.

------------------------------------------------------------------------

## 14. Evaluation Corpus

Create deterministic representative tasks.

Include examples requiring:

``` text
single-file implementation
multi-file implementation
existing test discovery
interface implementation
configuration change
documentation + code
bug fix
cross-package dependency
replan after verification failure
```

Do not optimize solely against one repository task.

------------------------------------------------------------------------

## 15. Retrieval Success Gate

Before adding more retrieval complexity, determine whether structural +
BM25 context materially improves execution.

``` text
Structural + BM25
        │
        ▼
Evaluation
        │
    ┌───┴────┐
    │        │
Improves   No meaningful
behavior   improvement
    │        │
    ▼        ▼
Continue    Stop / simplify
```

Do not assume vector retrieval is necessary.

------------------------------------------------------------------------

## 16. CTX-005 --- Prompt Compiler

Only begin after Context Engine and retrieval contracts stabilize.

### Objective

Replace scattered prompt construction with a canonical compiler.

Conceptually:

``` text
PromptInput {
    Capability
    Task
    Context
    Budget
    Recovery
    ModelClass
}
        │
        ▼
Prompt Compiler
        │
        ▼
Compiled Prompt
```

The compiler should own structure. The model should not determine what
instructions govern itself.

------------------------------------------------------------------------

## 17. Model-Class Compilation

The compiler may eventually produce different bounded prompts for:

``` text
SMALL
MEDIUM
LARGE
```

Example:

``` text
SMALL
→ compact instructions
→ highly targeted context
→ fewer optional docs

MEDIUM
→ broader structural context

LARGE
→ richer architecture/recovery context
```

Do not assume larger prompts improve larger models. Evaluate it.

------------------------------------------------------------------------

## 18. CTX-006 --- Response Normalizer

Introduce a deterministic normalization boundary around model output.

``` text
Provider output
      ↓
Response Normalizer
      ↓
Canonical Agent Response
      ↓
Harness
```

Normalize only what is safe and deterministic:

``` text
tool-call shape
structured fields
empty output
known wrappers
provider-specific formatting
```

Do not "repair" semantic mistakes using hidden heuristics.

------------------------------------------------------------------------

## 19. CTX-007 --- Verification Cache

This should be the first cache.

Verification is:

``` text
deterministic
expensive
repeatable
high-confidence
```

Candidate key:

``` text
repository state
+
verification command
+
relevant environment identity
```

Never return cached verification for a different repository state.

Prefer false cache misses over false cache hits.

------------------------------------------------------------------------

## 20. CTX-008 --- Prompt Result Cache

Only after Verification Cache proves the caching infrastructure.

Prompt-result caching is riskier because model output depends on:

``` text
model
provider
prompt
context
parameters
tool state
repository state
```

Require a strong cache key.

Never reuse an IMPLEMENT result merely because prompt text matches.

Start with read-only capabilities if implemented:

``` text
PLAN
REVIEW
analysis-like operations
```

Keep mutation-producing results out until correctness is proven.

------------------------------------------------------------------------

## 21. CTX-009 --- Vector Retrieval Evaluation

Only evaluate vector retrieval if CTX-004 shows a meaningful lexical
retrieval gap.

Do not start by introducing a vector database.

First define the failure:

``` text
BM25 misses semantically related code
because vocabulary differs.
```

Then build a controlled experiment:

``` text
Structural + BM25

vs

Structural + BM25 + Vector
```

Compare execution outcomes.

If vector retrieval does not materially improve results:

``` text
DO NOT SHIP IT
```

------------------------------------------------------------------------

## 22. CTX-010 --- Decision Memory

Decision Memory comes after retrieval.

It should store durable engineering decisions, not conversation history.

Good examples:

``` text
Postgres is canonical storage

controller delegates lifecycle authority to agentic-sop

NO_PROGRESS is BLOCK

SMALL uses local-first fallback

replan does not change model class
```

Bad examples:

``` text
model said X last time

agent tried command Y

temporary debug observation
```

Memory must have provenance and scope.

Potential scopes:

``` text
repository
project
architecture
```

Avoid user-global memory initially.

------------------------------------------------------------------------

## 23. CTX-011 --- Adaptive Routing

Only begin when enough evaluation evidence exists.

Adaptive routing should use deterministic historical evidence.

Potential future input:

``` text
task characteristics
context size
retrieval characteristics
historical eval outcomes
model-class success rates
```

Potential decision:

``` text
SMALL
MEDIUM
LARGE
```

Routing must remain bounded and explainable.

Every adaptive decision should answer:

``` text
Why was this class selected?
What evidence supported it?
```

Do not let the LLM select itself.

------------------------------------------------------------------------

## 24. CTX-012 --- Automatic Prompt Tuning

This is last.

Automatic prompt tuning requires:

``` text
stable evaluation corpus
stable prompt compiler
stable context engine
repeatable metrics
enough historical results
```

Without those, automatic prompt tuning is guesswork.

``` text
Prompt Version A
       │
       ▼
Evaluation corpus
       │
       ▼
Metrics
       │
       ▼
Candidate Prompt B
       │
       ▼
Evaluation corpus
       │
    ┌──┴───┐
 better   worse
   │        │
promote   reject
```

Never promote based solely on the tuning model's opinion.

------------------------------------------------------------------------

## 25. Explicitly Deferred

The following remain outside Phase 8 unless separately authorized:

``` text
JEV adapter
JEV-vs-deterministic evaluation

team/network service
small-device dashboard

distributed execution
parallel production agents

token budgets
token-based routing

automatic provider switching based on cost
automatic model escalation based on token use
```

------------------------------------------------------------------------

## 26. Token Accounting

Do not introduce token budgets yet.

Current rule remains:

> A metric must have reliable deterministic ownership before SOP uses it
> as an execution policy.

Until provider-independent accounting exists:

``` text
tokens → informational at most
tokens → NOT budget authority
```

Prefer currently reliable measurements:

``` text
bytes
items
files
iterations
tool calls
attempts
replans
verification results
```

------------------------------------------------------------------------

## 27. Phase Ordering

Recommended dependency chain:

``` text
CTX-001 Context Engine
        │
        ▼
CTX-002 Structural Index
        │
        ▼
CTX-003 BM25
        │
        ▼
CTX-004 Retrieval Evaluation Gate
        │
        ├──────────── insufficient benefit ─────→ STOP / simplify
        │
        ▼
CTX-005 Prompt Compiler
        │
        ▼
CTX-006 Response Normalizer
        │
        ▼
CTX-007 Verification Cache
        │
        ▼
CTX-008 Prompt Result Cache
        │
        ▼
CTX-009 Vector Retrieval Evaluation
        │
        ▼
CTX-010 Decision Memory
        │
        ▼
CTX-011 Adaptive Routing
        │
        ▼
CTX-012 Automatic Prompt Tuning
```

Not every box must ultimately ship. Evaluation determines whether
additional complexity is justified.

------------------------------------------------------------------------

## 28. Phase 8 Success Metrics

Phase 8 should attempt to improve:

``` text
task success rate            ↑
first-attempt success        ↑
verification success         ↑

discovery iterations         ↓
repeated discovery           ↓
model iterations             ↓
tool calls                   ↓
replans                      ↓
attempt count                ↓
wall-clock execution         ↓

context supplied             bounded
```

Do not optimize a metric at the expense of correctness. Correctness
remains the primary gate.

------------------------------------------------------------------------

## 29. Architectural Invariants

Throughout Phase 8 preserve:

``` text
model proposes
harness decides

trace observes
trace does not control

evaluation judges
evaluation does not execute

context supplies evidence
context does not grant authority

retrieval ranks information
retrieval does not alter lifecycle

cache accelerates
cache does not weaken verification

memory informs
memory does not override current repository evidence
```

Current repository evidence always outranks stale stored context.

------------------------------------------------------------------------

## 30. Recommended First Implementation

Do **not** implement all of Phase 8 in one run.

Authorize only:

``` text
CTX-001 — Context Engine
```

CTX-001 should deliver:

``` text
canonical context representation

deterministic provenance

deterministic context limits

task context

repository context from already-known evidence

execution context

recovery/replan context

trace observability

evaluation support

documentation
```

CTX-001 should **not** deliver:

``` text
structural index
BM25
vector embeddings
RAG
prompt compiler
cache
decision memory
adaptive routing
prompt tuning
```

------------------------------------------------------------------------

## 31. CTX-001 Completion Gate

CTX-001 is complete only when:

``` text
[ ] one canonical context contract exists

[ ] context has deterministic provenance
[ ] context ordering is deterministic
[ ] context size is bounded
[ ] token counting is not required

[ ] task evidence supported
[ ] repository evidence supported
[ ] execution evidence supported
[ ] recovery/replan evidence supported

[ ] model cannot expand its own context budget
[ ] context cannot grant additional authority
[ ] secrets are excluded

[ ] trace records context metadata
[ ] trace remains observational
[ ] evaluation can assert context behavior

[ ] same input produces same context package

[ ] AGENT-001 remains green
[ ] AGENT-002 remains green
[ ] AGENT-003 remains green
[ ] AGENT-004 remains green
[ ] AGENT-005 remains green

[ ] gofmt passes
[ ] go vet passes
[ ] go test passes
[ ] go test -race passes
[ ] go build passes
[ ] git diff --check passes
[ ] documentation links pass
```

On success:

``` text
CTX-001 = COMPLETE

NEXT:
CTX-002 — Structural Repository Index
```

Do not begin CTX-002 in the same implementation run.

------------------------------------------------------------------------

## 32. Phase 8 End State

If the useful portions of Phase 8 ultimately succeed:

``` text
                  AGENTIC-SOP

              CONTROL / RELIABILITY
                       │
     Trace → Progress → Eval → Budget → Replan
                       │
                       ▼
                  CONTEXT LAYER
                       │
      Structural → Retrieval → Context Engine
                       │
                       ▼
                 PROMPT COMPILER
                       │
                       ▼
                    MODEL
                       │
                       ▼
               NORMALIZED RESULT
                       │
                       ▼
                  EXECUTION
                       │
                       ▼
             VERIFICATION / CACHE
                       │
                       ▼
                TRACE + EVAL
                       │
                       ▼
            DATA-DRIVEN ROUTING
```

The end goal is not simply "more context."

The goal is:

> Give the smallest capable model the smallest sufficient context,
> execute within deterministic bounds, verify the result, and use
> measured evidence to improve future decisions.
