package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/decisionmemory"
	"github.com/imhttran/agentic-sop/internal/verifcache"
)

// runMemory is the CTX-010 Decision Memory operator surface. It records durable
// engineering decisions with provenance and scope, and lists the ones applicable to
// the current repository identity. It is model-free: it never runs an agent, and a
// decision recorded for another repository state is reported as stale, never applied.
func runMemory(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
	switch args[0] {
	case "add":
		return runMemoryAdd(args[1:], stdout, stderr, d)
	case "list":
		return runMemoryList(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "unknown memory command: %s\n", args[0])
		fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
}

const memoryUsage = "usage: sop memory add --decision <text> --reason <text> [--scope repository|project|architecture] [--source <id>] [--evidence <text>] | sop memory list [--scope <scope>]"

// runMemoryAdd records one decision.
func runMemoryAdd(args []string, stdout, stderr io.Writer, d deps) int {
	flags, ok := parseMemoryFlags(args, stderr)
	if !ok {
		return exitUsage
	}
	decision := strings.TrimSpace(flags["decision"])
	reason := strings.TrimSpace(flags["reason"])
	if decision == "" || reason == "" {
		fmt.Fprintln(stderr, "memory add: --decision and --reason are required")
		fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}

	scope := decisionmemory.Scope(strings.TrimSpace(flags["scope"]))
	if flags["scope"] == "" {
		scope = decisionmemory.ScopeRepository
	}
	if !scope.Valid() {
		fmt.Fprintf(stderr, "memory add: invalid --scope %q\n", flags["scope"])
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	head, dirty := repoState(dir)

	var evidence []string
	if e := strings.TrimSpace(flags["evidence"]); e != "" {
		evidence = []string{e}
	}
	source := strings.TrimSpace(flags["source"])
	if source == "" {
		source = "operator"
	}

	store := loadDecisionStore(dir)
	rec, err := store.Add(decisionmemory.Record{
		Decision:   decision,
		Scope:      scope,
		Reason:     reason,
		Evidence:   evidence,
		Repository: verifcache.RepositoryIdentity(head, dirty, ""),
		Source:     source,
	})
	if err != nil {
		fmt.Fprintf(stderr, "memory add: %v\n", err)
		return exitError
	}
	if err := saveDecisionStore(dir, store); err != nil {
		fmt.Fprintf(stderr, "memory add: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "Recorded decision (%s): %s\n", rec.Scope, rec.Decision)
	return exitOK
}

// runMemoryList prints the decisions applicable to the current repository identity and
// reports how many are stale (made under a different state and therefore not applied).
func runMemoryList(args []string, stdout, stderr io.Writer, d deps) int {
	flags, ok := parseMemoryFlags(args, stderr)
	if !ok {
		return exitUsage
	}
	var scope decisionmemory.Scope
	if flags["scope"] != "" {
		scope = decisionmemory.Scope(strings.TrimSpace(flags["scope"]))
		if !scope.Valid() {
			fmt.Fprintf(stderr, "memory list: invalid --scope %q\n", flags["scope"])
			return exitUsage
		}
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	head, dirty := repoState(dir)
	repository := verifcache.RepositoryIdentity(head, dirty, "")

	store := loadDecisionStore(dir)
	applicable, stale := 0, 0
	for _, r := range store.All() {
		if scope != "" && r.Scope != scope {
			continue
		}
		if r.Repository != repository {
			stale++
			continue
		}
		applicable++
		fmt.Fprintf(stdout, "%s  %s\n", r.Scope, r.Decision)
		fmt.Fprintf(stdout, "    reason: %s\n", r.Reason)
		if len(r.Evidence) > 0 {
			fmt.Fprintf(stdout, "    evidence: %s\n", strings.Join(r.Evidence, "; "))
		}
		fmt.Fprintf(stdout, "    source: %s\n", r.Source)
	}
	fmt.Fprintf(stdout, "%d applicable decision(s)\n", applicable)
	if stale > 0 {
		fmt.Fprintf(stdout, "%d stale decision(s) from another repository state (not applied)\n", stale)
	}
	return exitOK
}

// parseMemoryFlags parses --key value pairs. Unknown flags are rejected so a typo never
// silently records a decision with missing provenance.
func parseMemoryFlags(args []string, stderr io.Writer) (map[string]string, bool) {
	known := map[string]bool{"decision": true, "reason": true, "scope": true, "source": true, "evidence": true}
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			fmt.Fprintf(stderr, "memory: unexpected argument %q\n", arg)
			return nil, false
		}
		key := strings.TrimPrefix(arg, "--")
		value := ""
		if eq := strings.IndexByte(key, '='); eq >= 0 {
			key, value = key[:eq], key[eq+1:]
		} else {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "memory: --%s needs a value\n", key)
				return nil, false
			}
			i++
			value = args[i]
		}
		if !known[key] {
			fmt.Fprintf(stderr, "memory: unknown flag --%s\n", key)
			return nil, false
		}
		flags[key] = value
	}
	return flags, true
}

func decisionStorePath(dir string) string {
	return filepath.Join(dir, stateDirName, "context", "decisions.json")
}

func loadDecisionStore(dir string) *decisionmemory.Store {
	data, err := os.ReadFile(decisionStorePath(dir))
	if err != nil {
		return decisionmemory.New(0)
	}
	store, err := decisionmemory.Unmarshal(data)
	if err != nil {
		return decisionmemory.New(0)
	}
	return store
}

func saveDecisionStore(dir string, store *decisionmemory.Store) error {
	data, err := store.Marshal()
	if err != nil {
		return err
	}
	path := decisionStorePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
