package ollamaagent

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// newEvidenceSink appends each executed command's captured evidence to path as one
// JSON object per line, so a task that requires raw output has SOP-owned evidence.
// It returns nil when path is empty or cannot be opened: a failure to open must not
// break the run — the requirement then simply cannot be satisfied, which the caller
// reports rather than fabricating evidence.
func newEvidenceSink(path string) func(toolharness.CommandEvidence) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	var mu sync.Mutex
	return func(ev toolharness.CommandEvidence) {
		record := struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
			Exit    int    `json:"exit"`
			Output  string `json:"output"`
		}{ev.Command, ev.Cwd, ev.Exit, ev.Output}
		data, err := json.Marshal(record)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = f.Write(append(data, '\n'))
	}
}
