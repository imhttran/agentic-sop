// Command egmmanifest is the operator front end for the experiment-local EGM sealed
// manifest checks. It is not wired into SOP and makes no network or model call.
//
//	egmmanifest verify -manifest FILE [-root DIR] [-tree DIR] [-vocab FILE]
//
// verify prints counts and problem categories only. It never prints manifest contents or
// vocabulary terms. It exits 1 on a load error or any verification problem.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "verify" {
		fmt.Fprintln(os.Stderr, "usage: egmmanifest verify -manifest FILE [-root DIR] [-tree DIR] [-vocab FILE]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	manifestPath := fs.String("manifest", "", "sealed manifest path")
	root := fs.String("root", "", "optional artifact root")
	tree := fs.String("tree", "", "optional single-case exported tree")
	vocabPath := fs.String("vocab", "", "optional forbidden-term file, one term per line")
	_ = fs.Parse(os.Args[2:])

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "egmmanifest: -manifest is required")
		os.Exit(2)
	}

	m, err := egmmanifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "egmmanifest:", err)
		os.Exit(1)
	}

	opts := egmmanifest.Options{ArtifactRoot: *root, TreeRoot: *tree}
	if *vocabPath != "" {
		terms, err := readTerms(*vocabPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "egmmanifest:", err)
			os.Exit(1)
		}
		v, err := egmmanifest.NewVocabulary(terms)
		if err != nil {
			fmt.Fprintln(os.Stderr, "egmmanifest:", err)
			os.Exit(1)
		}
		opts.Vocabulary = v
	}

	rep, err := egmmanifest.Verify(m, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "egmmanifest:", err)
		os.Exit(1)
	}
	fmt.Printf("cases=%d verified=%d problems=%d\n", rep.Cases, rep.Verified, len(rep.Problems))
	for _, p := range rep.Problems {
		fmt.Println(egmmanifest.FormatProblem(p))
	}
	if !rep.OK() {
		os.Exit(1)
	}
}

func readTerms(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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
