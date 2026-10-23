package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskresult"
)

const resultUsage = "usage: sop result publish <task-id> --kind PROGRESS|BLOCKER|TEST|COMPLETION --summary TEXT [--evidence PATH]... " +
	"[--by NAME] [--idempotency-key K] [--expect-attempt N] [--expected-plan-revision R]\n" +
	"       sop result list <task-id> [--json]"

// runResult dispatches `sop result publish|list`. Publishing records durable
// executor evidence: it never changes the task's lifecycle, never approves, and
// never triggers execution.
func runResult(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, resultUsage)
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	switch args[0] {
	case "publish":
		return runResultPublish(args[1:], dir, stdout, stderr)
	case "list":
		return runResultList(args[1:], dir, stdout, stderr)
	default:
		fmt.Fprintln(stderr, resultUsage)
		return exitUsage
	}
}

// publishOptions is the parsed `sop result publish` request.
type publishOptions struct {
	taskID               string
	kind                 string
	summary              string
	evidence             []string
	by                   string
	idempotencyKey       string
	expectAttempt        int
	expectedPlanRevision string
}

// runResultPublish records one publication bound to the live task, attempt, and
// plan revision.
func runResultPublish(args []string, dir string, stdout, stderr io.Writer) int {
	opts, ok := parsePublishArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	kind, known := taskresult.ParseKind(opts.kind)
	if !known {
		fmt.Fprintf(stderr, "result: unknown kind %q; want one of %s\n", opts.kind, kindList())
		return exitError
	}
	if !requireState(statePath(dir), stderr) {
		return exitError
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return exitError
	}
	task, err := st.Get(opts.taskID)
	st.Close()
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return exitError
	}

	res, err := taskresult.Publish(dir, taskresult.PublishInput{
		TaskID:               task.ID,
		Attempt:              task.Attempt,
		PlanRevision:         currentPlanRevision(dir),
		RunEpoch:             currentRunEpoch(dir, task.ID),
		Kind:                 kind,
		Summary:              opts.summary,
		Evidence:             opts.evidence,
		PublishedBy:          opts.by,
		IdempotencyKey:       opts.idempotencyKey,
		ExpectedAttempt:      opts.expectAttempt,
		ExpectedPlanRevision: opts.expectedPlanRevision,
	})
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "%s %s %s attempt=%d\n", res.Outcome, task.ID, kind, task.Attempt)
	if res.Reason != "" {
		fmt.Fprintf(stdout, "reason: %s\n", res.Reason)
	}
	if res.Publication != nil {
		fmt.Fprintf(stdout, "publication: %s\n", res.Publication.ID)
	}
	if res.Outcome != taskresult.OutcomeAccepted && res.Outcome != taskresult.OutcomeDuplicate {
		return exitError
	}
	return exitOK
}

// parsePublishArgs parses the publish flags. It accepts both `--flag value` and
// `--flag=value`.
func parsePublishArgs(args []string, stderr io.Writer) (publishOptions, bool) {
	var opts publishOptions
	fail := func() (publishOptions, bool) {
		fmt.Fprintln(stderr, resultUsage)
		return publishOptions{}, false
	}
	for i := 0; i < len(args); i++ {
		key, inline, hasInline := strings.Cut(args[i], "=")
		value := func() (string, bool) {
			if hasInline {
				return inline, true
			}
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		switch key {
		case "--kind":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.kind = v
		case "--summary":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.summary = v
		case "--evidence":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.evidence = append(opts.evidence, v)
		case "--by":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.by = v
		case "--idempotency-key":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.idempotencyKey = v
		case "--expect-attempt":
			v, ok := value()
			if !ok {
				return fail()
			}
			n, perr := strconv.Atoi(strings.TrimSpace(v))
			if perr != nil || n <= 0 {
				fmt.Fprintf(stderr, "result: --expect-attempt %q must be a positive integer\n", v)
				return fail()
			}
			opts.expectAttempt = n
		case "--expected-plan-revision":
			v, ok := value()
			if !ok {
				return fail()
			}
			opts.expectedPlanRevision = v
		default:
			if strings.HasPrefix(key, "-") || opts.taskID != "" {
				return fail()
			}
			opts.taskID = key
		}
	}
	if strings.TrimSpace(opts.taskID) == "" || strings.TrimSpace(opts.summary) == "" || strings.TrimSpace(opts.kind) == "" {
		return fail()
	}
	return opts, true
}

// runResultList prints a task's published results.
func runResultList(args []string, dir string, stdout, stderr io.Writer) int {
	taskID := ""
	jsonOut := false
	for _, a := range args {
		switch {
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, resultUsage)
			return exitUsage
		default:
			taskID = a
		}
	}
	if strings.TrimSpace(taskID) == "" {
		fmt.Fprintln(stderr, resultUsage)
		return exitUsage
	}
	results, err := taskresult.List(dir, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "result: %v\n", err)
		return exitError
	}
	if jsonOut {
		data, merr := json.MarshalIndent(results, "", "  ")
		if merr != nil {
			fmt.Fprintf(stderr, "result: %v\n", merr)
			return exitError
		}
		fmt.Fprintln(stdout, string(data))
		return exitOK
	}
	if len(results) == 0 {
		fmt.Fprintf(stdout, "no published results for %s\n", taskID)
		return exitOK
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "%s %s attempt=%d by=%s: %s\n", r.Kind, r.ID, r.Attempt, oneLine(r.PublishedBy), oneLine(r.Summary))
		for _, e := range r.Evidence {
			fmt.Fprintf(stdout, "  evidence: %s\n", e)
		}
	}
	return exitOK
}

// kindList renders the closed kind vocabulary for diagnostics.
func kindList() string {
	kinds := taskresult.AllKinds()
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}
