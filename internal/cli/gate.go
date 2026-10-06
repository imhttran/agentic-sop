package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/imhttran/agentic-sop/internal/orchadopt"
	"github.com/imhttran/agentic-sop/internal/retrievalgate"
	"github.com/imhttran/agentic-sop/internal/vectoreval"
)

// runGate is the Phase 8 evidence-gate entry point for operators. Subcommands:
//
//	retrieve       the CTX-004 Retrieval Evaluation Gate (baseline vs structural + BM25)
//	vector         the CTX-009 Vector Retrieval Evaluation (BM25 vs vector vs hybrid)
//	orchestration  the ORCH-012 Phase 9 Multi-Agent Adoption Gate (single-agent vs multi-agent)
//
// Both are model-free and read-only and write no artifact. `gate retrieve` exits
// non-zero when retrieval does not pass; `gate vector` reports its ADOPT/REJECT
// decision and always exits zero, because a rejection is a valid outcome.
func runGate(args []string, stdout, stderr io.Writer, _ deps) int {
	sub := "retrieve"
	var rest []string
	if len(args) > 0 {
		sub, rest = args[0], args[1:]
	}
	switch sub {
	case "retrieve":
		return runGateRetrieve(rest, stdout, stderr)
	case "vector":
		return runGateVector(rest, stdout, stderr)
	case "orchestration":
		return runGateOrchestration(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown gate: %s\n", sub)
		fmt.Fprintln(stderr, "usage: sop gate retrieve|vector|orchestration [--k N] [--json]")
		return exitUsage
	}
}

// runGateRetrieve evaluates the CTX-004 gate over the built-in corpus.
func runGateRetrieve(args []string, stdout, stderr io.Writer) int {
	k, asJSON, ok := parseEvidenceGateArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	rep := retrievalgate.Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), k)
	if asJSON {
		if !writeGateJSON(stdout, stderr, rep) {
			return exitError
		}
	} else {
		fmt.Fprint(stdout, rep.String())
	}
	if rep.Decision != retrievalgate.Pass {
		return exitError
	}
	return exitOK
}

// runGateVector evaluates the CTX-009 gate over the built-in corpus. A REJECT is a
// successful outcome, so it always exits zero.
func runGateVector(args []string, stdout, stderr io.Writer) int {
	k, asJSON, ok := parseEvidenceGateArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	rep := vectoreval.Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), k)
	if asJSON {
		if !writeGateJSON(stdout, stderr, rep) {
			return exitError
		}
		return exitOK
	}
	fmt.Fprint(stdout, rep.String())
	return exitOK
}

// runGateOrchestration evaluates the ORCH-012 Phase 9 multi-agent adoption gate over
// the built-in corpus. It is deterministic and model-free. A REJECT is a successful
// outcome (default execution remains single-agent), so it always exits zero.
func runGateOrchestration(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		fmt.Fprintln(stderr, "usage: sop gate orchestration [--json]")
		return exitUsage
	}
	rep := orchadopt.Evaluate(orchadopt.Corpus())
	if asJSON {
		if !writeGateJSON(stdout, stderr, rep) {
			return exitError
		}
		return exitOK
	}
	fmt.Fprint(stdout, rep.String())
	return exitOK
}

// parseEvidenceGateArgs parses the shared --k and --json flags.
func parseEvidenceGateArgs(args []string, stderr io.Writer) (k int, asJSON, ok bool) {
	k = 5
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--k":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: sop gate retrieve|vector [--k N] [--json]")
				return 0, false, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "gate: invalid --k %q\n", args[i])
				return 0, false, false
			}
			k = n
		case "--json":
			asJSON = true
		default:
			fmt.Fprintln(stderr, "usage: sop gate retrieve|vector [--k N] [--json]")
			return 0, false, false
		}
	}
	return k, asJSON, true
}

// writeGateJSON prints rep as indented JSON, reporting success.
func writeGateJSON(stdout, stderr io.Writer, rep any) bool {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return false
	}
	fmt.Fprintf(stdout, "%s\n", data)
	return true
}
