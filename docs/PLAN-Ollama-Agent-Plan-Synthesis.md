# Ollama Agent PLAN Discovery and Synthesis

## Project

agentic-sop

## Summary

Fix the Ollama Agent PLAN capability so repository exploration is
bounded and always transitions into a tool-free synthesis phase.

Current PLAN runs can perform useful discovery but exhaust the iteration
budget because the model continues requesting tools instead of returning
FINAL.

Target:

``` text
PLAN
  ↓
DISCOVERY — max 8 read-only tool calls
  ↓
SYNTHESIS — tools disabled, max 2 model turns
  ↓
FINAL
```

This is a focused Ollama Agent improvement. It must not redesign the SOP
lifecycle or implement unrelated Agent Harness V2 work.

## PLANSYN001 --- Introduce PLAN Phase Model

Represent PLAN execution internally with DISCOVERY and SYNTHESIS phases.
These are Ollama Agent phases, not new SOP workflow stages.

### Acceptance Criteria

-   PLAN distinguishes DISCOVERY from SYNTHESIS
-   phase state is scoped to one PLAN invocation
-   no new SOP lifecycle state is introduced
-   command-provider contract remains unchanged
-   transition logic has unit tests

## PLANSYN002 --- Bound PLAN Discovery

Allow at most 8 PLAN discovery tool executions by default. Continue
using the existing centralized PLAN read-only tool policy. PLAN may
finish earlier by returning FINAL.

### Acceptance Criteria

-   PLAN discovery permits at most 8 tool executions
-   PLAN can finish earlier
-   existing read-only policy remains authoritative
-   mutation tools remain unavailable
-   reaching the discovery limit transitions to SYNTHESIS instead of
    immediate iteration-limit failure

## PLANSYN003 --- Add Forced Synthesis

After discovery reaches its limit without FINAL, transition to SYNTHESIS
and make repository tools unavailable.

Use an instruction equivalent to:

``` text
Exploration is complete.
Do not request any more tools.
Using only the repository context already gathered, produce the required PLAN response now.
Do not continue exploring.
Do not implement anything.
Return the exact structured PLAN response expected by SOP.
```

### Acceptance Criteria

-   discovery exhaustion enters SYNTHESIS
-   no repository tools execute during synthesis
-   gathered context remains available
-   synthesis explicitly requires the existing PLAN response contract
-   successful synthesis returns normally to SOP

## PLANSYN004 --- Bound Synthesis Turns

Allow at most 2 synthesis model turns. Tool requests during synthesis do
not execute and count against this allowance.

### Acceptance Criteria

-   synthesis maximum is 2 model turns
-   synthesis cannot return to discovery
-   tool requests are not executed
-   exhaustion returns `synthesis_limit` or equivalent
-   failure does not masquerade as the old generic discovery iteration
    limit

## PLANSYN005 --- Preserve Early Finalization

If the model returns a valid PLAN final response during discovery,
return immediately without entering synthesis.

### Acceptance Criteria

-   PLAN can finish after zero or more discovery calls
-   valid FINAL terminates immediately
-   synthesis occurs only when needed
-   trace distinguishes early FINAL from forced synthesis

## PLANSYN006 --- Handle Synthesis Tool Requests

If the model requests a tool during synthesis, deny execution and
respond with a concise correction:

``` text
Repository discovery is complete.
No additional tools are available.
Produce the required PLAN response using the context already gathered.
```

### Acceptance Criteria

-   synthesis tool requests never execute
-   repository remains unchanged
-   model receives a finalization instruction
-   repeated requests terminate at synthesis limit
-   denial is safely traced

## PLANSYN007 --- Improve Trace Output

Expose PLAN phase transitions in existing diagnostics.

Example:

``` text
PLAN DISCOVERY #1 list_files path=. [ok]
PLAN DISCOVERY #2 read_file path=internal/agent/agent.go [ok]
...
PLAN DISCOVERY #8 read_file path=internal/ollamaagent/policy.go [ok]
PLAN → SYNTHESIS
PLAN SYNTHESIS #1 final [ok]
```

### Acceptance Criteria

