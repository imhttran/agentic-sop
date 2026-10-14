// Package cli is the shared decision-adapter command implementation used by both
// `sop decision ...` and the legacy `sop-decision-adapter` executable.
//
// It is the decision provider layer's CLI surface, not the primary integration
// mechanism for agentic-sop: SOP reaches the adapter through the append-decide
// command connector (argv ["/abs/sop", "decision"] + "decide -file ... -json").
// The implementation is deliberately not a CLI framework: the "decide" and
// "shadow" subcommands, with room to grow.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/decision"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/clef"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/fallback"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/julia"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/nimble"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/shadow"
)

// Run dispatches one decision-adapter command. It writes normal output to stdout
// and diagnostics to stderr, and returns the process exit code. With no subcommand
// it prints usage to stderr and returns the usage exit code (the legacy contract).
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "decide":
		return runDecide(args[1:], stdout, stderr)
	case "shadow":
		return runShadow(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		usage(stderr)
		return 2
	}
}

func runDecide(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decide", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		providerName = fs.String("provider", "nimble", "decision provider to use")
		baseURL      = fs.String("base-url", "", "provider base URL (defaults to OLLAMA_BASE_URL)")
		model        = fs.String("model", "", "model name (defaults to NIMBLE_MODEL/JULIA_MODEL/CLEF_MODEL)")
		file         = fs.String("file", "", "path to a JSON DecisionRequest file")
		state        = fs.String("state", "", "free-form state/context (used when -file is not set)")
		choices      = fs.String("choices", "", "comma-separated allowed choices for a single choice question")
		decisionID   = fs.String("decision-id", "example", "question id for the single-question form")
		timeout      = fs.Duration("timeout", nimble.DefaultTimeout, "request timeout")
		asJSON       = fs.Bool("json", false, "print the DecisionResult as JSON")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	req, err := buildRequest(*file, *decisionID, *state, *choices)
	if err != nil {
		fmt.Fprintf(stderr, "invalid request: %v\n", err)
		return 2
	}
	if err := req.Validate(); err != nil {
		fmt.Fprintf(stderr, "invalid request: %v\n", err)
		return 2
	}

	provider, err := newProvider(*providerName, *baseURL, *model, *timeout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !provider.Available(ctx) {
		fmt.Fprintf(stderr, "warning: provider %q is not available\n", provider.Name())
	}

	result, err := provider.Decide(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, decision.ErrUnavailable):
			fmt.Fprintf(stderr, "provider unavailable: %v\n", err)
			return 3
		case errors.Is(err, decision.ErrInvalidRequest):
			fmt.Fprintf(stderr, "invalid request: %v\n", err)
			return 2
		default:
			fmt.Fprintf(stderr, "decide: %v\n", err)
			return 1
		}
	}

	printResult(stdout, result, *asJSON)
	return 0
}

// buildRequest loads a multi-question DecisionRequest from a JSON file when
// -file is set, otherwise builds a single-question request from flags.
func buildRequest(file, decisionID, state, choices string) (decision.DecisionRequest, error) {
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return decision.DecisionRequest{}, err
		}
		var req decision.DecisionRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return decision.DecisionRequest{}, fmt.Errorf("decode %s: %w", file, err)
		}
		return req, nil
	}

	choicesList := splitChoices(choices)
	if len(choicesList) == 0 {
		return decision.DecisionRequest{}, errors.New("-choices is required (or use -file)")
	}
	return decision.DecisionRequest{
		State: state,
		Questions: []decision.Question{{
			ID:       decisionID,
			Type:     decision.QuestionChoice,
			Criteria: state,
			Choices:  choicesList,
		}},
	}, nil
}

// availableProviders lists the provider names the CLI can select.
const availableProviders = "nimble, julia, clef"

// newProvider builds the selected provider and, when SOP_DECISION_FALLBACK names a
// different provider, wraps it in a primary->backup failover chain. The chain
// reuses the same provider implementations; it adds no new framework.
func newProvider(name, baseURL, model string, timeout time.Duration) (decision.Provider, error) {
	primary, err := buildProvider(name, baseURL, model, timeout)
	if err != nil {
		return nil, err
	}
	fb := strings.TrimSpace(os.Getenv("SOP_DECISION_FALLBACK"))
	if fb == "" || strings.EqualFold(fb, strings.TrimSpace(name)) {
		return primary, nil
	}
	backup, err := buildProvider(fb, "", "", timeout)
	if err != nil {
		return nil, err
	}
	return fallback.New(primary, backup), nil
}

// buildProvider resolves one provider by name.
func buildProvider(name, baseURL, model string, timeout time.Duration) (decision.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "nimble":
		cfg := nimble.ConfigFromEnv()
		if strings.TrimSpace(baseURL) != "" {
			cfg.BaseURL = baseURL
		}
		if strings.TrimSpace(model) != "" {
			cfg.Model = model
		}
		cfg.Timeout = timeout
		return nimble.New(cfg, nil), nil
	case "julia":
		cfg := julia.ConfigFromEnv()
		if strings.TrimSpace(model) != "" {
			cfg.Model = model
		}
		cfg.Timeout = timeout
		return julia.New(cfg, nil), nil
	case "clef":
		// Clef is OFF unless explicitly enabled via CLEF_ENABLED; selection
		// constructs the provider but does not enable it.
		cfg := clef.ConfigFromEnv()
		if strings.TrimSpace(baseURL) != "" {
			cfg.BaseURL = baseURL
		}
		if strings.TrimSpace(model) != "" {
			cfg.Model = model
		}
		cfg.Timeout = timeout
		return clef.New(cfg, nil), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (available: %s)", name, availableProviders)
	}
}

