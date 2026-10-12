package commandprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
)

// testRegistry is a provider-neutral demo profile (Julia-like) whose kind specs
// are config-driven data, not a provider-name branch.
func testRegistry() *bridge.Registry {
	thr := 0.5
	rng := [2]float64{0, 2}
	return bridge.NewRegistry(&bridge.Profile{Name: "julia", Version: 3, Kinds: map[string]bridge.KindSpec{
		"complexity": {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
			"low": decision.Low, "medium": decision.Medium, "high": decision.High, "human": decision.Human,
		}},
		"approve":   {Type: bridge.TypeBoolean, BooleanThreshold: &thr, TrueChoice: decision.High, FalseChoice: decision.Low},
		"magnitude": {Type: bridge.TypeScore, ScoreBands: []bridge.ScoreBand{{Max: 0, Choice: decision.Low}, {Max: 1, Choice: decision.Medium}, {Max: 2, Choice: decision.High}}, ScoreRange: &rng},
	}})
}

func testConnector(t *testing.T, mode string, timeout time.Duration) *Connector {
	t.Helper()
	t.Setenv("GO_WANT_ADAPTER_HELPER", "1")
	t.Setenv("ADAPTER_MODE", mode)
	c, err := New(Options{
		Registry: testRegistry(),
		Argv:     []string{os.Args[0], "-test.run=^TestAdapterHelperProcess$", "--"},
		Provider: "julia",
		Model:    "julia-1",
		Timeout:  timeout,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestAdapterConnectorMapsProfileEvidence(t *testing.T) {
	c := testConnector(t, "success", 10*time.Second)
	d, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity", Subject: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Choice != decision.High || d.Confidence != 0.9 {
		t.Fatalf("decision = %+v, want HIGH/0.9", d)
	}
}

func TestAdapterConnectorFailsClosed(t *testing.T) {
	cases := []struct {
		mode string
		want error
	}{
		{"malformed", decision.ErrInvalidResult},
		{"empty", decision.ErrProviderFailure},
		{"unknownfield", decision.ErrInvalidResult},
		{"invalid", decision.ErrInvalidResult},
		{"unavailable", decision.ErrProviderFailure},
		{"wrongprovider", decision.ErrInvalidResult},
		{"unmappedchoice", decision.ErrIndeterminate},
		{"wrongid", decision.ErrInvalidResult},
	}
	for _, tc := range cases {
		c := testConnector(t, tc.mode, 10*time.Second)
		if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity", Subject: "x"}); !errors.Is(err, tc.want) {
			t.Errorf("mode %q: err = %v, want %v", tc.mode, err, tc.want)
		}
	}
}

func TestAdapterConnectorRecognizesProfileStructurally(t *testing.T) {
	// A structurally invalid result is rejected even with a matching provider name:
	// recognition is not provider-name-only.
	c := testConnector(t, "typemismatch", 10*time.Second)
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity", Subject: "x"}); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("type mismatch: err = %v, want ErrInvalidResult", err)
	}
}

func TestAdapterConnectorEnforcesAnswerIdentity(t *testing.T) {
	c := testConnector(t, "wrongid", 10*time.Second)
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity", Subject: "x"}); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("wrong answer id: err = %v, want ErrInvalidResult", err)
	}
}

func TestAdapterConnectorDistinguishesCancelAndTimeout(t *testing.T) {
	// Timeout: a short connector timeout against a sleeping adapter.
	ct := testConnector(t, "sleep", 300*time.Millisecond)
	_, err := ct.Decide(context.Background(), decision.Request{UseCase: "complexity"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: err = %v, want a timeout classification", err)
	}

	// Cancellation: an already-canceled context.
	cc := testConnector(t, "sleep", 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = cc.Decide(ctx, decision.Request{UseCase: "complexity"})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancel: err = %v, want a cancellation classification", err)
	}
}

func TestAdapterConnectorBoundsSubprocess(t *testing.T) {
	c := testConnector(t, "oversize", 10*time.Second)
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity"}); !errors.Is(err, decision.ErrProviderFailure) {
		t.Fatalf("oversize: err = %v, want ErrProviderFailure", err)
	}
}

// TestAdapterHelperProcess is the deterministic fake adapter: when the connector
// execs the test binary with GO_WANT_ADAPTER_HELPER=1, it acts as the adapter.
func TestAdapterHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ADAPTER_HELPER") != "1" {
		return
	}
	mode := os.Getenv("ADAPTER_MODE")

	reqPath, provider := "", "julia"
	for i, a := range os.Args {
		if a == "-file" && i+1 < len(os.Args) {
			reqPath = os.Args[i+1]
		}
		if a == "-provider" && i+1 < len(os.Args) {
			provider = os.Args[i+1]
		}
	}

	id := "q1"
	if b, err := os.ReadFile(reqPath); err == nil {
		var r struct {
			Questions []struct {
				ID string `json:"id"`
			} `json:"questions"`
		}
		if json.Unmarshal(b, &r) == nil && len(r.Questions) > 0 {
			id = r.Questions[0].ID
		}
	}

	emit := func(answerID, choice string, extra string) {
		fmt.Printf(`{"provider":%q,"model":"julia-1","answers":{%q:{"type":"choice","choice":%q,"confidence":0.9%s}},"usage":{}}`, provider, answerID, choice, extra)
		os.Exit(0)
	}

	choice := os.Getenv("ADAPTER_CHOICE")
	if choice == "" {
		choice = "high"
	}

	switch mode {
	case "success":
		emit(id, choice, "")
	case "wrongid":
		emit("some-other-id", "high", "")
	case "typemismatch":
		fmt.Printf(`{"provider":%q,"model":"julia-1","answers":{%q:{"type":"boolean","probability":0.9}},"usage":{}}`, provider, id)
		os.Exit(0)
	case "unmappedchoice":
		emit(id, "weird", "")
	case "wrongprovider":
		fmt.Printf(`{"provider":"other","model":"x","answers":{%q:{"type":"choice","choice":"high","confidence":0.9}},"usage":{}}`, id)
		os.Exit(0)
	case "unknownfield":
		emit(id, "high", `,"surprise":true`)
	case "malformed":
		fmt.Print("{not json")
		os.Exit(0)
	case "empty":
		os.Exit(0)
	case "invalid":
		os.Exit(2)
	case "exit1":
		os.Exit(1)
	case "exit7":
		os.Exit(7)
	case "unavailable":
		fmt.Fprintln(os.Stderr, "adapter: provider unavailable")
		os.Exit(3)
	case "oversize":
		fmt.Print(strings.Repeat("x", maxOutputBytes+4096))
		os.Exit(0)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "grandchild":
		go func() {
			time.Sleep(10 * time.Second)
			if m := os.Getenv("ADAPTER_MARKER"); m != "" {
				_ = os.WriteFile(m, []byte("survived"), 0o644)
			}
		}()
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(1)
	}
}
