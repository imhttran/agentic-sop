package agent

import "context"

// Harness is the engineering-execution layer, separate from the model provider.
// The responsibility split is explicit and must not blur:
//
//	Provider -> model inference
//	Harness  -> engineering execution + controlled tools
//	SOP      -> workflow authority
//
// A Provider (an Agent implementation such as CommandAgent, Ollama, LlamaCpp, or
// MLX) performs model inference only: it turns a Request into a Response
// and must never mutate the filesystem or workflow state. A Harness owns
// engineering execution itself, driving controlled tools (for example the
// read_file/write_file/create_file/list_files/search_files/run_command/
// git_status/git_diff surface exposed by internal/toolharness) under policy
// checks and audit.
//
// A Harness is an execution adapter beneath SOP, never a workflow authority.
// Task scheduling, retries, validation gates, review gates, and state
// transitions stay in SOP; a Harness must not implement, duplicate, or move any
// of them, and must never read or mutate SOP's workflow state
// (.agent-sdlc/state.db).
type Harness interface {
	// Execute performs one engineering-execution request against the controlled
	// tool surface, returning the same Response shape a Provider would. It is an
	// adapter only: it does not schedule, retry, decide validation or review
	// gates, or transition workflow state.
	Execute(ctx context.Context, request Request) (Response, error)
}

// Compile-time assertions documenting the boundary: the model providers satisfy
// Agent (Provider -> model inference), while a Harness is a distinct
// abstraction (engineering execution + controlled tools). A type that satisfies
// both would blur the separation, so the two interfaces are deliberately not
// interchangeable.
var (
	_ Agent = (*CommandAgent)(nil)
	_ Agent = (*Ollama)(nil)
	_ Agent = (*OpenAICompatible)(nil)
	_ Agent = (*LlamaCpp)(nil)
	_ Agent = (*MLX)(nil)

	_ Harness = (*CommandHarness)(nil)
)

// HarnessFunc adapts a function to the Harness interface, mirroring
// http.HandlerFunc. It lets a caller supply an execution adapter without
// declaring a named type, for example in tests or in a composition root that
// wraps another harness.
type HarnessFunc func(context.Context, Request) (Response, error)

// Execute calls f.
func (f HarnessFunc) Execute(ctx context.Context, request Request) (Response, error) {
	return f(ctx, request)
}