-   trace identifies both phases
-   transition is visible
-   successful FINAL is visible
-   denied synthesis tool requests are visible
-   termination reason is visible
-   secrets/full hidden prompts are not logged

## PLANSYN008 --- Improve Failure Diagnostics

Return phase-specific errors such as:

``` text
Ollama agent PLAN failed during synthesis.
Model: deepseek-v4.1-flash:cloud
Discovery tool calls: 8
Synthesis turns: 2
Termination: synthesis_limit
```

Do not hard-code DeepSeek as the agent identity.

### Acceptance Criteria

-   failure identifies phase
-   discovery and synthesis counts are available
-   useful termination reason is reported
-   model name is separate from agent name
-   generic `iteration limit reached` is not the only diagnostic

## PLANSYN009 --- Preserve Other Capability Policies

Keep other current capability policies unchanged:

``` text
PLAN          discovery 8 + synthesis 2
DESIGN_TESTS  12
IMPLEMENT     24
FIX           24
REVIEW        12
```

Do not add forced synthesis to IMPLEMENT/FIX in this focused plan.

### Acceptance Criteria

-   PLAN uses two-phase behavior
-   other capability limits remain unchanged
-   PLAN remains read-only
-   IMPLEMENT/FIX mutation behavior remains unchanged
-   policy remains centralized

## PLANSYN010 --- Deterministic Tests

Use fake Ollama responses; normal tests must not require live Ollama.

Cover:

1.  early PLAN completion
2.  forced synthesis success after 8 discovery calls
3.  tool request during synthesis is denied
4.  synthesis succeeds after one denied tool request
5.  synthesis exhaustion
6.  PLAN mutation denial
7.  other capability policies unchanged
8.  trace safety

### Acceptance Criteria

-   all cases have deterministic coverage
-   no live provider is required
-   existing Ollama Agent tests pass
-   tests prove synthesis cannot execute tools
-   tests prove early completion remains possible

## PLANSYN011 --- AHV2006-Shaped Integration Fixture

Add a fake-model integration fixture resembling AHV2006: several useful
repository reads/searches consume discovery, then the model synthesizes
successfully.

Do not modify real `.agent-sdlc` task state in this test.

### Acceptance Criteria

-   fixture performs realistic discovery
-   discovery transitions to synthesis
-   synthesis returns valid PLAN
-   invocation succeeds
-   no live provider is required
-   real SOP state is untouched

## PLANSYN012 --- Documentation and Validation

Document that PLAN is bounded, read-only, transitions to tool-free
synthesis, and does not own final SOP validation.

Run:

``` bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

### Acceptance Criteria

-   behavior is documented
-   documentation preserves SOP validation authority
-   all validation commands pass
-   no unrelated workflow redesign is included
-   implementation is ready for normal SOP recovery testing

## Safety Constraints

Do not:

-   delete `.agent-sdlc` or `state.db`
-   manually reset/edit AHV2006 state
-   run `sop init` as recovery
-   weaken PLAN read-only restrictions
-   allow tools during synthesis
-   remove iteration limits
-   solve this by raising another global iteration limit
-   add Claude
-   rename Ollama Agent to DeepSeek Agent
-   switch away from configurable `SOP_OLLAMA_MODEL`
-   commit, push, or merge

SOP remains the workflow authority.

## Recovery Test

After implementation and validation:

``` bash
sop status
sop task AHV2006
```

Use normal SOP recovery commands according to persisted state, then:

``` bash
sop run docs/PLAN-Agent-Harness-V2.md
```

Expected:

``` text
PLAN DISCOVERY #1 ...
...
PLAN DISCOVERY #8 ...
PLAN → SYNTHESIS
PLAN SYNTHESIS #1 final [ok]
```

instead of another discovery iteration-limit failure.

## Definition of Done

PLAN has a deterministic completion path:

``` text
DISCOVERY
  ≤ 8 read-only tool calls
       ↓
SYNTHESIS
  tools disabled
  ≤ 2 model turns
       ↓
FINAL
```

Early FINAL remains supported. Failed synthesis produces a clear
phase-specific diagnostic. AHV2006-shaped work can gather useful context
and then must synthesize rather than continuing optional exploration.
