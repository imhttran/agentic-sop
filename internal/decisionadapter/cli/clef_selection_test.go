package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/clef"
)

// TestNewProviderClefOffByDefault proves that selecting the clef provider
// through the in-process selection seam constructs a Clef provider but does NOT
// enable it unless CLEF_ENABLED is explicitly set.
func TestNewProviderClefOffByDefault(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")

	p, err := newProvider("clef", "", "", time.Second)
	if err != nil {
		t.Fatalf("newProvider(clef): unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("newProvider(clef): got nil provider")
	}
	if got := p.Name(); got != clef.ProviderName {
		t.Fatalf("provider name = %q, want %q", got, clef.ProviderName)
	}

	if p.Available(context.Background()) {
		t.Fatal("clef provider reports Available=true without CLEF_ENABLED; must be OFF by default")
	}

	_, err = p.Decide(context.Background(), decision.DecisionRequest{})
	if err == nil {
		t.Fatal("Decide on disabled clef provider: expected error, got nil")
	}
	if !errors.Is(err, decision.ErrUnavailable) {
		t.Fatalf("Decide error = %v, want ErrUnavailable", err)
	}
}

// TestNewProviderClefMixedCase proves name resolution matches the existing
// lower/trim handling.
func TestNewProviderClefMixedCase(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")

	p, err := newProvider("  Clef ", "", "", time.Second)
	if err != nil {
		t.Fatalf("newProvider(Clef): unexpected error: %v", err)
	}
	if p == nil || p.Name() != clef.ProviderName {
		t.Fatalf("mixed-case clef did not resolve to the Clef provider")
	}
}

// TestNewProviderClefExplicitlyEnabled proves that with CLEF_ENABLED set, the
// clef provider is selected and constructed with Enable=true (off-by-default is
// opt-in only).
func TestNewProviderClefExplicitlyEnabled(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "1")
	t.Setenv("CLEF_MODEL", "clef-flash")
	t.Setenv("CLEF_TIMEOUT", "45s")

	p, err := newProvider("clef", "http://example.invalid:11434", "clef-override", 5*time.Second)
	if err != nil {
		t.Fatalf("newProvider(clef): unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("newProvider(clef): got nil provider")
	}
	if got := p.Name(); got != clef.ProviderName {
		t.Fatalf("provider name = %q, want %q", got, clef.ProviderName)
	}

	// The provider is explicitly enabled: Decide must not fail with ErrDisabled.
	// It may still fail for transport reasons (no live backend here), but the
	// failure must not be the disabled gate.
	_, err = p.Decide(context.Background(), decision.DecisionRequest{})
	if errors.Is(err, clef.ErrDisabled) {
		t.Fatalf("enabled clef provider returned ErrDisabled: %v", err)
	}
}

// TestNewProviderUnknownFailsClosed proves unknown providers fail closed rather
// than falling through to any default.
func TestNewProviderUnknownFailsClosed(t *testing.T) {
	p, err := newProvider("not-a-provider", "", "", time.Second)
	if err == nil {
		t.Fatal("unknown provider: expected error, got nil")
	}
	if p != nil {
		t.Fatalf("unknown provider: expected nil provider, got %v", p)
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("unknown provider error = %v, want unsupported provider", err)
	}
}

// TestAvailableProvidersIncludesClef pins the advertised selection list.
func TestAvailableProvidersIncludesClef(t *testing.T) {
	if !strings.Contains(availableProviders, "clef") {
		t.Fatalf("availableProviders = %q, want it to include \"clef\"", availableProviders)
	}
	if !strings.Contains(availableProviders, "nimble") || !strings.Contains(availableProviders, "julia") {
		t.Fatalf("availableProviders = %q, want it to still include nimble and julia", availableProviders)
	}
}

// TestNewProviderNimbleAndJuliaUnchanged proves the existing selection is
// preserved alongside the new clef case.
func TestNewProviderNimbleAndJuliaUnchanged(t *testing.T) {
	np, err := newProvider("nimble", "", "", time.Second)
	if err != nil || np == nil || np.Name() != "nimble" {
		t.Fatalf("newProvider(nimble) = %v, %v; want nimble provider", np, err)
	}

	jp, err := newProvider("julia", "", "", time.Second)
	if err != nil || jp == nil || jp.Name() != "julia" {
		t.Fatalf("newProvider(julia) = %v, %v; want julia provider", jp, err)
	}
}

// --- SP-006 additions -------------------------------------------------------

// TestDefaultProviderUnchangedWhenNothingSelected proves default preservation:
// selecting nothing (absent or empty -provider) resolves to the default nimble
// provider, not Clef.
func TestDefaultProviderUnchangedWhenNothingSelected(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "1") // even with Clef enabled, the default must not change

	for _, name := range []string{"", "   "} {
		t.Run("selection="+name, func(t *testing.T) {
			p, err := newProvider(name, "", "", time.Second)
			if err != nil {
				t.Fatalf("newProvider(%q): unexpected error: %v", name, err)
			}
			if p == nil || p.Name() != "nimble" {
				t.Fatalf("newProvider(%q) = %v, want default nimble provider", name, p)
			}
		})
	}
}

