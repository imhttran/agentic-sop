package ollamaagent

// Command policy, command tokenizing, and command execution used to live here.
// They now live in internal/toolharness (SplitCommand, CheckCommand, and the
// harness's run_command), so there is exactly one implementation of the command
// policy and one place that denies destructive Git operations and access to
// .agent-sdlc/state.db. The agent loop reaches them through toolbox.run.
