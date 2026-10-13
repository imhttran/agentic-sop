// Command sop-decision-adapter is a minimal CLI for exercising the decision
// provider layer.
//
// It is a debugging and testing surface, not the primary integration mechanism
// for agentic-sop. It is deliberately not a CLI framework: one subcommand today
// ("decide"), with room to grow.
package main

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

	"github.com/imhttran/sop-decision-adapters/decision"
	"github.com/imhttran/sop-decision-adapters/internal/providers/clef"
	"github.com/imhttran/sop-decision-adapters/internal/providers/julia"
	"github.com/imhttran/sop-decision-adapters/internal/providers/nimble"
	"github.com/imhttran/sop-decision-adapters/internal/shadow"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "decide":
		os.Exit(runDecide(os.Args[2:]))
	case "shadow":
		os.Exit(runShadow(os.Args[2:]))
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func runDecide(args []string) int {
	fs := flag.NewFlagSet("decide", flag.ContinueOnError)
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
		fmt.Fprintf(os.Stderr, "invalid request: %v\n", err)
		return 2
	}
	if err := req.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid request: %v\n", err)
		return 2
	}

	provider, err := newProvider(*providerName, *baseURL, *model, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !provider.Available(ctx) {
		fmt.Fprintf(os.Stderr, "warning: provider %q is not available\n", provider.Name())
	}

	result, err := provider.Decide(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, decision.ErrUnavailable):
			fmt.Fprintf(os.Stderr, "provider unavailable: %v\n", err)
			return 3
		case errors.Is(err, decision.ErrInvalidRequest):
			fmt.Fprintf(os.Stderr, "invalid request: %v\n", err)
			return 2
		default:
			fmt.Fprintf(os.Stderr, "decide: %v\n", err)
			return 1
		}
	}

	printResult(os.Stdout, result, *asJSON)
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

// newProvider resolves the requested provider name. The switch is the seam for
// additional adapters.
func newProvider(name, baseURL, model string, timeout time.Duration) (decision.Provider, error) {
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

func runShadow(args []string) int {
	fs := flag.NewFlagSet("shadow", flag.ContinueOnError)
	var (
		in  = fs.String("in", "", "path to a shadow run-sheet JSON file")
		out = fs.String("out", "", "path to write the evidence artifact (default: stdout)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*in) == "" {
		fmt.Fprintln(os.Stderr, "shadow: -in is required")
		return 2
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shadow: read %s: %v\n", *in, err)
		return 2
	}
	var rs shadow.RunSheet
	if err := json.Unmarshal(data, &rs); err != nil {
		fmt.Fprintf(os.Stderr, "shadow: decode %s: %v\n", *in, err)
		return 2
	}
	ev, err := shadow.ProduceEvidence(rs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shadow: %v\n", err)
		return 2
	}
	encoded, err := shadow.MarshalEvidence(ev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shadow: encode: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*out) == "" || *out == "-" {
		_, _ = os.Stdout.Write(encoded)
		return 0
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "shadow: write %s: %v\n", *out, err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprint(w, `sop-decision-adapter - decision-model adapter CLI

Usage:
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
  sop-decision-adapter decide -provider nimble -file tests/fixtures/risk-evaluation.json
  sop-decision-adapter decide -provider julia -file tests/fixtures/risk-evaluation.json
  CLEF_ENABLED=1 sop-decision-adapter decide -provider clef -file tests/fixtures/risk-evaluation.json
  sop-decision-adapter decide -decision-id risk -choices LOW,MEDIUM,HIGH \
    -state "deploying a schema migration during business hours"
`)
}
