package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/planner"
)

// runPlan reads PRD.md, asks the configured agent for a plan, then writes both
// PLAN.md (human) and .agent-sdlc/plan.json (machine). An existing PLAN.md is
// never overwritten, so a human-edited plan is safe.
func runPlan(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop plan")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	planPath := filepath.Join(dir, "PLAN.md")
	planExists, err := exists(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}
	if planExists {
		fmt.Fprintln(stderr, "PLAN.md already exists; refusing to overwrite")
		return exitError
	}

	prdPath := filepath.Join(dir, "PRD.md")
	prd, err := os.ReadFile(prdPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintln(stderr, "PRD.md not found in project directory")
			return exitError
		}
		fmt.Fprintf(stderr, "plan: read PRD.md: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(string(prd)) == "" {
		fmt.Fprintln(stderr, "PRD.md is empty")
		return exitError
	}

	a, err := d.newAgent()
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	plan, err := planner.New(a).Generate(context.Background(), string(prd))
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	if err := writePlanArtifacts(dir, plan); err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	fmt.Fprintln(stdout, "wrote PLAN.md")
	return exitOK
}

// writePlanArtifacts writes the human PLAN.md and the machine plan.json from
// the same validated Plan. Payloads are built first and each file is written to
// a temporary file and renamed, so a failure never leaves a half-written file.
func writePlanArtifacts(dir string, plan *planner.Plan) error {
	markdown := plan.RenderMarkdown()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan.json: %w", err)
	}
	data = append(data, '\n')

	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, "PLAN.md"), markdown); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(stateDir, "plan.json"), string(data)); err != nil {
		return err
	}
	return nil
}

// writeFileAtomic writes content to path via a temporary file and rename, so a
// failure part-way through does not leave a truncated file behind.
func writeFileAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// exists reports whether path exists, distinguishing "not found" from other
// filesystem errors. It is the single os.Stat wrapper for the CLI package.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
