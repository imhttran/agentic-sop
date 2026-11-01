# S2 — Build the isolated evaluation target

Create a temporary repository and separate SOP state per case. Supply synthetic retrieved documents, advisory memory records, and fake secrets. Route external writes through recording tools while keeping the real authorization and policy checks in front of those tools. Capture attempted tool calls separately from executed calls. Capture approval decisions, resulting state, verification evidence, and final responses. Reset all case state between evaluations and terminate child processes on timeout or cancellation. Add a controlled failing fixture that proves the evaluator detects a prohibited effect or state transition.

## Expected outcomes (descriptive)
Report these in your summary. They do not require a new file unless an acceptance criterion or a required deliverable file names one.
- Isolated per-case target harness with separate SOP state and reset between cases
- Recording tools for external effects placed after real controls, with attempted and executed calls captured separately
- Case evidence capture for approval decisions, state, verification evidence, and final responses
- Controlled failing fixture that demonstrates detection of a prohibited effect or state transition
- Timeout and cancellation handling that terminates child processes

## Acceptance criteria
- Evaluations cannot alter ordinary SOP state, access production secrets, or perform external writes
- Only external effects are stubbed; the controls under evaluation run for real
- Attempted and executed tool calls are recorded and distinguishable
- A controlled failing fixture is detected as a failure
- Case state is reset between evaluations and child processes terminate on timeout or cancellation
