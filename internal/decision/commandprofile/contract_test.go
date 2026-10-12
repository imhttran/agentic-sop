package commandprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
)

// contractRegistry holds the config-driven M3 profiles for the recorded adapter
// wire documents (reused from the JEV-206 cross-repository evidence). Recognition
// is structural; the provider name is a correlation hint.
func contractRegistry() *bridge.Registry {
	return bridge.NewRegistry(
		&bridge.Profile{Name: "julia", Version: 3, Kinds: map[string]bridge.KindSpec{
			"complexity": {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"low": decision.Low, "medium": decision.Medium, "high": decision.High, "human": decision.Human,
			}},
		}},
		&bridge.Profile{Name: "nimble", Version: 3, Kinds: map[string]bridge.KindSpec{
			"risk": {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"LOW": decision.Low, "MEDIUM": decision.Medium, "HIGH": decision.High,
			}},
		}},
	)
}

// loadFixture strictly decodes a recorded adapter wire document, proving the
// fixture uses exactly the connector's schema (no unknown fields).
func loadFixture(t *testing.T, name string) bridge.Result {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var res bridge.Result
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return res
}

func TestAdapterContractWireCompatibility(t *testing.T) {
	reg := contractRegistry()

	t.Run("julia-1 choice and capability semantics", func(t *testing.T) {
		b, err := reg.Build(decision.Request{UseCase: "complexity"})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		out, err := b.Interpret(loadFixture(t, "julia_choice.json"))
		if err != nil {
			t.Fatalf("Interpret: %v", err)
		}
		if out.Choice != decision.High || out.Confidence == nil || *out.Confidence != 0.9 {
			t.Fatalf("out = %+v, want HIGH/0.9", out)
		}
	})

	t.Run("nimble choice and capability semantics", func(t *testing.T) {
		b, err := reg.Build(decision.Request{UseCase: "risk"})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		out, err := b.Interpret(loadFixture(t, "nimble_choice.json"))
		if err != nil {
			t.Fatalf("Interpret: %v", err)
		}
		if out.Choice != decision.High || out.Confidence == nil || *out.Confidence != 0.8349416441131589 {
			t.Fatalf("out = %+v, want HIGH/0.8349", out)
		}
	})

	t.Run("legacy profile stamping by SOP (M3)", func(t *testing.T) {
		b, _ := reg.Build(decision.Request{UseCase: "complexity"})
		out, err := b.Interpret(loadFixture(t, "julia_choice.json"))
		if err != nil {
			t.Fatalf("Interpret: %v", err)
		}
		if out.Diagnostics["profile"] != "julia" || out.Diagnostics["profile_version"] != "3" {
			t.Fatalf("diagnostics = %v, want the SOP-stamped julia profile v3", out.Diagnostics)
		}
	})

	t.Run("provider/model correlation", func(t *testing.T) {
		// The julia wire document interpreted against the nimble profile is rejected:
		// the provider name must correlate with the recognized profile.
		b, _ := reg.Build(decision.Request{UseCase: "risk"})
		if _, err := b.Interpret(loadFixture(t, "julia_choice.json")); !errors.Is(err, decision.ErrInvalidResult) {
			t.Fatalf("correlation: err = %v, want ErrInvalidResult", err)
		}
	})

	t.Run("cardinality: a batched multi-answer result is rejected", func(t *testing.T) {
		b, _ := reg.Build(decision.Request{UseCase: "complexity"})
		res := bridge.Result{Provider: "julia", Answers: map[string]bridge.Answer{
			"complexity": {Type: bridge.TypeChoice, Choice: "high", Confidence: fp(0.9)},
			"extra":      {Type: bridge.TypeChoice, Choice: "low", Confidence: fp(0.9)},
		}}
		if _, err := b.Interpret(res); !errors.Is(err, decision.ErrInvalidResult) {
			t.Fatalf("cardinality: err = %v, want ErrInvalidResult", err)
		}
	})

	t.Run("answer type mismatch", func(t *testing.T) {
		b, _ := reg.Build(decision.Request{UseCase: "risk"})
		res := bridge.Result{Provider: "nimble", Answers: map[string]bridge.Answer{
			"risk": {Type: bridge.TypeBoolean, Probability: 0.9},
		}}
		if _, err := b.Interpret(res); !errors.Is(err, decision.ErrInvalidResult) {
			t.Fatalf("type mismatch: err = %v, want ErrInvalidResult", err)
		}
	})

	t.Run("unsupported/empty response fails closed", func(t *testing.T) {
		b, _ := reg.Build(decision.Request{UseCase: "complexity"})
		if _, err := b.Interpret(bridge.Result{Provider: "julia", Answers: map[string]bridge.Answer{}}); !errors.Is(err, decision.ErrIndeterminate) {
			t.Fatalf("empty: err = %v, want ErrIndeterminate", err)
		}
	})

	t.Run("malformed response is rejected", func(t *testing.T) {
		var res bridge.Result
		dec := json.NewDecoder(bytes.NewReader([]byte("{not json")))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&res); err == nil {
			t.Fatal("expected a decode error for malformed JSON")
		}
	})

	t.Run("no provider-name-only recognition", func(t *testing.T) {
		// A structurally invalid result with the matching provider name is rejected.
		b, _ := reg.Build(decision.Request{UseCase: "complexity"})
		res := bridge.Result{Provider: "julia", Answers: map[string]bridge.Answer{
			"complexity": {Type: bridge.TypeChoice, Choice: "invented", Confidence: fp(0.9)},
		}}
		if _, err := b.Interpret(res); !errors.Is(err, decision.ErrIndeterminate) {
			t.Fatalf("unknown choice: err = %v, want ErrIndeterminate", err)
		}
	})
}

func fp(f float64) *float64 { return &f }
