package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/imhttran/agentic-sop/internal/retrievalgate"
)

// runGate is the CTX-004 Retrieval Evaluation Gate entry point for operators. It compares
// the deterministic baseline (unranked lexical matches in stable-path order) against the
// structural-index + BM25 candidate over a hermetic, deterministic corpus, and prints
// RETRIEVAL_GATE = PASS or FAIL. It is model-free and read-only and writes no artifact. It
// exits non-zero when the gate does not pass, so callers can gate on the decision.
func runGate(args []string, stdout, stderr io.Writer, _ deps) int {
	sub := "retrieve"
	var rest []string
	if len(args) > 0 {
		sub, rest = args[0], args[1:]
	}
	switch sub {
	case "retrieve":
		return runGateRetrieve(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown gate: %s\n", sub)
		fmt.Fprintln(stderr, "usage: sop gate retrieve [--k N] [--json]")
		return exitUsage
	}
}

// runGateRetrieve evaluates the CTX-004 gate over the built-in corpus.
func runGateRetrieve(args []string, stdout, stderr io.Writer) int {
	k := 5
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--k":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: sop gate retrieve [--k N] [--json]")
				return exitUsage
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "gate: invalid --k %q\n", args[i])
				return exitUsage
			}
			k = n
		case "--json":
			asJSON = true
		default:
			fmt.Fprintln(stderr, "usage: sop gate retrieve [--k N] [--json]")
			return exitUsage
		}
	}

	rep := retrievalgate.Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), k)
	if asJSON {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "gate: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "%s\n", data)
	} else {
		fmt.Fprint(stdout, rep.String())
	}
	if rep.Decision != retrievalgate.Pass {
		return exitError
	}
	return exitOK
}
