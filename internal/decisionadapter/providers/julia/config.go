package julia

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Defaults used when configuration is not supplied.
const (
	// DefaultModel is the Julia decision model identifier.
	DefaultModel = "julia"
	// DefaultPython is the interpreter used to run the repository-owned helper.
	DefaultPython = "python3"
	// DefaultTimeout bounds a single inference call.
	DefaultTimeout = 30 * time.Second
)

// defaultHelperRelPath is the repository-owned inference helper, relative to the
// module root. It is resolved against the working directory and its ancestors.
const defaultHelperRelPath = "tools/julia/infer.py"

// Config holds the Julia provider's settings.
type Config struct {
	// Model is the model identifier reported in DecisionResult.Model when the
	// runner does not report one.
	Model string
	// ModelPath is the path to the ONNX model. It is forwarded to the inference
	// command as JULIA_MODEL_PATH.
	ModelPath string
	// Python is the interpreter used for the default, repository-owned helper
	// command. It is ignored when InferenceCmd overrides the command.
	Python string
	// InferenceCmd is an optional operator-provided inference command argv. When
	// empty, the provider uses the default repository helper.
	InferenceCmd []string
	// Timeout bounds a single inference call.
	Timeout time.Duration
}

// WithDefaults returns a copy of cfg with zero fields replaced by defaults.
//
// HelperPath is intentionally absent: it depends on the working directory and is
// resolved by New (see resolveHelperPath), so that Config and its tests stay
// free of filesystem state.
func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.Model) == "" {
		c.Model = DefaultModel
	}
	if strings.TrimSpace(c.Python) == "" {
		c.Python = DefaultPython
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

// ConfigFromEnv reads Julia configuration from the environment:
//
//	JULIA_MODEL          model identifier                     (default "julia")
//	JULIA_MODEL_PATH     path to the ONNX model (forwarded as JULIA_MODEL_PATH)
//	JULIA_PYTHON         interpreter for the default helper   (default "python3")
//	JULIA_INFERENCE_CMD  external inference command overriding the default helper,
//	                     whitespace-split into argv (empty => use the helper)
//	JULIA_TIMEOUT        inference timeout                   (default 30s, Go duration syntax)
func ConfigFromEnv() Config {
	return Config{
		Model:        envOr("JULIA_MODEL", DefaultModel),
		ModelPath:    strings.TrimSpace(os.Getenv("JULIA_MODEL_PATH")),
		Python:       envOr("JULIA_PYTHON", DefaultPython),
		InferenceCmd: splitCommand(os.Getenv("JULIA_INFERENCE_CMD")),
		Timeout:      envDurationOr("JULIA_TIMEOUT", DefaultTimeout),
	}.WithDefaults()
}

// resolveHelperPath locates the repository-owned helper (tools/julia/infer.py)
// relative to the working directory or one of its ancestors. When no candidate
// exists it returns the module-relative path, so Available (and any diagnostic)
// can name the expected location rather than inventing one.
func resolveHelperPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return defaultHelperRelPath
	}
	for {
		candidate := filepath.Join(dir, defaultHelperRelPath)
		if fileExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return defaultHelperRelPath
		}
		dir = parent
	}
}

// fileExists reports whether path names an existing regular file.
func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// splitCommand whitespace-splits a command string into argv. No shell quoting
// rules are applied: an argument containing spaces is unsupported.
func splitCommand(raw string) []string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	return fields
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
