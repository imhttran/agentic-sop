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

// isolate wraps argv in the platform sandbox: Seatbelt on macOS, bubblewrap
// on Linux. Both deny outbound network (loopback stays usable), deny writes
// outside the root and temp, and always deny writes under <root>/.agent-sdlc.
// Each run gets its own throwaway GOCACHE, and the shared module cache is
// read-only (go.sum verifies its content), so one run cannot poison the Go
// caches of a later run. The caller must call cleanup after the command
// exits. GOCACHE is returned as extra environment for the child, never put on
// the command line, so policy-checked argv cannot be reinterpreted. It fails
// closed: no backend means no run.
// ponytail: every sandboxed build starts with a cold build cache; seeding it
// from a trusted snapshot would make it warm again.
func isolate(root string, argv []string) (wrapped, env []string, detail string, cleanup func(), err error) {
	prefix, detail, cache, err := sandboxArgv(root)
	if err != nil {
		return nil, nil, "", nil, err
	}
	// Both sandbox-exec and bwrap (no --clearenv) pass the environment through.
	env = append(os.Environ(), "GOCACHE="+cache)
	return append(prefix, argv...), env, detail, func() { _ = os.RemoveAll(cache) }, nil
}

// sandboxArgv builds the sandbox prefix and a fresh build-cache dir for one run.
func sandboxArgv(root string) (prefix []string, detail, cache string, err error) {
	name, bin := "seatbelt", "sandbox-exec"
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		name, bin = "bubblewrap", "bwrap"
	default:
		return nil, "", "", fmt.Errorf("%w: no sandbox backend on %s", errIsolationUnavailable, runtime.GOOS)
	}
	tool, err := exec.LookPath(bin)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: %s not found", errIsolationUnavailable, bin)
	}
	// Both backends match real paths (/var -> /private/var on macOS), so
	// canonicalize and fail closed when the root cannot be resolved.
	canon, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: cannot canonicalize root: %v", errIsolationUnavailable, err)
	}
	var args []string
	if name == "seatbelt" {
		args = []string{"-p", seatbeltProfile(canon, writableDirs(canon))}
	} else {
		if err := refuseSymlinkedState(canon); err != nil {
			return nil, "", "", err
		}
		args = bubblewrapArgs(canon, writableDirs(canon))
	}
	// Under the temp dir, which both backends keep writable (on Linux it is
	// the sandbox's private /tmp, so the host dir only reserves the name).
	if cache, err = os.MkdirTemp("", "sop-gocache-"); err != nil {
		return nil, "", "", fmt.Errorf("%w: cannot create build cache: %v", errIsolationUnavailable, err)
	}
	sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
	detail = fmt.Sprintf("isolate=%s backend=%s root=%s profile=%s", name, tool, canon, hex.EncodeToString(sum[:]))
	return append([]string{tool}, args...), detail, cache, nil
}

// refuseSymlinkedState fails closed when <root>/.agent-sdlc is a symlink. The
// root is writable inside the bubblewrap view, so the command could replace the
// link itself and redirect SOP to a forged state dir. Seatbelt needs no such
// check: it denies writes on the link path.
func refuseSymlinkedState(root string) error {
	if fi, err := os.Lstat(filepath.Join(root, ".agent-sdlc")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: .agent-sdlc is a symlink", errIsolationUnavailable)
	}
	return nil
}

// bubblewrapArgs builds the Linux view: the host read-only, the writable dirs
// bound read-write, a private /tmp and an empty /run (hiding host sockets such
// as Docker's), the SOP state dir read-only, and a private network namespace
// in which only the sandbox's own loopback exists.
// ponytail: a Unix socket outside /tmp and /run (for example under $HOME)
// stays connectable; a deny-default view would close that.
func bubblewrapArgs(root string, writable []string) []string {
	args := []string{
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--tmpfs", "/run",
	}
	for _, d := range writable {
		// The private /tmp and /run tmpfs mounts are already writable; binding a
		// host path under them would re-expose host sockets. The root is always
		// bound: a root under /tmp would otherwise be hidden by the tmpfs.
		if d != root && underAny(d, "/tmp", "/run", "/var/run") {
			continue
		}
		args = append(args, "--bind-try", d, d)
	}
	// Mounted after the root bind so it wins, and the mount point cannot be
	// unlinked or renamed (isolate refuses a symlinked state dir). A missing
	// state dir gets an empty read-only tmpfs so the command cannot create one.
	state := filepath.Join(root, ".agent-sdlc")
	if _, err := os.Lstat(state); err == nil {
		args = append(args, "--ro-bind", state, state)
	} else {
		args = append(args, "--tmpfs", state, "--remount-ro", state)
	}
	return append(args,
		"--unshare-net", "--unshare-pid", "--unshare-ipc",
		"--die-with-parent", "--new-session",
		"--chdir", root,
		"--")
}

// writableDirs are the canonical directories a build or test needs to write:
// the root and the temp dirs (the per-run GOCACHE lives under them). The Go
// caches are deliberately absent. Relative or unresolvable entries are dropped
// (narrower, never wider).
func writableDirs(root string) []string {
	dirs := []string{root, os.TempDir()}
	if runtime.GOOS == "darwin" {
		dirs = append(dirs, "/private/tmp", "/private/var/folders")
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

// systemAlias maps the macOS /var, /tmp and /etc symlinks to their real paths
// (they are real directories elsewhere).
func systemAlias(p string) string {
	if runtime.GOOS != "darwin" {
		return p
	}
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

// underAny reports whether p is one of dirs or below one of them.
func underAny(p string, dirs ...string) bool {
	for _, d := range dirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}
