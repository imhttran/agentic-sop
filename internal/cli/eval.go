package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/eval"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runEval runs every Markdown task in a directory through the local lifecycle
// and reports aggregate metrics: the harness benchmark entry point (plan T045).
// Comparison across review engines (plan T046) is then a matter of running the
// corpus under different configurations and comparing the metrics.
func runEval(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop eval DIR")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	corpus := args[0]
	if !filepath.IsAbs(corpus) {
		corpus = filepath.Join(dir, args[0])
	}
	files, err := filepath.Glob(filepath.Join(corpus, "*.md"))
	if err != nil {
		fmt.Fprintf(stderr, "eval: %v\n", err)
		return exitError
	}
	if len(files) == 0 {
		fmt.Fprintf(stderr, "eval: no .md task files in %s\n", corpus)
		return exitError
	}
	sort.Strings(files)

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "eval: %v\n", err)
		return exitError
	}
	a, err := d.newAgent(cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		fmt.Fprintf(stderr, "eval: %v\n", err)
		return exitError
	}

	specs := make(map[string]*taskfile.Spec, len(files))
	cases := make([]eval.Case, 0, len(files))
	for _, path := range files {
		spec, err := taskfile.Load(path)
		if err != nil {
			fmt.Fprintf(stderr, "eval: %v\n", err)
			return exitError
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		specs[name] = spec
		cases = append(cases, eval.Case{ID: name, Name: spec.Title})
	}

	ctx := context.Background()
	metrics, outcomes := eval.Run(ctx, cases, func(ctx context.Context, c eval.Case) eval.Outcome {
		start := time.Now()
		spec := specs[c.ID]
		rn, err := runpkg.New(dir, "eval-"+c.ID)
		if err != nil {
			return eval.Outcome{Case: c, Err: err}
		}
		_ = rn.Write("task.md", spec.Render())

		res, err := executeLifecycle(ctx, dir, cfg, a, d, spec, rn)
		out := eval.Outcome{
			Case:     c,
			Duration: time.Since(start),
			Cycles:   res.cycles,
			Findings: len(res.report.Findings),
			Decision: res.gate.Decision,
		}
		if err != nil {
			out.Err = err
		}
		return out
	})

	writeEval(stdout, metrics, outcomes)
	return exitOK
}

// writeEval renders the benchmark table and totals.
func writeEval(w io.Writer, m eval.Metrics, outcomes []eval.Outcome) {
	fmt.Fprintf(w, "%-16s %-12s %-7s %-9s %s\n", "case", "gate", "cycles", "findings", "duration")
	for _, out := range outcomes {
		gate := string(out.Decision)
		if out.Err != nil {
			gate = "ERROR"
		}
		fmt.Fprintf(w, "%-16s %-12s %-7d %-9d %s\n",
			out.Case.ID, gate, out.Cycles, out.Findings, out.Duration.Round(time.Millisecond))
	}
	fmt.Fprintf(w, "\ntotal: %d  passed: %d  failed: %d  needs_human: %d  errored: %d\n",
		m.Total, m.Passed, m.Failed, m.NeedsHuman, m.Errored)
	fmt.Fprintf(w, "success: %.0f%%  cycles: %d  findings: %d  duration: %s\n",
		m.SuccessRate()*100, m.Cycles, m.Findings, m.Duration.Round(time.Millisecond))
}