// TestFlagProviderDefaultIsNimble pins the -provider flag default as documented
// in usage() so an absent selection cannot silently drift to Clef. The default
// is observable through the flag description text emitted by usage().
func TestFlagProviderDefaultIsNimble(t *testing.T) {
	var buf strings.Builder
	usage(&buf)
	text := buf.String()
	if !strings.Contains(text, `-provider string`) {
		t.Fatalf("usage() does not document -provider: %q", text)
	}
	if !strings.Contains(text, `(default "nimble"`) {
		t.Fatalf("usage() does not document the -provider default as nimble: %q", text)
	}
}

// TestDisabledClefNotAccidentallySelected proves no-accidental-selection: with
// CLEF_ENABLED unset/empty/invalid the constructed Clef provider is never
// available and its Decide is gated by ErrDisabled/KindUnavailable.
func TestDisabledClefNotAccidentallySelected(t *testing.T) {
	for _, val := range []string{"", "0", "false", "no", "off", "nonsense"} {
		t.Run("CLEF_ENABLED="+val, func(t *testing.T) {
			t.Setenv("CLEF_ENABLED", val)
			p, err := newProvider("clef", "", "", time.Second)
			if err != nil {
				t.Fatalf("newProvider(clef): unexpected error: %v", err)
			}
			if p.Available(context.Background()) {
				t.Fatalf("CLEF_ENABLED=%q: clef reports Available=true, want disabled", val)
			}

			_, derr := p.Decide(context.Background(), decision.DecisionRequest{})
			if !errors.Is(derr, clef.ErrDisabled) {
				t.Fatalf("CLEF_ENABLED=%q: Decide err = %v, want clef.ErrDisabled", val, derr)
			}
			if !errors.Is(derr, decision.ErrUnavailable) {
				t.Fatalf("CLEF_ENABLED=%q: Decide err = %v, want ErrUnavailable", val, derr)
			}
		})
	}
}

// TestSelectingClefDoesNotEnableItNorAffectOthers proves selection is separate
// from enablement and is isolated from other providers.
func TestSelectingClefDoesNotEnableItNorAffectOthers(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")

	cp, err := newProvider("clef", "", "", time.Second)
	if err != nil || cp == nil || cp.Name() != clef.ProviderName {
		t.Fatalf("newProvider(clef) = %v, %v; want Clef provider", cp, err)
	}
	if cp.Available(context.Background()) {
		t.Fatal("selecting clef enabled it; it must stay disabled without CLEF_ENABLED")
	}

	// Other providers are unaffected by Clef selection.
	np, err := newProvider("nimble", "", "", time.Second)
	if err != nil || np == nil || np.Name() != "nimble" {
		t.Fatalf("newProvider(nimble) = %v, %v; want nimble provider", np, err)
	}
	jp, err := newProvider("julia", "", "", time.Second)
	if err != nil || jp == nil || jp.Name() != "julia" {
		t.Fatalf("newProvider(julia) = %v, %v; want julia provider", jp, err)
	}
}

