# Agent Harness V2

## Project

agentic-sop

## Summary

Separate the concepts of agent harness, model provider, and model in
`agentic-sop`, then add a tool-enabled coding-agent path using Ollama
with `deepseek-v4.1-flash:cloud`.

SOP remains the deterministic workflow authority. Existing command-agent
configurations remain backward compatible.

## Goal

Separate the concepts of **agent harness**, **model provider**, and
**model** in `agentic-sop`, then make Ollama with
`deepseek-v4.1-flash:cloud` the preferred/default tool-enabled
coding-agent path.

The implementation must preserve SOP as the deterministic workflow
authority.

## AHV2001 --- Correct Provider Capabilities

Fix the current capability mismatch so native text-only providers do not
advertise repository-mutating capabilities they cannot actually perform.

### Acceptance Criteria

-   capability declarations reflect real execution ability
-   text generation is distinguished from repository mutation
-   unsupported IMPLEMENT/FIX requests fail clearly
-   no silent fallback
-   existing command provider behavior remains compatible

## AHV2002 --- Introduce Harness Abstraction

Introduce an explicit harness/execution layer separate from model
providers.

Conceptually:

``` go
type Harness interface {
    Execute(context.Context, Request) (Response, error)
}
```

Responsibilities:

``` text
Provider -> model inference
Harness  -> engineering execution + controlled tools
SOP      -> workflow authority
```

Do not move task scheduling, retries, validation gates, review gates, or
state transitions into the harness.

### Acceptance Criteria

-   harness abstraction exists
-   provider and harness responsibilities are separated
-   SOP remains the sole workflow authority
-   existing command-provider behavior continues working

## AHV2003 --- Extend Configuration

Support explicit configuration of harness, provider, and model.

Target:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

Precedence:

``` text
environment
  ↓
project configuration
  ↓
built-in defaults
```

Preserve compatibility with:

``` yaml
agent:
  provider: command
```

### Acceptance Criteria

-   config supports harness, provider, and model
-   environment overrides retain highest precedence
-   legacy command configuration remains valid
-   unknown harness values fail clearly

## AHV2004 --- Implement Controlled Tool Harness

Create a tool-enabled harness capable of performing IMPLEMENT, FIX, and
DESIGN_TESTS.

Initial tools:

``` text
read_file
write_file
create_file
list_files
search_files
run_command
git_status
git_diff
```

Reuse existing SOP command/security policy where practical. Protect
`.agent-sdlc/state.db` and deny destructive Git operations unless an
existing SOP-controlled workflow explicitly authorizes them.

### Acceptance Criteria

-   model can read and modify repository files through controlled tools
-   command execution is policy checked
-   `.agent-sdlc/state.db` is protected
-   destructive Git operations are denied
-   tool execution is auditable

## AHV2005 --- Implement Agent Tool Loop

Implement a bounded model/tool execution loop:

``` text
SOP Request
  ↓
Harness
  ↓
Model
  ↓
tool request
  ↓
policy check
  ↓
execute tool
  ↓
tool result
  ↓
Model
  ↓
structured outcome
```

Bound maximum tool calls, iterations, execution timeout, and output
size.

### Acceptance Criteria

-   tool loop has explicit iteration limits
-   tool calls have timeouts
-   malformed tool calls fail safely
-   model cannot create an unbounded loop

## AHV2006 --- Ollama Tool Integration

Extend the Ollama integration to support the tool-enabled harness.

Preferred model:

``` text
deepseek-v4.1-flash:cloud
```

Clearly report Ollama unavailable, model unavailable, unsupported tool
calling, malformed tool requests, timeouts, and empty responses. Do not
silently substitute another provider or model.

### Acceptance Criteria

-   Ollama participates in the tool harness
-   DeepSeek model selection is explicit
-   provider/model failures are actionable
-   no silent fallback occurs

## AHV2007 --- Structured Execution Outcome

The tool harness must produce SOP's existing structured execution
outcome:

``` json
{"status":"completed","summary":"Implemented change","changes_expected":true}
```

``` json
{"status":"completed","summary":"Verification completed","changes_expected":false}
```

``` json
{"status":"needs_human","reason":"Human authorization required"}
```

``` json
{"status":"failed","reason":"Unable to complete implementation"}
```

SOP must independently validate the repository after the model reports
completion.

### Acceptance Criteria

-   IMPLEMENT/FIX results use existing structured outcomes
-   changes_expected reflects repository reality
-   SOP validation still runs independently
-   model output cannot bypass gates

## AHV2008 --- Default Selection

Only after the tool harness is proven functional, change new-project
defaults to:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

Update `config.Default()`, `config.applyDefaults()`,
`config.Template()`, and tests.

