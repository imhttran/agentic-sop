package toolharness

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// IsolationEnv opts run_command into the OS sandbox (LLM03,
// PROPOSAL-RUN-COMMAND-ISOLATION). Off by default; Config.IsolateCommands is
// the programmatic equivalent.
const IsolationEnv = "SOP_AGENT_RUN_COMMAND_ISOLATION"

// errIsolationUnavailable is a policy denial: isolation was requested but no
// backend can enforce it, so the command never runs unsandboxed.
var errIsolationUnavailable = errors.New("run_command isolation unavailable")

func (h *Harness) isolationRequested() bool {
	if h.cfg.IsolateCommands {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv(IsolationEnv)))
	return v == "1" || v == "true" || v == "yes"
}

// isolate wraps argv in sandbox-exec with a profile that denies network
// (except loopback) and denies writes outside the root, temp, and Go caches,
// and always denies writes under <root>/.agent-sdlc. It fails closed.
// ponytail: macOS Seatbelt only; Linux (bubblewrap) and other platforms fail
// closed until a backend is added.
func isolate(root string, argv []string) (wrapped []string, detail string, err error) {
	if runtime.GOOS != "darwin" {
		return nil, "", fmt.Errorf("%w: no sandbox backend on %s", errIsolationUnavailable, runtime.GOOS)
	}
	tool, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return nil, "", fmt.Errorf("%w: sandbox-exec not found", errIsolationUnavailable)
	}
	// Seatbelt matches real paths (/var -> /private/var), so canonicalize and
	// fail closed when the root cannot be resolved.
	canon, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", fmt.Errorf("%w: cannot canonicalize root: %v", errIsolationUnavailable, err)
	}
	profile := seatbeltProfile(canon, writableDirs(canon))
	sum := sha256.Sum256([]byte(profile))
	detail = fmt.Sprintf("isolate=seatbelt backend=%s root=%s profile=%s", tool, canon, hex.EncodeToString(sum[:]))
	return append([]string{tool, "-p", profile}, argv...), detail, nil
}

// writableDirs are the canonical directories a build or test needs to write.
// Unresolvable entries are dropped (narrower, never wider).
func writableDirs(root string) []string {
	dirs := []string{root, os.TempDir(), "/private/tmp", "/private/var/folders"}
	if c, err := os.UserCacheDir(); err == nil { // default GOCACHE lives here
		dirs = append(dirs, c)
	}
	for _, k := range []string{"GOCACHE", "GOMODCACHE"} {
		if v := os.Getenv(k); v != "" {
			dirs = append(dirs, v)
		}
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		dirs = append(dirs, filepath.Join(gopath, "pkg", "mod"))
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "pkg", "mod"))
	}
	var out []string
	for _, d := range dirs {
		if c, err := filepath.EvalSymlinks(d); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// seatbeltProfile is allow-default with network and file-write denied, then
// re-allowed for loopback and the writable dirs. The SOP state deny comes last
// so it wins over the root allow.
func seatbeltProfile(root string, writable []string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny network*)\n")
	// (local ip ...) on network* would also match outbound sockets, so only
	// bind/inbound are keyed on the local address.
	b.WriteString("(allow network-bind network-inbound (local ip \"localhost:*\"))\n(allow network* (remote ip \"localhost:*\"))\n")
	b.WriteString("(allow network* (remote unix-socket))\n")
	b.WriteString("(deny file-write*)\n")
	for _, d := range writable {
		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", d)
	}
	b.WriteString("(allow file-write-data (literal \"/dev/null\"))\n")
	fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", filepath.Join(root, ".agent-sdlc"))
	return b.String()
}
