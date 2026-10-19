// Command sop-memory ingests completed runs, rebuilds the local advisory index, and
// exercises the production advisory path for a task or failure. It never calls a model.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/cli"
	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/prompt"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "ingest":
		ingest(os.Args[2:])
	case "index":
		buildIndex(os.Args[2:])
	case "advise":
		advise(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  sop-memory ingest --runs .agent-sdlc/runs --out .agent-sdlc/memory/outcomes
  sop-memory index  --outcomes .agent-sdlc/memory/outcomes --db .agent-sdlc/memory/index.db
  sop-memory advise --dir . --task-title T --task-description D [--failure F] [--json]
`)
	os.Exit(2)
}

func ingest(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	runs := fs.String("runs", ".agent-sdlc/runs", "recorded run directory")
	out := fs.String("out", ".agent-sdlc/memory/outcomes", "authoritative outcome directory")
	repository := fs.String("repository", "", "repository identity (defaults to project.name)")
	fs.Parse(args)
	repo := *repository
	if repo == "" {
		if cfg, err := config.LoadDir("."); err == nil {
			repo = cfg.Project.Name
		}
	}
	if repo == "" {
		fail(fmt.Errorf("repository identity is required; pass --repository"))
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	entries, err := os.ReadDir(*runs)
	if err != nil {
		fail(err)
	}
	ingested, skipped := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rec, err := outcomefile.IngestRunFor(*runs, e.Name(), repo)
		if err != nil {
			skipped++
			continue
		}
		// Failed and blocked outcomes are failure history, not remedies.
		if rec.Status == outcomefile.StatusFailed || rec.Status == outcomefile.StatusBlocked {
			rec.Corpus = retrievalboundary.CorpusFailure
		}
		if _, _, err := outcomefile.Write(*out, rec); err != nil {
			skipped++
			continue
		}
		ingested++
	}
	fmt.Printf("ingested=%d skipped=%d out=%s\n", ingested, skipped, *out)
}

func buildIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	outcomes := fs.String("outcomes", ".agent-sdlc/memory/outcomes", "authoritative outcome directory")
	db := fs.String("db", ".agent-sdlc/memory/index.db", "SQLite index path")
	fs.Parse(args)
	ix, err := memindex.Open(*db)
	if err != nil {
		fail(err)
	}
	defer ix.Close()
	rep, err := ix.RebuildFromDir(context.Background(), *outcomes)
	if err != nil {
		fail(err)
	}
	fmt.Printf("indexed=%d excluded=%d rejected=%d db=%s\n", rep.Indexed, rep.Excluded, len(rep.Rejected), *db)
}

func advise(args []string) {
	fs := flag.NewFlagSet("advise", flag.ExitOnError)
	dir := fs.String("dir", ".", "project directory")
	title := fs.String("task-title", "", "current task title")
	desc := fs.String("task-description", "", "current task description")
	failure := fs.String("failure", "", "current failure context (enables the recovery phase)")
	asJSON := fs.Bool("json", false, "print JSON")
	fs.Parse(args)
	if strings.TrimSpace(*title) == "" {
		fail(fmt.Errorf("--task-title is required"))
	}
	cfg, err := config.LoadDir(*dir)
	if err != nil {
		fail(err)
	}
	spec := &taskfile.Spec{ID: "MEMORY-DEMO", Title: *title, Description: *desc}
	items, res := cli.AdvisoryKnowledgeForAttempt(context.Background(), *cfg, spec, cfg.Project.Name, *failure)

	// Build the same canonical context and compiled prompt the run uses, then check which
	// evidence identities actually appear.
	ctxItems := []sopctx.Item{{
		Source: sopctx.SourceExecution, Identity: "execution",
		Reason: "the current task", Priority: sopctx.PriorityExecution,
		Text: strings.TrimSpace(*title + "\n" + *desc),
	}}
	ctx := sopctx.FromInputs(sopctx.Inputs{TaskID: spec.ID, Execution: ctxItems, Knowledge: items}, sopctx.DefaultLimits())
	compiled := prompt.Compile(prompt.Input{
		Capability: agent.Implement, Task: spec.Render(), Context: ctx,
		OutputRequirements: "Describe the plan only. Do not execute.",
	})
	evidence := compiled.Evidence()
	included := []string{}
	for _, it := range items {
		if strings.Contains(evidence, prompt.ItemHeader(sopctx.SourceKnowledgeRAG, it.Identity)+"\n") {
			included = append(included, it.Identity)
		}
	}

	type outItem struct {
		Identity string `json:"identity"`
		Reason   string `json:"reason"`
		Text     string `json:"text"`
	}
	doc := map[string]any{
		"task":             *title,
		"failure":          *failure,
		"phase":            string(res.Phase),
		"status":           res.Status,
		"reason_code":      res.ReasonCode,
		"attempted":        res.Attempted,
		"items":            []outItem{},
		"included":         included,
		"prompt_bytes":     len(evidence),
		"retrieved_tokens": res.Tokens,
	}
	list := make([]outItem, 0, len(items))
	for _, it := range items {
		list = append(list, outItem{Identity: it.Identity, Reason: it.Reason, Text: it.Text})
	}
	doc["items"] = list
	if *asJSON {
		b, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Printf("current task: %s\nphase: %s  status: %s  reason: %s\n",
		taskLine(*title, *desc), res.Phase, res.Status, res.ReasonCode)
	if len(items) == 0 {
		fmt.Println("retrieved: nothing relevant found")
		return
	}
	fmt.Printf("retrieved %d item(s):\n", len(items))
	for _, it := range items {
		fmt.Printf("  - [%s] %s\n    %s\n", it.Identity, it.Reason, snippet(it.Text, 220))
	}
	fmt.Printf("included in compiled prompt: %d/%d\n", len(included), len(items))
}

func taskLine(title, desc string) string {
	s := strings.TrimSpace(title)
	if d := strings.TrimSpace(desc); d != "" {
		s += " — " + d
	}
	return s
}

func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "sop-memory: %v\n", err)
	os.Exit(1)
}