// TestSelectionFailClosedPaths drives each fail-closed failure path through the
// supported surface (newProvider) against a model-free httptest backend. No
// live runtime is required.
func TestSelectionFailClosedPaths(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "1")

	t.Run("unavailable", func(t *testing.T) {
		// A server that has been closed refuses connections on its address,
		// which the transport classifies as a connectivity failure.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()

		p, err := newProvider("clef", url, "", 200*time.Millisecond)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		_, derr := p.Decide(context.Background(), clefChoiceRequest())
		assertDecisionKind(t, derr, decision.KindUnavailable)
	})

	t.Run("timeout", func(t *testing.T) {
		// The handler sleeps far longer than the client timeout, then returns
		// on its own so httptest.Server.Close cannot block on an active
		// connection. The client's short timeout fires first and yields a
		// connectivity (KindUnavailable) failure.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
		}))
		defer srv.Close()

		p, err := newProvider("clef", srv.URL, "", 20*time.Millisecond)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, derr := p.Decide(ctx, clefChoiceRequest())
		assertDecisionKind(t, derr, decision.KindUnavailable)
	})

	t.Run("malformed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "{not json")
		}))
		defer srv.Close()

		p, err := newProvider("clef", srv.URL, "", time.Second)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		_, derr := p.Decide(context.Background(), clefChoiceRequest())
		assertDecisionKind(t, derr, decision.KindMalformedResponse)
	})

	t.Run("invalid-request", func(t *testing.T) {
		p, err := newProvider("clef", "http://127.0.0.1:0", "", time.Second)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		// State present but no questions -> request validation fails closed.
		_, derr := p.Decide(context.Background(), decision.DecisionRequest{State: "x"})
		assertDecisionKind(t, derr, decision.KindInvalidRequest)
	})

	t.Run("invalid-choice", func(t *testing.T) {
		p, err := newProvider("clef", "http://127.0.0.1:0", "", time.Second)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		// A choice question with no allowed choices is an invalid request.
		req := decision.DecisionRequest{
			State:     "deploying a schema migration during business hours",
			Questions: []decision.Question{{ID: "risk", Type: decision.QuestionChoice}},
		}
		_, derr := p.Decide(context.Background(), req)
		assertDecisionKind(t, derr, decision.KindInvalidRequest)
	})

	t.Run("unsupported-operation", func(t *testing.T) {
		p, err := newProvider("clef", "http://127.0.0.1:0", "", time.Second)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		req := decision.DecisionRequest{
			State:     "deploying a schema migration during business hours",
			Questions: []decision.Question{{ID: "confidence_score", Type: decision.QuestionScore}},
		}
		_, derr := p.Decide(context.Background(), req)
		if !errors.Is(derr, clef.ErrUnsupported) {
			t.Fatalf("Decide err = %v, want clef.ErrUnsupported", derr)
		}
		assertDecisionKind(t, derr, decision.KindInvalidRequest)
	})

	t.Run("provider-failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "boom")
		}))
		defer srv.Close()

		p, err := newProvider("clef", srv.URL, "", time.Second)
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		_, derr := p.Decide(context.Background(), clefChoiceRequest())
		assertDecisionKind(t, derr, decision.KindProviderFailure)
	})
}

// clefChoiceRequest is a minimal valid single-choice request.
func clefChoiceRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:       "risk",
			Type:     decision.QuestionChoice,
			Criteria: "deploying a schema migration during business hours",
			Choices:  []string{"LOW", "MEDIUM", "HIGH"},
		}},
	}
}

// assertDecisionKind asserts err is a *decision.ProviderError with the wanted
// kind and matches the corresponding sentinel.
func assertDecisionKind(t *testing.T, err error, want decision.ErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want ProviderError kind %q", want)
	}
	var perr *decision.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *decision.ProviderError", err)
	}
	if perr.Provider != clef.ProviderName {
		t.Errorf("ProviderError.Provider = %q, want %q", perr.Provider, clef.ProviderName)
	}
	if perr.Kind != want {
		t.Errorf("ProviderError.Kind = %q, want %q (err=%v)", perr.Kind, want, err)
	}
	var sentinel error
	switch want {
	case decision.KindUnavailable:
		sentinel = decision.ErrUnavailable
	case decision.KindMalformedResponse:
		sentinel = decision.ErrMalformedResponse
	case decision.KindInvalidRequest:
		sentinel = decision.ErrInvalidRequest
	case decision.KindProviderFailure:
		sentinel = decision.ErrProviderFailure
	}
	if sentinel != nil && !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(%v, %v) = false, want true", err, sentinel)
	}
}