func printResult(w io.Writer, result decision.DecisionResult, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	fmt.Fprintf(w, "provider: %s\n", result.Provider)
	if result.Model != "" {
		fmt.Fprintf(w, "model:    %s\n", result.Model)
	}
	fmt.Fprintln(w, "answers:")
	for _, id := range sortedKeys(result.Answers) {
		answer := result.Answers[id]
		fmt.Fprintf(w, "  %s:\n", id)
		fmt.Fprintf(w, "    type:        %s\n", answer.Type)
		switch answer.Type {
		case decision.AnswerChoice:
			fmt.Fprintf(w, "    choice:      %s\n", answer.Choice)
		case decision.AnswerBoolean:
			fmt.Fprintf(w, "    probability: %.4f\n", answer.Probability)
		case decision.AnswerScore:
			fmt.Fprintf(w, "    score:       %.4f\n", answer.Score)
		}
		if answer.Confidence != nil {
			fmt.Fprintf(w, "    confidence:  %.4f\n", *answer.Confidence)
		}
	}
	if result.Usage.InputTokens != 0 || result.Usage.OutputTokens != 0 {
		fmt.Fprintf(w, "usage:    input=%d output=%d\n", result.Usage.InputTokens, result.Usage.OutputTokens)
	}
}

func splitChoices(raw string) []string {
	parts := strings.Split(raw, ",")
	choices := make([]string, 0, len(parts))
	for _, p := range parts {
		if c := strings.TrimSpace(p); c != "" {
			choices = append(choices, c)
		}
	}
	return choices
}

func sortedKeys(m map[string]decision.Answer) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runShadow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("shadow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		in  = fs.String("in", "", "path to a shadow run-sheet JSON file")
		out = fs.String("out", "", "path to write the evidence artifact (default: stdout)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*in) == "" {
		fmt.Fprintln(stderr, "shadow: -in is required")
		return 2
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(stderr, "shadow: read %s: %v\n", *in, err)
		return 2
	}
	var rs shadow.RunSheet
	if err := json.Unmarshal(data, &rs); err != nil {
		fmt.Fprintf(stderr, "shadow: decode %s: %v\n", *in, err)
		return 2
	}
	ev, err := shadow.ProduceEvidence(rs)
	if err != nil {
		fmt.Fprintf(stderr, "shadow: %v\n", err)
		return 2
	}
	encoded, err := shadow.MarshalEvidence(ev)
	if err != nil {
		fmt.Fprintf(stderr, "shadow: encode: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*out) == "" || *out == "-" {
		_, _ = stdout.Write(encoded)
		return 0
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(stderr, "shadow: write %s: %v\n", *out, err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprint(w, `decision-adapter - decision-model adapter CLI

Usage:
  sop decision decide [flags]
  sop decision shadow -in sheet.json [-out evidence.json]
  sop-decision-adapter decide [flags]
  sop-decision-adapter shadow -in sheet.json [-out evidence.json]

Flags (decide):
  -provider string    decision provider to use (default "nimble"; available: nimble, julia, clef)
  -base-url string    provider base URL (defaults to OLLAMA_BASE_URL)
  -model string       model name (defaults to NIMBLE_MODEL, JULIA_MODEL, or CLEF_MODEL)
  -file string        path to a JSON DecisionRequest file (multi-question)
  -state string       free-form state/context (single-question form)
  -choices string     comma-separated allowed choices (single-question form)
  -decision-id string question id for the single-question form (default "example")
  -timeout duration   request timeout (default 30s)
  -json               print the DecisionResult as JSON

Environment (nimble):
  OLLAMA_BASE_URL     Ollama-compatible backend root (default http://localhost:11434)
  NIMBLE_MODEL        Nimble model name (default "nimble")
  NIMBLE_TIMEOUT      request timeout (default 30s)

Environment (julia):
  JULIA_MODEL          Julia model identifier (default "julia")
  JULIA_MODEL_PATH     path to the ONNX model (required by the default helper;
                       forwarded as JULIA_MODEL_PATH)
  JULIA_PYTHON         interpreter for the default helper (default "python3")
  JULIA_INFERENCE_CMD  optional inference command overriding the default helper
  JULIA_TIMEOUT        inference timeout (default 30s)

  The default Julia path runs the repository-owned helper tools/julia/infer.py
  via JULIA_PYTHON. Set JULIA_INFERENCE_CMD to override it with a custom command.

Environment (clef):
  OLLAMA_BASE_URL     Ollama-compatible backend root (default http://localhost:11434)
  CLEF_MODEL          Clef model name (default "clef-flash")
  CLEF_TIMEOUT        request timeout (default 30s)
  CLEF_ENABLED        explicit opt-in; Clef is OFF unless set to 1/true/yes

  Clef is OFF by default and is only selected when enabled explicitly with
  -provider clef AND CLEF_ENABLED=1. Selecting it without CLEF_ENABLED yields
  an unavailable provider rather than serving decisions.

Examples:
  sop decision decide -provider nimble -file tests/fixtures/risk-evaluation.json
  sop decision decide -provider julia -file tests/fixtures/risk-evaluation.json
  CLEF_ENABLED=1 sop decision decide -provider clef -file tests/fixtures/risk-evaluation.json
  sop decision decide -decision-id risk -choices LOW,MEDIUM,HIGH \
    -state "deploying a schema migration during business hours"
`)
}
