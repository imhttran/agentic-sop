package clef

import (
	"context"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/decision"
)

// unsupportedMarker is the sentinel that identifies an operation the Clef
// adapter cannot perform. Unsupported operations return this classification
// rather than a success-shaped answer.
//
// It carries an Unsupported method so the provider-neutral wire layer can
// classify it as UNSUPPORTED through a structural interface, without importing
// any adapter package.
type unsupportedMarker struct{ reason string }

func (e *unsupportedMarker) Error() string { return "unsupported operation: " + e.reason }

// Unsupported reports that this error represents an unsupported operation. The
// provider-neutral wire layer detects it structurally.
func (e *unsupportedMarker) Unsupported() bool { return true }

// ErrUnsupported is the sentinel that identifies an operation the Clef adapter
// cannot perform. Unsupported operations return this classification rather
// than a success-shaped answer.
var ErrUnsupported = &unsupportedMarker{reason: "operation not supported by the clef adapter"}

// Capability describes what the Clef adapter can currently serve. It is
// provider-neutral in shape: it reports the adapter name, whether the adapter
// is enabled/available, and the operations it cannot perform.
type Capability struct {
	// Name is the stable adapter identifier.
	Name string
	// Enabled reports whether the operator explicitly enabled Clef.
	Enabled bool
	// Available reports whether the backend can currently serve decisions.
	Available bool
	// UnsupportedOperations lists operation identifiers the adapter cannot
	// perform through the selected transport. Unsupported operations return
	// UNSUPPORTED rather than a success-shaped answer.
	UnsupportedOperations []string
}

// unsupportedOperations enumerates the operations the Clef adapter does not
// implement. They are reported as UNSUPPORTED; none of them is silently mapped
// onto a successful outcome.
var unsupportedOperations = []string{
	"stream",
	"batch",
	"function_call",
	"embeddings",
	"completion",
	"chat",
}

// Capability reports the adapter's current capability without issuing an
// inference request. When Clef is disabled it reports Name and Enabled=false
// and never claims availability.
func (p *Provider) Capability(ctx context.Context) Capability {
	cap := Capability{
		Name:                  ProviderName,
		Enabled:               p.cfg.Enabled(),
		UnsupportedOperations: append([]string(nil), unsupportedOperations...),
	}
	if !cap.Enabled {
		return cap
	}
	cap.Available = p.Available(ctx)
	return cap
}

// Supports reports whether the named operation is supported. Unsupported
// operations must be surfaced as UNSUPPORTED by callers.
func Supports(operation string) bool {
	op := strings.ToLower(strings.TrimSpace(operation))
	for _, unsupported := range unsupportedOperations {
		if op == unsupported {
			return false
		}
	}
	return op == "decide"
}

// UnsupportedError returns a normalized UNSUPPORTED error attributed to this
// adapter. The unsupported classification is carried by the wrapped cause so
// the provider-neutral wire layer can surface it as UNSUPPORTED; the error also
// carries KindInvalidRequest so an unsupported operation is never mistaken for
// a transport failure.
func UnsupportedError(operation string) error {
	return &decision.ProviderError{
		Provider: ProviderName,
		Kind:     decision.KindInvalidRequest,
		Err:      fmt.Errorf("%w: operation %q", ErrUnsupported, operation),
	}
}
