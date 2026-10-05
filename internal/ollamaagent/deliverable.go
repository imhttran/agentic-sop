package ollamaagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// A declared deliverable is caller-owned task data: an exact repository path the
// task requires the agent to create (for example a Markdown report). SOP passes
// only the paths it has already validated as task-owned report deliverables. The
// harness uses them only to steer and gate. It never writes them itself, and a
// declared deliverable never substitutes for mutation evidence, validation, or
// review.

// deliverableTool reports whether a tool stays usable while a declared
// deliverable is still missing. File reads and writes remain available so the
// agent can compose the deliverable; command and git tools are withheld, because
// more validation is not what the invocation still owes.
func deliverableTool(name string) bool {
	switch name {
	case toolharness.ToolReadFile, toolharness.ToolWriteFile,
		toolharness.ToolCreateFile, toolharness.ToolListFiles,
		toolharness.ToolSearchFiles:
		return true
	}
	return false
}

// implementDeliverableInstruction names the deliverables the invocation must
// create and states that command tools are withheld until they exist.
func implementDeliverableInstruction(paths []string) string {
	var b strings.Builder
	b.WriteString("You have gathered enough context, but the required deliverable has not been created.\n\n")
	b.WriteString("Create the required file now with the file tools, before anything else:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nCommand and git tools are unavailable until the deliverable exists. The final structured outcome does not replace the file: this invocation is incomplete until the deliverable has been written to the repository.\n")
	return b.String()
}

// deliverableRequirement is the standing requirement placed in the task input
// when the task declares deliverables.
func deliverableRequirement(paths []string) string {
	var b strings.Builder
	b.WriteString("Required deliverable(s) -- create each with the file tools before returning your outcome:\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nThe final structured outcome is not a substitute for these files: the task is incomplete until they exist in the repository.\n")
	return b.String()
}

// deliverableMissing reports whether any declared deliverable is absent from the
// repository. It is read-only and fails closed (an unreadable path counts as
// missing). Most tasks declare no deliverable, so it is false and changes nothing.
func (h *Harness) deliverableMissing(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	root, err := os.OpenRoot(h.tools.Root())
	if err != nil {
		return true
	}
	defer root.Close()
	for _, p := range paths {
		f, err := root.Open(filepath.FromSlash(p))
		if err != nil {
			return true
		}
		f.Close()
	}
	return false
}
