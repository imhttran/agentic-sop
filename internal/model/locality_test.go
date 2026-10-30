package model

import (
	"errors"
	"strings"
	"testing"
)

func TestLocalitySourceTypedValues(t *testing.T) {
	for _, s := range []LocalitySource{
		LocalitySourceExplicit,
		LocalitySourceInferred,
		LocalitySourceRuntime,
		LocalitySourceDefault,
	} {
		if !s.Valid() {
			t.Fatalf("%q must be a valid locality source", s)
		}
	}
	if LocalitySource("guess").Valid() {
		t.Fatal("an unknown locality source must be invalid")
	}
	if LocalitySource("").Valid() {
		t.Fatal("an empty locality source must be invalid")
	}
}

func TestInferLocality(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     Locality
		ok       bool
	}{
		{"ollama cloud suffix", "ollama", "nemotron-3-super:cloud", LocalityCloud, true},
		{"ollama cloud suffix with spaces", " ollama ", " nemotron:cloud ", LocalityCloud, true},
		{"ollama name containing cloud", "ollama", "cloudy-day", "", false},
		{"ollama name with cloud not as suffix", "ollama", "cloud:7b", "", false},
		{"ollama local model", "ollama", "qwen3:4b", "", false},
		{"cloud suffix on another provider", "openai_compatible", "gpt:cloud", "", false},
		{"llamacpp model", "llamacpp", "something:cloud", "", false},
		{"claude name", "openai_compatible", "claude-3-5-sonnet", "", false},
		{"claude name on ollama", "ollama", "claude-3-5-sonnet", "", false},
		{"custom provider arbitrary model", "command", "whatever:cloud", "", false},
		{"empty provider", "", "model:cloud", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := InferLocality(tc.provider, tc.model)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("InferLocality(%q, %q) = (%q, %v), want (%q, %v)", tc.provider, tc.model, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestLocalityRefSettings(t *testing.T) {
	primary := LocalityRef{Class: ClassSmall, Role: LocalityRolePrimary}
	if got, want := primary.ModelSetting(), "models.small.locality"; got != want {
		t.Fatalf("primary ModelSetting() = %q, want %q", got, want)
	}
	if got, want := primary.EnvSetting(), "SOP_MODEL_SMALL_LOCALITY"; got != want {
		t.Fatalf("primary EnvSetting() = %q, want %q", got, want)
	}

	fb := LocalityRef{Class: ClassLarge, Role: LocalityRoleFallback}
	if got, want := fb.ModelSetting(), "models.large.fallback.locality"; got != want {
		t.Fatalf("fallback ModelSetting() = %q, want %q", got, want)
	}
	if got, want := fb.EnvSetting(), "SOP_MODEL_LARGE_FALLBACK_LOCALITY"; got != want {
		t.Fatalf("fallback EnvSetting() = %q, want %q", got, want)
	}
}

func TestResolveLocalitySources(t *testing.T) {
	evidence := map[LocalityEvidence]Locality{
		{Provider: "ollama", Model: "runtime-model"}: LocalityLocal,
	}
	cases := []struct {
		name     string
		ref      LocalityRef
		explicit Locality
		evidence map[LocalityEvidence]Locality
		useDef   bool
		def      Locality
		want     Locality
		source   LocalitySource
	}{
		{
			name:     "explicit wins",
			ref:      LocalityRef{Class: ClassSmall, Role: LocalityRolePrimary, Provider: "ollama", Model: "qwen3:4b"},
			explicit: LocalityLocal,
			want:     LocalityLocal,
			source:   LocalitySourceExplicit,
		},
		{
			name:     "explicit equals runtime keeps explicit",
			ref:      LocalityRef{Class: ClassSmall, Role: LocalityRolePrimary, Provider: "ollama", Model: "runtime-model"},
			explicit: LocalityLocal,
			evidence: evidence,
			want:     LocalityLocal,
			source:   LocalitySourceExplicit,
		},
		{
			name:     "runtime evidence",
			ref:      LocalityRef{Class: ClassSmall, Role: LocalityRolePrimary, Provider: "ollama", Model: "runtime-model"},
			evidence: evidence,
			want:     LocalityLocal,
			source:   LocalitySourceRuntime,
		},
		{
			name:   "naming inference",
			ref:    LocalityRef{Class: ClassLarge, Role: LocalityRolePrimary, Provider: "ollama", Model: "deepseek:cloud"},
			want:   LocalityCloud,
			source: LocalitySourceInferred,
		},
		{
			name:   "built-in default",
			ref:    LocalityRef{Class: ClassMedium, Role: LocalityRolePrimary, Provider: "ollama", Model: "plain-model"},
			useDef: true,
			def:    LocalityLocal,
			want:   LocalityLocal,
			source: LocalitySourceDefault,
		},
		{
			name:   "default from the built-in class table",
			ref:    LocalityRef{Class: ClassMedium, Role: LocalityRolePrimary, Provider: "ollama", Model: "plain-model"},
			useDef: true,
			want:   LocalityCloud,
			source: LocalitySourceDefault,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ResolveLocality(tc.ref, tc.explicit, tc.evidence, tc.useDef, tc.def)
			if err != nil {
				t.Fatalf("ResolveLocality: %v", err)
			}
			if res.Locality != tc.want || res.Source != tc.source {
				t.Fatalf("resolution = %+v, want locality %q source %q", res, tc.want, tc.source)
			}
		})
	}
}

func TestResolveLocalityConflicts(t *testing.T) {
	runtimeEvidence := map[LocalityEvidence]Locality{
		{Provider: "ollama", Model: "runtime-model"}: LocalityLocal,
	}
	suffixEvidence := map[LocalityEvidence]Locality{
		{Provider: "ollama", Model: "name:cloud"}: LocalityLocal,
	}
	cases := []struct {
		name     string
		ref      LocalityRef
		explicit Locality
		evidence map[LocalityEvidence]Locality
	}{
		{
			name:     "explicit contradicts runtime",
			ref:      LocalityRef{Class: ClassLarge, Role: LocalityRolePrimary, Provider: "ollama", Model: "runtime-model"},
			explicit: LocalityCloud,
			evidence: runtimeEvidence,
		},
		{
			name:     "explicit contradicts inference",
			ref:      LocalityRef{Class: ClassLarge, Role: LocalityRolePrimary, Provider: "ollama", Model: "deepseek:cloud"},
			explicit: LocalityLocal,
		},
		{
			name:     "runtime contradicts inference",
			ref:      LocalityRef{Class: ClassLarge, Role: LocalityRolePrimary, Provider: "ollama", Model: "name:cloud"},
			evidence: suffixEvidence,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveLocality(tc.ref, tc.explicit, tc.evidence, false, "")
			var conflict *LocalityConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("error = %v, want *LocalityConflictError", err)
			}
			msg := err.Error()
			for _, want := range []string{
				string(tc.ref.Class),
				tc.ref.Provider,
				tc.ref.Model,
				tc.ref.ModelSetting(),
				tc.ref.EnvSetting(),
			} {
				if !strings.Contains(msg, want) {
					t.Fatalf("error %q must name %q", msg, want)
				}
			}
		})
	}
}

