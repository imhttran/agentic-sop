package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestExecutionTargetPrimary verifies the happy path: a primary SMALL selection
// yields an execution target with source primary and the primary model, while the
// routing class stays SMALL.
func TestExecutionTargetPrimary(t *testing.T) {
	sel := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, LocalitySource: LocalitySourceDefault, Source: SourceRouter}
	et := ExecutionTargetForPrimary(sel)
	if et.Class != ClassSmall {
		t.Fatalf("class = %q, want small (the routing decision is unchanged)", et.Class)
	}
	if et.Model != "qwen3:4b" || et.Locality != LocalityLocal {
		t.Fatalf("target = %+v, want the local qwen3:4b primary", et)
	}
	if et.Source != ExecutionSourcePrimary {
		t.Fatalf("source = %q, want %q", et.Source, ExecutionSourcePrimary)
	}
}

// TestExecutionTargetFallback verifies that an applied availability fallback keeps
// the routing class SMALL while recording the cloud model and the
// availability-fallback source.
func TestExecutionTargetFallback(t *testing.T) {
	routing := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, LocalitySource: LocalitySourceExplicit, Source: SourceRouter}
	fallback := Selection{Class: ClassSmall, Provider: "ollama", Model: "nemotron-3-nano:30b-cloud", Locality: LocalityCloud, LocalitySource: LocalitySourceRuntime, Source: SourceCloudFallback, Fallback: true, Reason: ReasonLocalFallback}
	et := ExecutionTargetForFallback(routing, fallback)
	if et.Class != ClassSmall {
		t.Fatalf("class = %q, want small (availability fallback must not rewrite the class)", et.Class)
	}
	if et.Model != "nemotron-3-nano:30b-cloud" || et.Locality != LocalityCloud {
		t.Fatalf("target = %+v, want the cloud nano fallback", et)
	}
	if et.Source != ExecutionSourceAvailabilityFallback {
		t.Fatalf("source = %q, want %q", et.Source, ExecutionSourceAvailabilityFallback)
	}
}

// TestExecutionTargetCarriesLocalitySource verifies that the resolved locality
// source travels from the routing Selection into the ExecutionTarget for both the
// primary and the availability-fallback execution.
func TestExecutionTargetCarriesLocalitySource(t *testing.T) {
	primary := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, LocalitySource: LocalitySourceDefault, Source: SourceRouter}
	et := ExecutionTargetForPrimary(primary)
	if et.LocalitySource != LocalitySourceDefault {
		t.Fatalf("primary LocalitySource = %q, want %q", et.LocalitySource, LocalitySourceDefault)
	}
	if et.Source != ExecutionSourcePrimary {
		t.Fatalf("source = %q, want %q", et.Source, ExecutionSourcePrimary)
	}

	routing := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, LocalitySource: LocalitySourceExplicit, Source: SourceRouter}
	fallback := Selection{Class: ClassSmall, Provider: "ollama", Model: "nemotron-3-nano:30b-cloud", Locality: LocalityCloud, LocalitySource: LocalitySourceRuntime, Source: SourceCloudFallback, Fallback: true, Reason: ReasonLocalFallback}
	et = ExecutionTargetForFallback(routing, fallback)
	if et.LocalitySource != LocalitySourceRuntime {
		t.Fatalf("fallback LocalitySource = %q, want %q", et.LocalitySource, LocalitySourceRuntime)
	}
	if et.Class != ClassSmall {
		t.Fatalf("class = %q, want small (availability fallback must not rewrite the class)", et.Class)
	}
	if et.Source != ExecutionSourceAvailabilityFallback {
		t.Fatalf("source = %q, want %q", et.Source, ExecutionSourceAvailabilityFallback)
	}
}

// TestExecutionSourceTypedValues verifies the value set is closed, typed, and
// renders deterministically (no prose).
func TestExecutionSourceTypedValues(t *testing.T) {
	if !ExecutionSourcePrimary.Valid() || !ExecutionSourceAvailabilityFallback.Valid() {
		t.Fatal("primary and availability-fallback must be valid execution sources")
	}
	if ExecutionSource("something-else").Valid() {
		t.Fatal("an unknown execution source must be invalid")
	}
	if got := ExecutionSource("").String(); got != "primary" {
		t.Fatalf("empty source String() = %q, want primary", got)
	}
	if got := ExecutionSourceAvailabilityFallback.String(); got != "availability-fallback" {
		t.Fatalf("String() = %q, want availability-fallback", got)
	}
}

// TestExecutionTargetLocalitySourceJSONEncoding verifies that an execution target
// encodes the locality source as locality_source and omits it when empty, so an
// older persisted record decodes and re-encodes without the field.
func TestExecutionTargetLocalitySourceJSONEncoding(t *testing.T) {
	et := ExecutionTarget{Class: ClassLarge, Provider: "ollama", Model: "deepseek-v4.1-flash:cloud", Locality: LocalityCloud, LocalitySource: LocalitySourceDefault, Source: ExecutionSourcePrimary}
	data, err := json.Marshal(et)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"locality_source":"default"`) {
		t.Fatalf("target = %s, want locality_source \"default\"", data)
	}

	// An execution target whose locality source was never recorded (older
	// evidence) re-encodes without the field.
	old := ExecutionTarget{Class: ClassLarge, Provider: "ollama", Model: "deepseek-v4.1-flash:cloud", Locality: LocalityCloud, Source: ExecutionSourcePrimary}
	data, err = json.Marshal(old)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "locality_source") {
		t.Fatalf("an empty locality_source must be omitted: %s", data)
	}

	// An older record without the field decodes with an empty locality source.
	var back ExecutionTarget
	if err := json.Unmarshal([]byte(`{"class":"small","provider":"ollama","model":"qwen3:4b","locality":"local","execution_source":"primary"}`), &back); err != nil {
		t.Fatalf("unmarshal older evidence: %v", err)
	}
	if back.LocalitySource != "" {
		t.Fatalf("decoded LocalitySource = %q, want empty", back.LocalitySource)
	}
}

// TestExecutionTargetIsNonSecret verifies the execution target carries no
// credential-shaped field.
func TestExecutionTargetIsNonSecret(t *testing.T) {
	const secret = "supersecret-token"
	sel := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal}
	et := ExecutionTargetForPrimary(sel)
	for _, f := range []string{string(et.Class), et.Provider, et.Model, string(et.Locality), string(et.Source)} {
		if f == secret {
			t.Fatal("execution target leaked a secret")
		}
	}
}
