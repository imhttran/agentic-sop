package config

import "testing"

func envLookup(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestRAGDisabledByDefault(t *testing.T) {
	got, err := RAGConfig{}.Resolved(envLookup(nil))
	if err != nil {
		t.Fatalf("Resolved: %v", err)
	}
	if got.Enabled {
		t.Fatal("RAG must be disabled by default")
	}
	if got.TimeoutMS != DefaultRAGTimeoutMS || got.TopK != DefaultRAGTopK || got.ContextBudgetBytes != DefaultRAGContextBudgetBytes {
		t.Fatalf("defaults = %d/%d/%d", got.TimeoutMS, got.TopK, got.ContextBudgetBytes)
	}
	if len(got.AllowedCorpora) != 0 {
		t.Fatalf("allowlist = %v, want none", got.AllowedCorpora)
	}
}

func TestRAGEnabledRequiresTrustedConfiguration(t *testing.T) {
	base := map[string]string{EnvRAGEnabled: "true"}
	cases := map[string]map[string]string{
		"missing executable": merge(base, nil),
		"missing corpus":     merge(base, map[string]string{EnvRAGExecutable: "/p"}),
		"empty allowlist":    merge(base, map[string]string{EnvRAGExecutable: "/p", EnvRAGCorpus: "kb"}),
		"unauthorized corpus": merge(base, map[string]string{
			EnvRAGExecutable: "/p", EnvRAGCorpus: "other", EnvRAGAllowedCorpora: "kb",
		}),
		"relative executable": merge(base, map[string]string{
			EnvRAGExecutable: "bin/rag", EnvRAGCorpus: "kb", EnvRAGAllowedCorpora: "kb",
		}),
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (RAGConfig{}).Resolved(envLookup(tc)); err == nil {
				t.Fatal("Resolved accepted an invalid enabled configuration, want error")
			}
		})
	}
}

func TestRAGValidEnablementNormalizesAllowlist(t *testing.T) {
	got, err := (RAGConfig{}).Resolved(envLookup(map[string]string{
		EnvRAGEnabled:        "true",
		EnvRAGExecutable:     "/usr/local/bin/rag-retrieval",
		EnvRAGCorpus:         "a",
		EnvRAGAllowedCorpora: "b, a, a ,",
	}))
	if err != nil {
		t.Fatalf("Resolved: %v", err)
	}
	if !got.Enabled {
		t.Fatal("RAG should be enabled")
	}
	if len(got.AllowedCorpora) != 2 || got.AllowedCorpora[0] != "a" || got.AllowedCorpora[1] != "b" {
		t.Fatalf("allowlist = %v, want deterministic [a b]", got.AllowedCorpora)
	}
}

func TestRAGEnvironmentOverridesFileBlock(t *testing.T) {
	file := RAGConfig{Enabled: false, Executable: "/file/exe", Corpus: "kb", AllowedCorpora: []string{"kb"}}
	got, err := file.Resolved(envLookup(map[string]string{
		EnvRAGEnabled:    "true",
		EnvRAGExecutable: "/env/exe",
	}))
	if err != nil {
		t.Fatalf("Resolved: %v", err)
	}
	if !got.Enabled || got.Executable != "/env/exe" {
		t.Fatalf("env did not override the block: %+v", got)
	}
}

func TestRAGInvalidValuesRejected(t *testing.T) {
	cases := map[string]map[string]string{
		"invalid boolean":        {EnvRAGEnabled: "maybe"},
		"invalid timeout":        {EnvRAGTimeout: "abc"},
		"zero timeout":           {EnvRAGTimeout: "0"},
		"invalid result limit":   {EnvRAGTopK: "-1"},
		"invalid context budget": {EnvRAGContextBudget: "nope"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (RAGConfig{}).Resolved(envLookup(env)); err == nil {
				t.Fatalf("Resolved accepted %v, want error", env)
			}
		})
	}
}

func TestRAGDisabledIgnoresIncompleteValues(t *testing.T) {
	// Disabled + no executable/corpus must not error: existing behavior is preserved.
	got, err := (RAGConfig{}).Resolved(envLookup(map[string]string{EnvRAGTimeout: "1500"}))
	if err != nil {
		t.Fatalf("Resolved: %v", err)
	}
	if got.Enabled || got.TimeoutMS != 1500 {
		t.Fatalf("disabled resolve = %+v", got)
	}
}

func merge(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