func TestResolveLocalityUnknown(t *testing.T) {
	ref := LocalityRef{Class: ClassLarge, Role: LocalityRolePrimary, Provider: "ollama", Model: "plain-model"}
	_, err := ResolveLocality(ref, "", nil, false, "")
	var unknown *LocalityUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want *LocalityUnknownError", err)
	}
	if len(unknown.Refs) != 1 || unknown.Refs[0] != ref {
		t.Fatalf("unknown refs = %+v, want the single unresolved reference", unknown.Refs)
	}
	msg := err.Error()
	for _, want := range []string{
		"large",
		"primary",
		"ollama",
		"plain-model",
		"models.large.locality",
		"SOP_MODEL_LARGE_LOCALITY",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q must name %q", msg, want)
		}
	}
}

func TestLocalityUnknownErrorFallbackSettings(t *testing.T) {
	ref := LocalityRef{Class: ClassSmall, Role: LocalityRoleFallback, Provider: "ollama", Model: "fb-model"}
	err := (&LocalityUnknownError{Refs: []LocalityRef{ref}}).Error()
	for _, want := range []string{
		"fallback",
		"models.small.fallback.locality",
		"SOP_MODEL_SMALL_FALLBACK_LOCALITY",
	} {
		if !strings.Contains(err, want) {
			t.Fatalf("error %q must name %q", err, want)
		}
	}
}
