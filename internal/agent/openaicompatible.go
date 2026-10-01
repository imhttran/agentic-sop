package agent

// Environment variables for the generic OpenAI-compatible provider's
// configuration surface. Phase 1 recognizes and configures this provider, but it
// is not executable yet: these names exist so configuration, tests, and drift
// guards share one vocabulary (SOP_OPENAI_COMPATIBLE_BASE_URL is the same setting
// the config layer resolves, internal/config.EnvOpenAICompatibleBaseURL).
//
// The actual endpoint precedence (environment > configuration > default) is
// resolved by the config layer (internal/config), not here, and no executable
// transport is wired in this phase.
const (
	EnvOpenAICompatibleBaseURL = "SOP_OPENAI_COMPATIBLE_BASE_URL"
	EnvOpenAICompatibleModel   = "SOP_OPENAI_COMPATIBLE_MODEL"
	EnvOpenAICompatibleTimeout = "SOP_OPENAI_COMPATIBLE_TIMEOUT"
	EnvOpenAICompatibleAPIKey  = "SOP_OPENAI_COMPATIBLE_API_KEY"
)
