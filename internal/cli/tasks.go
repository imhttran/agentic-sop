package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sdlc/internal/planner"
	"github.com/imhttran/agentic-sdlc/internal/store"
	"github.com/imhttran/agentic-sdlc/internal/taskbuilder"
)

// runTasks turns the machine plan (.agent-sdlc/plan.json) into persisted tasks.
// It is deterministic and never calls the agent.
func runTasks(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop tasks")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	stateDB := statePath(dir)
	if !requireState(stateDB, stderr) {
		return exitError
	}

	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintln(stderr, "plan.json not found; run `sop plan` first")
			return exitError
		}
		fmt.Fprintf(stderr, "tasks: read plan.json: %v\n", err)
		return exitError
	}

	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		fmt.Fprintf(stderr, "tasks: parse plan.json: %v\n", err)
		return exitError
	}

	st, err := store.Open(stateDB)
	if err != nil {
		fmt.Fprintf(stderr, "tasks: %v\n", err)
		return exitError
	}
	defer st.Close()

	tasks, err := taskbuilder.CreateTasksFromPlan(&plan, st.SaveTasks)
	if err != nil {
		fmt.Fprintf(stderr, "tasks: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "created %d task(s)\n", len(tasks))
	return exitOK
}