Existing projects explicitly configured with `provider: command` must
continue using the command path.

### Acceptance Criteria

-   new projects default to tool + Ollama + DeepSeek
-   explicit command configurations remain unchanged
-   template and defaults agree
-   tests cover precedence and migration behavior

## AHV2009 --- Runtime Visibility

Show the actual execution stack at startup.

Example:

``` text
Project: sop-controller
Harness: tool
Provider: ollama
Model: deepseek-v4.1-flash:cloud
Provider source: configuration
```

For command:

``` text
Harness: command
Provider: command
Command: scripts/sop-agent.sh
```

### Acceptance Criteria

-   runtime output shows harness
-   runtime output shows provider
-   runtime output shows model when applicable
-   source of effective configuration is visible

## AHV2010 --- Preserve Command/Claude Compatibility

Keep the existing command-agent path working:

``` text
SOP
 ↓
command
 ↓
scripts/sop-agent.sh
 ↓
Claude Code / other external implementation agent
```

Claude must remain optional rather than an architectural dependency.

### Acceptance Criteria

-   command provider remains supported
-   SOP_AGENT_COMMAND remains supported
-   existing command adapters still work
-   Claude remains optional

## AHV2011 --- Tests

Add deterministic tests for:

1.  harness/provider/model parsing
2.  environment override precedence
3.  legacy command configuration
4.  Ollama capability boundaries
5.  tool harness IMPLEMENT
6.  tool harness FIX
7.  DESIGN_TESTS
8.  read/write tools
9.  command policy enforcement
10. destructive Git denial
11. `.agent-sdlc/state.db` protection
12. bounded tool loops
13. timeout handling
14. malformed tool calls
15. malformed model responses
16. structured outcomes
17. changes_expected behavior
18. validation after implementation
19. retry behavior
20. needs_human behavior
21. provider/model visibility
22. unavailable Ollama
23. unavailable model

Use fake providers and tool executors for unit tests. Normal tests must
not require a live Ollama server.

### Acceptance Criteria

-   deterministic tests cover harness, provider, tool, policy, outcome,
    retry, and visibility behavior
-   normal unit tests require no live Ollama service
-   external dependencies are represented by fakes in the normal test
    suite
-   legacy command-provider behavior remains covered
-   the complete normal test suite passes

## AHV2012 --- End-to-End Ollama Dogfood

Add an opt-in integration test or documented dogfood procedure using:

``` text
ollama
deepseek-v4.1-flash:cloud
```

Use a small disposable fixture repository and verify:

``` text
read existing file
→ modify code
→ create test
→ run test
→ inspect failure if applicable
→ fix
→ return structured outcome
→ SOP validation
→ PASS
```

Do not run destructive tests against `agentic-sop` itself.

### Acceptance Criteria

-   an opt-in Ollama/DeepSeek dogfood path exists
-   dogfood uses a disposable fixture repository
-   the model can read, edit, test, and fix through the controlled tool
    harness
-   SOP independently validates the resulting repository
-   no destructive integration test runs against the agentic-sop working
    repository

## AHV2013 --- Documentation

Update README documentation to distinguish:

``` text
Harness
Provider
Model
```

Document both:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

and:

``` yaml
agent:
  harness: command
  provider: command
```

Explain precedence and remove wording that implies an Ollama text
endpoint alone is an editing coding agent.

### Acceptance Criteria

-   README clearly distinguishes harness, provider, and model
-   tool-harness Ollama configuration is documented
-   legacy command-harness configuration is documented
-   environment/config/default precedence is documented
-   documentation does not imply a text-only Ollama provider can mutate
    a repository by itself

## Migration

Existing projects must continue working. In particular, current projects
using:

``` yaml
agent:
  provider: command
```

must retain command-agent semantics.

Do not automatically migrate `sop-controller` during this
implementation. After Agent Harness V2 passes dogfood testing, migrate
`sop-controller` separately.

## Safety Constraints

The harness must not:

-   directly mutate SOP workflow state
-   modify `.agent-sdlc/state.db`
-   commit automatically
-   push automatically
-   merge automatically
-   bypass human gates
-   bypass validation
-   bypass review
-   silently switch providers
-   silently switch models
-   execute arbitrary destructive Git operations
-   create an independent workflow state machine

SOP remains authoritative.

## Validation

Before completion:

``` bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

All must pass, including backward compatibility with the command
provider.

## Definition of Done

This configuration:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

can execute a real SOP task that reads files, modifies code, runs
validation, and returns a structured outcome.

SOP independently validates the result, existing command-agent projects
continue working unchanged, and new projects can safely default to:

``` text
Harness: tool
Provider: ollama
Model: deepseek-v4.1-flash:cloud
```
