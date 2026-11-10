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
// A path that does not exist yet (a fresh GOCACHE) keeps its canonical
// existing ancestor plus the remainder; relative or unresolvable entries are
// dropped (narrower, never wider).
func writableDirs(root string) []string {
	dirs := []string{root, os.TempDir(), "/private/tmp", "/private/var/folders"}
	// ponytail: the shared Go build/module caches stay writable so builds are
	// not cold; a per-run throwaway GOCACHE/GOMODCACHE would remove cache
	// poisoning across runs at the cost of speed.
	if c, err := os.UserCacheDir(); err == nil { // default GOCACHE
		dirs = append(dirs, filepath.Join(c, "go-build"))
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
	out := []string{root}
	for _, d := range dirs[1:] {
		if !filepath.IsAbs(d) {
			continue
		}
		// Refuse a path that crosses a symlink (other than the macOS system
		// aliases): an earlier sandboxed run could have planted one in a
		// writable dir to widen this run's allow rule.
		if c, err := evalExisting(d); err == nil && c == systemAlias(filepath.Clean(d)) {
			out = append(out, c)
		}
	}
	return out
}

// systemAlias maps the macOS /var, /tmp and /etc symlinks to their real paths.
func systemAlias(p string) string {
	for _, a := range []string{"/var", "/tmp", "/etc"} {
		if p == a || strings.HasPrefix(p, a+"/") {
			return "/private" + p
		}
	}
	return p
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
	b.WriteString("(deny file-write*)\n")
	for _, d := range writable {
		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", d)
	}
	b.WriteString("(allow file-write-data (literal \"/dev/null\"))\n")
	// Deny both the path and its target: a symlinked .agent-sdlc would
	// otherwise resolve to a real path the root allow still covers.
	state := filepath.Join(root, ".agent-sdlc")
	fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", state)
	if real, err := evalExisting(state); err == nil && real != state {
		fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", real)
	}
	return b.String()
}
