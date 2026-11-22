// Command egmmanifest is the operator front end for the experiment-local EGM sealed
// manifest checks. It is not wired into SOP and makes no network or model call.
//
//	egmmanifest verify -manifest FILE [-root DIR] [-tree DIR] [-vocab FILE]
//	egmmanifest seal -input FILE -output FILE
//
// verify prints counts and problem categories only. seal prints the output path, the
// SHA-256 of the exact output bytes, and the verification status of the file it wrote.
// Neither command prints manifest contents, defect descriptions, or vocabulary terms.
// Both exit non-zero on failure. seal creates no output when it fails, except that a
// post-publication verification failure leaves the sealed file for inspection.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

const usage = `usage:
  egmmanifest verify -manifest FILE [-root DIR] [-tree DIR] [-vocab FILE]
  egmmanifest seal -input FILE -output FILE`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "seal":
		return runSeal(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "sealed manifest path")
	root := fs.String("root", "", "optional artifact root")
	tree := fs.String("tree", "", "optional single-case exported tree")
	vocabPath := fs.String("vocab", "", "optional forbidden-term file, one term per line")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *manifestPath == "" {
		fmt.Fprintln(stderr, "egmmanifest: -manifest is required")
		return 2
	}

	m, err := egmmanifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	opts := egmmanifest.Options{ArtifactRoot: *root, TreeRoot: *tree}
	if *vocabPath != "" {
		terms, err := readTerms(*vocabPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		v, err := egmmanifest.NewVocabulary(terms)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		opts.Vocabulary = v
	}

	rep, err := egmmanifest.Verify(m, opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "cases=%d verified=%d problems=%d\n", rep.Cases, rep.Verified, len(rep.Problems))
	for _, p := range rep.Problems {
		fmt.Fprintln(stdout, egmmanifest.FormatProblem(p))
	}
	if !rep.OK() {
		return 1
	}
	return 0
}

// runSeal writes a canonical sealed manifest and reports only its path, the digest of the
// exact output bytes, and the verification status of the file it just wrote.
func runSeal(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("seal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "unsigned manifest path")
	output := fs.String("output", "", "sealed manifest path; must not exist")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		fmt.Fprintln(stderr, "egmmanifest: seal requires -input and -output")
		return 2
	}

	res, err := egmmanifest.SealFile(*input, *output)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	status := "ok"
	if m, err := egmmanifest.Load(res.OutputPath); err != nil {
		status = "FAIL"
		fmt.Fprintln(stderr, "egmmanifest: sealed output did not load:", err)
	} else if rep, err := egmmanifest.Verify(m, egmmanifest.Options{}); err != nil || !rep.OK() {
		status = "FAIL"
		fmt.Fprintln(stderr, "egmmanifest: sealed output did not verify")
	}

	fmt.Fprintf(stdout, "sealed=%s sha256=%s verify=%s\n", res.OutputPath, res.SHA256, status)
	if status != "ok" {
		return 1
	}
	return 0
}

func readTerms(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("egmmanifest: read vocabulary: %w", err)
	}
	var terms []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			terms = append(terms, line)
		}
	}
	return terms, nil
}
