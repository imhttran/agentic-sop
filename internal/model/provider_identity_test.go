package model_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

// TestKnownProvidersMatchCanonicalIDs prevents provider identity drift.
//
// internal/model cannot import internal/provider (provider imports model, so the
// reverse is an import cycle), so model keeps its own copy of the provider
// allow-list. This external test is the guarantee that the copy stays identical to
// the canonical set in internal/provider: adding a provider id there without
// updating model (or vice versa) fails here.
func TestKnownProvidersMatchCanonicalIDs(t *testing.T) {
	canonical := make([]string, 0, len(provider.KnownIDs()))
	for _, id := range provider.KnownIDs() {
		canonical = append(canonical, id.String())
	}
	sort.Strings(canonical)

	if got := model.KnownProviders(); !reflect.DeepEqual(got, canonical) {
		t.Fatalf("model.KnownProviders() = %v, want provider.KnownIDs() = %v", got, canonical)
	}
}
