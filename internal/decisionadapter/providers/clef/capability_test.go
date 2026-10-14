package clef

import (
	"context"
	"errors"
	"testing"
)

// unsupportedOps must match the operations capability.go reports as
// unsupported. The contract is that these return UNSUPPORTED rather than a
// success-shaped answer.
var unsupportedOps = []string{
	"stream",
	"batch",
	"function_call",
	"embeddings",
	"completion",
	"chat",
}

func TestCapabilityDisabledByDefault(t *testing.T) {
	p := New(Config{}, nil)

	cap := p.Capability(context.Background())
	if cap.Name != ProviderName {
		t.Errorf("Name = %q, want %q", cap.Name, ProviderName)
	}
	if cap.Enabled {
		t.Error("Enabled = true, want false for default (disabled) config")
	}
	// A disabled provider must never claim availability.
	if cap.Available {
		t.Error("Available = true, want false when Clef is disabled")
	}
}

func TestCapabilityReportsUnsupportedOperations(t *testing.T) {
	p := New(Config{}, nil)
	cap := p.Capability(context.Background())

	set := make(map[string]bool, len(cap.UnsupportedOperations))
	for _, op := range cap.UnsupportedOperations {
		set[op] = true
	}
	for _, op := range unsupportedOps {
		if !set[op] {
			t.Errorf("UnsupportedOperations missing %q", op)
		}
	}
}

func TestSupportsUnsupportedOperationsReturnFalse(t *testing.T) {
	for _, op := range unsupportedOps {
		if Supports(op) {
			t.Errorf("Supports(%q) = true, want false", op)
		}
	}
}

func TestSupportsDecideOnly(t *testing.T) {
	if !Supports("decide") {
		t.Error("Supports(\"decide\") = false, want true")
	}
	// Case/whitespace normalization.
	if !Supports("  DECIDE  ") {
		t.Error("Supports(\"  DECIDE  \") = false, want true")
	}
	if Supports("unknown-op") {
		t.Error("Supports(\"unknown-op\") = true, want false")
	}
}

// TestUnsupportedErrorMatchesSentinel proves UnsupportedError surfaces the
// ErrUnsupported sentinel so the provider-neutral wire layer can classify the
// failure as UNSUPPORTED.
func TestUnsupportedErrorMatchesSentinel(t *testing.T) {
	if ErrUnsupported == nil {
		t.Fatal("ErrUnsupported is nil")
	}
	if !unsupportedMarkerIsUnsupported(ErrUnsupported) {
		t.Error("ErrUnsupported does not report Unsupported()=true")
	}

	err := UnsupportedError("stream")
	if err == nil {
		t.Fatal("UnsupportedError returned nil")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("errors.Is(UnsupportedError, ErrUnsupported) = false, want true; err=%v", err)
	}
	if !unsupportedMarkerIsUnsupported(err) {
		t.Errorf("UnsupportedError does not carry the unsupported classification: %v", err)
	}
}

// unsupportedMarkerIsUnsupported reports whether err (or anything it wraps)
// structurally reports Unsupported()=true, without importing adapter packages.
func unsupportedMarkerIsUnsupported(err error) bool {
	type unsupported interface{ Unsupported() bool }
	for err != nil {
		if u, ok := err.(unsupported); ok {
			return u.Unsupported()
		}
		err = errors.Unwrap(err)
	}
	return false
}
