package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RAG configuration (RAG-SOP-003). The layer is opt-in and DISABLED by default:
// with an omitted block and no SOP_RAG_* environment, Enabled is false and the
// resolved values are inert, so existing behavior is unchanged.
//
// Precedence is the project-wide one: SOP_RAG_* environment > the `rag:` block in
// .agent-sdlc/config.yaml > the built-in defaults below. It is resolved by
// RAGConfig.Resolved; nothing reads these fields yet (the verifier and context
// integration are later packages).
const (
	EnvRAGEnabled        = "SOP_RAG_ENABLED"
	EnvRAGExecutable     = "SOP_RAG_EXECUTABLE"
	EnvRAGCorpus         = "SOP_RAG_CORPUS"
	EnvRAGAllowedCorpora = "SOP_RAG_ALLOWED_CORPORA"
	EnvRAGTimeout        = "SOP_RAG_TIMEOUT"
	EnvRAGTopK           = "SOP_RAG_TOP_K"
	EnvRAGContextBudget  = "SOP_RAG_CONTEXT_BUDGET"

	// DefaultRAGTimeoutMS bounds one retrieval request (milliseconds).
	DefaultRAGTimeoutMS = 3000
	// DefaultRAGTopK caps the number of retrieved evidence items.
	DefaultRAGTopK = 5
	// DefaultRAGContextBudgetBytes bounds RAG evidence in the context engine.
	DefaultRAGContextBudgetBytes = 4096
)

// RAGConfig is the RAG knowledge-retrieval configuration block.
type RAGConfig struct {
	// Enabled is the master opt-in. It is false unless explicitly enabled by the
	// environment or the `rag:` block, so RAG is never enabled implicitly.
	Enabled bool `yaml:"enabled"`
	// Executable is the absolute path of the retrieval-only producer.
	Executable string `yaml:"executable"`
	// Corpus is the default/trusted corpus a request is scoped to when a task does
	// not name one.
	Corpus string `yaml:"corpus"`
	// AllowedCorpora is the trusted corpus allowlist. Empty forbids retrieval.
	AllowedCorpora []string `yaml:"allowed_corpora"`
	// TimeoutMS bounds one retrieval request, in milliseconds.
	TimeoutMS int `yaml:"timeout_ms"`
	// TopK caps the number of retrieved evidence items.
	TopK int `yaml:"top_k"`
	// ContextBudgetBytes bounds RAG evidence in the context engine (bytes, never
	// tokens, matching SOP's provider-independent accounting).
	ContextBudgetBytes int `yaml:"context_budget_bytes"`
}

// Timeout is the resolved per-request deadline.
func (r RAGConfig) Timeout() time.Duration {
	return time.Duration(r.TimeoutMS) * time.Millisecond
}

// Resolved applies the environment over this block, fills defaults, normalizes
// the corpus allowlist deterministically, and validates the result. It never
// enables RAG implicitly: an omitted block with no environment resolves to
// disabled and is returned without validation, so existing configuration and
// execution behavior are unchanged. An unparseable or out-of-range value is an
// actionable error rather than a silently ignored setting.
func (r RAGConfig) Resolved(getenv func(string) string) (RAGConfig, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	if v := strings.TrimSpace(getenv(EnvRAGEnabled)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return RAGConfig{}, fmt.Errorf("rag: %s: invalid boolean %q (want true or false)", EnvRAGEnabled, v)
		}
		r.Enabled = b
	}
	if v := strings.TrimSpace(getenv(EnvRAGExecutable)); v != "" {
		r.Executable = v
	}
	if v := strings.TrimSpace(getenv(EnvRAGCorpus)); v != "" {
		r.Corpus = v
	}
	if v := strings.TrimSpace(getenv(EnvRAGAllowedCorpora)); v != "" {
		r.AllowedCorpora = splitCorpora(v)
	}
	if v := strings.TrimSpace(getenv(EnvRAGTimeout)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return RAGConfig{}, fmt.Errorf("rag: %s: invalid timeout %q (want a positive integer of milliseconds)", EnvRAGTimeout, v)
		}
		r.TimeoutMS = n
	}
	if v := strings.TrimSpace(getenv(EnvRAGTopK)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return RAGConfig{}, fmt.Errorf("rag: %s: invalid result limit %q (want a positive integer)", EnvRAGTopK, v)
		}
		r.TopK = n
	}
	if v := strings.TrimSpace(getenv(EnvRAGContextBudget)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return RAGConfig{}, fmt.Errorf("rag: %s: invalid context budget %q (want a positive integer of bytes)", EnvRAGContextBudget, v)
		}
		r.ContextBudgetBytes = n
	}

	if r.TimeoutMS <= 0 {
		r.TimeoutMS = DefaultRAGTimeoutMS
	}
	if r.TopK <= 0 {
		r.TopK = DefaultRAGTopK
	}
	if r.ContextBudgetBytes <= 0 {
		r.ContextBudgetBytes = DefaultRAGContextBudgetBytes
	}
	r.AllowedCorpora = normalizeCorpora(r.AllowedCorpora)

	if !r.Enabled {
		return r, nil
	}

	if strings.TrimSpace(r.Executable) == "" {
		return RAGConfig{}, fmt.Errorf("rag: executable is required when %s is true", EnvRAGEnabled)
	}
	if !filepath.IsAbs(r.Executable) {
		return RAGConfig{}, fmt.Errorf("rag: executable must be an absolute path, got %q", r.Executable)
	}
	if strings.TrimSpace(r.Corpus) == "" {
		return RAGConfig{}, fmt.Errorf("rag: a trusted corpus is required when %s is true", EnvRAGEnabled)
	}
	if len(r.AllowedCorpora) == 0 {
		return RAGConfig{}, fmt.Errorf("rag: a non-empty allowed_corpora is required when %s is true", EnvRAGEnabled)
	}
	if !containsCorpus(r.AllowedCorpora, r.Corpus) {
		return RAGConfig{}, fmt.Errorf("rag: corpus %q is not in allowed_corpora", r.Corpus)
	}

	return r, nil
}

func splitCorpora(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// normalizeCorpora deduplicates and sorts the allowlist so the result is stable
// regardless of input order. An empty input returns nil.
func normalizeCorpora(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func containsCorpus(corpora []string, corpus string) bool {
	corpus = strings.TrimSpace(corpus)
	for _, c := range corpora {
		if c == corpus {
			return true
		}
	}
	return false
}
