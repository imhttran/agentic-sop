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

// TestNewProviderClefEnabledBySelection proves that naming clef is the opt-in:
// with CLEF_ENABLED unset, the selected Clef provider attempts to serve (it
// reaches the backend) instead of failing as disabled. Clef is still never the
// default provider; see TestDefaultProviderUnchangedWhenNothingSelected.
func TestNewProviderClefEnabledBySelection(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")
	srv, hits := countingServer(t)

	p, err := newProvider("clef", srv.URL, "", time.Second)
	if err != nil {
		t.Fatalf("newProvider(clef): unexpected error: %v", err)
	}
	if got := p.Name(); got != clef.ProviderName {
		t.Fatalf("provider name = %q, want %q", got, clef.ProviderName)
	}

	_, err = p.Decide(context.Background(), clefReq())
	if errors.Is(err, clef.ErrDisabled) {
		t.Fatalf("selected clef is disabled without CLEF_ENABLED: %v", err)
	}
	if *hits == 0 {
		t.Fatal("selected clef never reached the backend; it must attempt to serve")
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

// TestDisabledClefNotAccidentallySelected proves CLEF_ENABLED is still a kill
// switch: with an explicit off or an unparseable value the constructed Clef
// provider is never available and its Decide is gated by
// ErrDisabled/KindUnavailable (a typo fails closed).
func TestDisabledClefNotAccidentallySelected(t *testing.T) {
	for _, val := range []string{"0", "false", "no", "off", "nonsense"} {
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

// TestSelectingClefEnablesOnlyClef proves naming clef enables clef alone and is
// isolated from other providers, and that an explicit SOP_DECISION_FALLBACK=clef
// is also an opt-in.
func TestSelectingClefEnablesOnlyClef(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")

	// Other providers are unaffected by Clef selection.
	np, err := newProvider("nimble", "", "", time.Second)
	if err != nil || np == nil || np.Name() != "nimble" {
		t.Fatalf("newProvider(nimble) = %v, %v; want nimble provider", np, err)
	}
	jp, err := newProvider("julia", "", "", time.Second)
	if err != nil || jp == nil || jp.Name() != "julia" {
		t.Fatalf("newProvider(julia) = %v, %v; want julia provider", jp, err)
	}

	// Clef named only as the fallback is enabled too.
	primary, _ := countingServer(t)
	backup, backupHits := countingServer(t)
	t.Setenv("SOP_DECISION_FALLBACK", "clef")
	t.Setenv("DECIDER_URL", primary.URL)
	t.Setenv("DECIDER_FALLBACK_URL", backup.URL)
	fp, err := newProvider("nimble", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fp.Decide(context.Background(), clefReq())
	if *backupHits == 0 {
		t.Fatal("clef named as SOP_DECISION_FALLBACK never served; naming it must enable it")
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

// TestDeciderURLsRoutePrimaryAndFallbackSeparately proves DECIDER_URL feeds the
// primary and DECIDER_FALLBACK_URL feeds the backup, overriding the shared
// OLLAMA_BASE_URL, so Clef (oMLX) and Nimble (Ollama) can differ.
func TestDeciderURLsRoutePrimaryAndFallbackSeparately(t *testing.T) {
	var primaryHits, backupHits, sharedHits int
	count := func(n *int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*n++
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
	}
	primary, backup, shared := count(&primaryHits), count(&backupHits), count(&sharedHits)
	defer primary.Close()
	defer backup.Close()
	defer shared.Close()

	t.Setenv("CLEF_ENABLED", "1")
	t.Setenv("OLLAMA_BASE_URL", shared.URL)
	t.Setenv("DECIDER_URL", primary.URL)
	t.Setenv("DECIDER_FALLBACK_URL", backup.URL)
	t.Setenv("SOP_DECISION_FALLBACK", "nimble")

	p, err := newProvider("clef", "", "", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := decision.DecisionRequest{State: "s", Questions: []decision.Question{
		{ID: "q", Type: decision.QuestionChoice, Criteria: "c", Choices: []string{"a", "b"}},
	}}
	_, _ = p.Decide(context.Background(), req)

	if primaryHits == 0 || backupHits == 0 || sharedHits != 0 {
		t.Fatalf("hits primary=%d backup=%d shared=%d; want primary>0, backup>0, shared=0", primaryHits, backupHits, sharedHits)
	}
}

// TestDeciderModelsRoutePrimaryAndFallbackSeparately proves DECIDER_MODEL names the
// primary's model and DECIDER_FALLBACK_MODEL the backup's, overriding each
// provider's own *_MODEL, and that the -model flag still wins for the primary.
func TestDeciderModelsRoutePrimaryAndFallbackSeparately(t *testing.T) {
	var primaryBody, backupBody string
	capture := func(dst *string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			*dst = string(b)
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
	}
	primary, backup := capture(&primaryBody), capture(&backupBody)
	defer primary.Close()
	defer backup.Close()

	req := decision.DecisionRequest{State: "s", Questions: []decision.Question{
		{ID: "q", Type: decision.QuestionChoice, Criteria: "c", Choices: []string{"a", "b"}},
	}}
	run := func(flagModel string) {
		primaryBody, backupBody = "", ""
		p, err := newProvider("clef", "", flagModel, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = p.Decide(context.Background(), req)
	}

	t.Setenv("CLEF_ENABLED", "1")
	t.Setenv("CLEF_MODEL", "clef-own")
	t.Setenv("NIMBLE_MODEL", "nimble-own")
	t.Setenv("DECIDER_URL", primary.URL)
	t.Setenv("DECIDER_FALLBACK_URL", backup.URL)
	t.Setenv("SOP_DECISION_FALLBACK", "nimble")
	t.Setenv("DECIDER_MODEL", "primary-model")
	t.Setenv("DECIDER_FALLBACK_MODEL", "backup-model")

	run("")
	if !strings.Contains(primaryBody, "primary-model") || !strings.Contains(backupBody, "backup-model") {
		t.Fatalf("env models not used: primary=%q backup=%q", primaryBody, backupBody)
	}
	if strings.Contains(primaryBody+backupBody, "clef-own") || strings.Contains(primaryBody+backupBody, "nimble-own") {
		t.Fatalf("provider *_MODEL must not win over DECIDER_*_MODEL: primary=%q backup=%q", primaryBody, backupBody)
	}

	run("flag-model")
	if !strings.Contains(primaryBody, "flag-model") || strings.Contains(primaryBody, "primary-model") {
		t.Fatalf("-model flag must win for the primary: %q", primaryBody)
	}
}

// clefReq is a minimal valid choice request for exercising Decide.
func clefReq() decision.DecisionRequest {
	return decision.DecisionRequest{State: "s", Questions: []decision.Question{
		{ID: "q", Type: decision.QuestionChoice, Criteria: "c", Choices: []string{"a", "b"}},
	}}
}

// countingServer answers every request with a 500 and counts them, so a test can
// tell "Clef tried to serve" (hits > 0) from "Clef was disabled" (no request).
func countingServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}
