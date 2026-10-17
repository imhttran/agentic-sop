package cli

import (
	"os"
	"os/exec"
)

// trueExecutable returns a path to the system `true` command, which exits 0 and
// ignores its arguments — the stand-in RAG producer the RAG tests configure. The
// absolute location differs across platforms (`/usr/bin/true` on macOS and the
// BSDs, `/bin/true` on Linux), so resolve it from PATH instead of hardcoding one.
func trueExecutable() string {
	if p, err := exec.LookPath("true"); err == nil {
		return p
	}
	for _, p := range []string{"/bin/true", "/usr/bin/true"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "true"
}
