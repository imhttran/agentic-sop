package model

import "testing"

// TestExecutionTargetPrimary verifies the happy path: a primary SMALL selection
// yields an execution target with source primary and the primary model, while the
// routing class stays SMALL.
func TestExecutionTargetPrimary(t *testing.T) {
	sel := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, Source: SourceRouter}
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
	routing := Selection{Class: ClassSmall, Provider: "ollama", Model: "qwen3:4b", Locality: LocalityLocal, Source: SourceRouter}
	fallback := Selection{Class: ClassSmall, Provider: "ollama", Model: "nemotron-3-nano:30b-cloud", Locality: LocalityCloud, Source: SourceCloudFallback, Fallback: true, Reason: ReasonLocalFallback}
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
