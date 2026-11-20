package clef

import (
	"os"
	"strings"
	"time"
)

// Defaults used when configuration is not supplied.
//
// Clef is OFF by default: the zero Config is disabled and must be explicitly
// enabled by the operator (Enable=true or CLEF_ENABLED=1). Transport defaults
// MUST NOT implicitly enable Clef.
const (
	// DefaultBaseURL is the conventional local Ollama endpoint. It is only a
	// default: callers must be able to point at a remote backend via
	// OLLAMA_BASE_URL, so no code should assume the server is local.
	DefaultBaseURL = "http://localhost:11434"
	// DefaultModel is the Clef decision model name.
	DefaultModel = "clef-flash"
	// DefaultTimeout bounds a single Clef/Ollama call.
	DefaultTimeout = 30 * time.Second
)

// Transport selectors for the Clef provider.
const (
	// TransportSystemOne is the Ollama /v1/systemone transport; it is the default
	// and is preserved unchanged.
	TransportSystemOne = "systemone"
	// TransportChat is the OpenAI-compatible /v1/chat/completions transport
	// (for example oMLX serving clef-4bit).
	TransportChat = "chat"
)

// Config holds the Clef provider's connection settings.
//
// Enable is the single gate that turns the adapter on. It defaults to false in
// every construction path: Config{}, ConfigFromEnv with no CLEF_ENABLED value,
// and WithDefaults all leave Clef disabled. Nothing in the transport defaults
// (BaseURL, Model, Timeout) enables the provider.
type Config struct {
	// BaseURL is the Ollama-compatible backend root, for example
	// "http://ollama.internal:11434".
	BaseURL string
	// Model is the model served by the backend.
	Model string
	// Timeout bounds a single request.
	Timeout time.Duration
	// Enable explicitly enables the Clef provider. Required operator action.
	Enable bool
	// Transport selects the wire protocol: TransportSystemOne (default) or
	// TransportChat. The default preserves the /v1/systemone transport.
	Transport string
}

// Enabled reports whether the provider is explicitly enabled.
func (c Config) Enabled() bool { return c.Enable }

// WithDefaults returns a copy of cfg with zero transport fields replaced by
// defaults. It never changes Enable: defaults must not implicitly enable Clef.
func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.BaseURL) == "" {
		c.BaseURL = DefaultBaseURL
	}
	if strings.TrimSpace(c.Model) == "" {
		c.Model = DefaultModel
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if strings.TrimSpace(c.Transport) == "" {
		c.Transport = TransportSystemOne
	}
	return c
}

// ConfigFromEnv reads Clef configuration from the environment:
//
//	OLLAMA_BASE_URL  backend root URL (default http://localhost:11434)
//	CLEF_MODEL       model name          (default "clef-flash")
//	CLEF_TIMEOUT     request timeout     (default 30s, Go duration syntax)
//	CLEF_ENABLED     explicit enable flag (default false; required to enable)
//	CLEF_TRANSPORT   "systemone" (default) or "chat" (OpenAI /v1/chat/completions)
//
// The package default stays off: ConfigFromEnv leaves the provider disabled unless
// CLEF_ENABLED is "1"/"true"/"yes" (case-insensitive). An operator who selects
// Clef by name gets EnabledWhenSelected instead.
func ConfigFromEnv() Config {
	return Config{
		BaseURL:   envOr("OLLAMA_BASE_URL", DefaultBaseURL),
		Model:     envOr("CLEF_MODEL", DefaultModel),
		Timeout:   envDurationOr("CLEF_TIMEOUT", DefaultTimeout),
		Enable:    envBool("CLEF_ENABLED"),
		Transport: envOr("CLEF_TRANSPORT", TransportSystemOne),
	}.WithDefaults()
}

// EnabledWhenSelected reports whether Clef may serve once an operator has
// selected it by name (-provider clef, SOP_DECISION_ADAPTER_PROVIDER=clef, or an
// explicit SOP_DECISION_FALLBACK=clef). Naming it is the opt-in, so an unset or
// empty CLEF_ENABLED allows it and "1"/"true"/"yes" allows it explicitly. Any
// other value (0, false, off, or something unparseable) turns it off, so
// CLEF_ENABLED=0 is a kill switch and a typo fails closed.
func EnabledWhenSelected() bool {
	if strings.TrimSpace(os.Getenv("CLEF_ENABLED")) == "" {
		return true
	}
	return envBool("CLEF_ENABLED")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
