package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/repoindex"
)

// runIndex builds the deterministic Structural Repository Index and writes it beside the
// other SOP state, then prints a summary. It is model-free and read-only: it runs no task
// and invokes no model. It indexes the current working tree and records the repository
// HEAD and dirty flag in the index identity.
func runIndex(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop index")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	head, dirty := repoState(dir)
	idx, err := repoindex.Build(repoindex.Options{Root: dir, Head: head, Dirty: dirty})
	if err != nil {
		fmt.Fprintf(stderr, "index: %v\n", err)
		return exitError
	}
	data, err := idx.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "index: %v\n", err)
		return exitError
	}
	path := filepath.Join(dir, stateDirName, repoindex.FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "index: %v\n", err)
		return exitError
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "index: %v\n", err)
		return exitError
	}

	printIndexSummary(stdout, idx)
	fmt.Fprintf(stdout, "wrote %s\n", filepath.ToSlash(filepath.Join(stateDirName, repoindex.FileName)))
	return exitOK
}

// repoState returns the repository HEAD and whether the working tree is dirty. A missing
// git repository is not an error here: the index simply records no HEAD.
func repoState(dir string) (head string, dirty bool) {
	g := git.New(dir)
	ctx := context.Background()
	if err := g.ValidateRepository(ctx); err != nil {
		return "", false
	}
	head, _ = g.Head(ctx)
	if st, err := g.Status(ctx); err == nil {
		dirty = st != git.Clean
	}
	return head, dirty
}

// printIndexSummary prints the deterministic index identity and counts.
func printIndexSummary(w io.Writer, idx repoindex.Index) {
	c := idx.Counts
	head := idx.Identity.Head
	if head == "" {
		head = "(no HEAD)"
	}
	fmt.Fprintf(w, "Index identity: %s (schema %d, head %s, dirty=%v)\n", idx.Identity.Digest, idx.Identity.SchemaVersion, head, idx.Identity.Dirty)
	fmt.Fprintf(w, "  modules: %d  packages: %d  files: %d\n", c.Modules, c.Packages, c.Files)
	fmt.Fprintf(w, "  types: %d  structs: %d  interfaces: %d  functions: %d  methods: %d  imports: %d  tests: %d\n",
		c.Types, c.Structs, c.Interfaces, c.Functions, c.Methods, c.Imports, c.Tests)
	fmt.Fprintf(w, "  documents: %d (current %d, historical %d, unknown %d)\n", c.Documents, c.Current, c.Historical, c.Unknown)
}
