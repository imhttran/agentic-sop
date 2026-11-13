package toolharness

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/imhttran/agentic-sop/internal/redact"
	"github.com/imhttran/agentic-sop/internal/sensitivepath"
)

// IsolationEnv selects run_command isolation (LLM03,
// PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON). Isolation is required by default on
// every platform: where no backend exists the command is refused, never run
// unsandboxed. The operator override is "off" (also 0/false/no) and takes effect
// only with UnsandboxedAckEnv, so one inherited or stale variable cannot remove
// the control on its own. Config.IsolateCommands is the programmatic equivalent
// of "on" and wins.
const IsolationEnv = "SOP_AGENT_RUN_COMMAND_ISOLATION"

// UnsandboxedAckEnv must be set to an affirmative value together with the
// IsolationEnv off spellings before run_command may run without the OS sandbox.
// The exception is deliberate: an operator on a host where the backend cannot
// start opts out by asserting two variables, and an ambient value cannot do it
// alone.
const UnsandboxedAckEnv = "SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK"

// errIsolationUnavailable is a policy denial: isolation is required but no
// backend can enforce it, so the command never runs unsandboxed.
var errIsolationUnavailable = errors.New("run_command isolation unavailable")

// isOffValue reports whether an override value disables isolation. An
// unrecognized value leaves the default in place rather than silently disabling
// the sandbox.
func isOffValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// isAcknowledged reports whether raw is a deliberate acknowledgement of
// unsandboxed execution. Every other value, including an empty one, leaves
// isolation in effect, so a partial or misspelled setting fails safe.
func isAcknowledged(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// isolationOverrideOff reports whether the environment explicitly disables
// isolation, and the raw value, so the caller can echo what the operator set.
func isolationOverrideOff() (bool, string) {
	raw := strings.TrimSpace(os.Getenv(IsolationEnv))
	return isOffValue(raw), raw
}

// IsolationPolicyNotice is the one-line notice `sop` prints at startup when the
// environment asks to disable run_command isolation, so a stale or inherited
// value cannot change the control silently. It returns "" when isolation is in
// effect and no override was requested. A missing backend is not a notice: that
// path refuses the command instead, and the override has no effect there.
//
// An unacknowledged override gets its own notice, because the operator's intent
// did not take effect and the reason must be visible.
func IsolationPolicyNotice() string {
	if !hasBackend(runtime.GOOS) {
		return ""
	}
	off, raw := isolationOverrideOff()
	if !off {
		return ""
	}
	if !isAcknowledged(os.Getenv(UnsandboxedAckEnv)) {
		return fmt.Sprintf("[config] %s=%q was ignored: set %s=1 to confirm unsandboxed run_command execution, so isolation remains in effect", IsolationEnv, raw, UnsandboxedAckEnv)
	}
	return fmt.Sprintf("[config] %s=%q disables run_command isolation; commands run with policy checks only and a scrubbed credential environment", IsolationEnv, raw)
}

// backendFor maps a GOOS to the sandbox backend and the binary it needs. An empty
// name means the platform has no supported backend. sandboxArgv and the isolation
// decision both use it, so the platform table exists once.
func backendFor(goos string) (name, bin string) {
	switch goos {
	case "darwin":
		return "seatbelt", "sandbox-exec"
	case "linux":
		return "bubblewrap", "bwrap"
	}
	return "", ""
}

// hasBackend reports whether a platform can enforce isolation at all.
func hasBackend(goos string) bool {
	name, _ := backendFor(goos)
	return name != ""
}

// isolationDecision reports whether the command must run inside the OS sandbox.
// It is pure so the platform rule can be tested on any host.
//
// A platform with no backend always returns true: the caller then refuses the
// command through sandboxArgv, and no override can turn that into policy-only
// execution (Windows B1). Where a backend exists the default is true, the
// operator override can disable it only when it is also acknowledged, and
// Config.IsolateCommands always wins.
func isolationDecision(goos, override, ack string, isolateCommands bool) bool {
	if !hasBackend(goos) {
		return true
	}
	if isolateCommands {
		return true
	}
	if !isOffValue(override) {
		return true
	}
	return !isAcknowledged(ack)
}

// isolationRequested reports whether the command must run inside the OS sandbox.
func (h *Harness) isolationRequested() bool {
	return isolationDecision(runtime.GOOS, os.Getenv(IsolationEnv), os.Getenv(UnsandboxedAckEnv), h.cfg.IsolateCommands)
}

// isolate wraps argv in the platform sandbox: Seatbelt on macOS, bubblewrap
// on Linux. Both deny outbound network (loopback stays usable), deny writes
// outside the root and temp, deny reading the user's home credential
// locations, and always deny writes under <root>/.agent-sdlc.
// Each run gets its own throwaway GOCACHE, and the shared module cache is
// read-only (go.sum verifies its content), so one run cannot poison the Go
// caches of a later run. The caller must call cleanup after the command
// exits. GOCACHE is returned as extra environment for the child, never put on
// the command line, so policy-checked argv cannot be reinterpreted. The child
// environment drops credential-named variables: the sandbox already denies
// network and credential reads, so a sandboxed command cannot use them. It
// fails closed: no backend means no run.
// ponytail: every sandboxed build starts with a cold build cache; seeding it
// from a trusted snapshot would make it warm again.
func isolate(root string, argv []string) (wrapped, env []string, detail string, cleanup func(), err error) {
	prefix, detail, cache, err := sandboxArgv(root)
	if err != nil {
		return nil, nil, "", nil, err
	}
	env, dropped := sandboxEnv(cache)
	detail += fmt.Sprintf(" scrubbed_env=%d", dropped)
	return append(prefix, argv...), env, detail, func() { _ = os.RemoveAll(cache) }, nil
}

// scrubbedEnvironment is the inherited environment without any credential-named
// variable or credential-shaped value. It is used for every run_command child,
// sandboxed or not: under isolation the sandbox already denies network and
// credential reads, and when the operator explicitly disables isolation the
// child must still not inherit SOP's own credentials, because a model-authored
// command would read them straight from the environment. IsSecretName and Shapes
// are the shared vocabulary; names match as substrings and values by shape, so a
// few harmless entries are dropped too. The result is never nil: a nil child
// environment would mean "inherit everything", the opposite of this function.
func scrubbedEnvironment() (env []string, dropped int) {
	marker := redact.Fixed("[redacted]")
	inherited := os.Environ()
	env = make([]string, 0, len(inherited))
	for _, kv := range inherited {
		name, value, _ := strings.Cut(kv, "=")
		if controlEnvNames[name] || redact.IsSecretName(name) || redact.Shapes(value, marker) != value {
			dropped++
			continue
		}
		env = append(env, kv)
	}
	return env, dropped
}

// controlEnvNames are SOP control-plane switches. They never reach a
// model-controlled child: a nested SOP process must decide its own isolation
// instead of inheriting the parent's, and a model-authored command has no reason
// to see either value.
var controlEnvNames = map[string]bool{
	IsolationEnv:      true,
	UnsandboxedAckEnv: true,
}

// sandboxEnv is the sandboxed child environment: the scrubbed environment plus
// this run's throwaway GOCACHE, so one run cannot warm or poison the next.
func sandboxEnv(cache string) (env []string, dropped int) {
	env, dropped = scrubbedEnvironment()
	return append(env, "GOCACHE="+cache), dropped
}

// sandboxArgv builds the sandbox prefix and a fresh build-cache dir for one run.
func sandboxArgv(root string) (prefix []string, detail, cache string, err error) {
	name, bin := backendFor(runtime.GOOS)
	if name == "" {
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
	// Credential reads are denied only when the sandbox runs; an unresolvable
	// home fails closed here rather than leaving the command unsandboxed.
	denyRead, err := homeCredentials()
	if err != nil {
		return nil, "", "", err
	}
	var args []string
	switch name {
	case "seatbelt":
		args = []string{"-p", seatbeltProfile(canon, writableDirs(canon), denyRead)}
	case "bubblewrap":
		if err := refuseSymlinkedState(canon); err != nil {
			return nil, "", "", err
		}
		// A Unix socket under $HOME is connectable by the same user, so each one is
		// masked with /dev/null like a credential file. Fail closed if the scan cannot
		// complete within its bounds.
		socks, err := homeSocketPaths()
		if err != nil {
			return nil, "", "", err
		}
		args = bubblewrapArgs(canon, writableDirs(canon), append(denyRead, socks...))
	default:
		// A backendFor entry without an argv builder must fail closed rather than
		// silently run with another backend's arguments.
		return nil, "", "", fmt.Errorf("%w: no argv builder for backend %q", errIsolationUnavailable, name)
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

// credentialPath is one home-relative credential location resolved to an
// absolute path that exists on this host. dir selects a subpath deny (the whole
// directory) rather than a literal one (a single file).
type credentialPath struct {
	path string
	dir  bool
}

// homeCredentials resolves the fixed credential locations under the user's home
// directory and those named by environment variables. Entries that do not exist
// are skipped: there is nothing to hide. A home directory that cannot be
// resolved, or a set relative override, fails closed, matching the rest of the
// sandbox, so the command never runs unsandboxed.
// ponytail: the denies match paths, so a credential inside a writable dir (the
// root or temp, for example KUBECONFIG pointing into the repo) can be renamed,
// or its parent dir moved, and then read; failing closed on such paths would
// close that if it ever matters.
func homeCredentials() ([]credentialPath, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve home directory: %v", errIsolationUnavailable, err)
	}
	if strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("%w: cannot resolve home directory", errIsolationUnavailable)
	}
	var out []credentialPath
	add := func(p string, dir bool) {
		if _, err := os.Lstat(p); err != nil {
			return
		}
		out = append(out, credentialPath{path: p, dir: dir})
		// Deny the real path too, as the state dir does: a symlinked home or
		// credential directory would otherwise resolve outside the deny.
		if real, err := evalExisting(p); err == nil && real != p {
			out = append(out, credentialPath{path: real, dir: dir})
		}
	}
	for _, d := range sensitivepath.HomeCredentialDirs() {
		add(filepath.Join(home, d), true)
	}
	for _, f := range sensitivepath.HomeCredentialFiles() {
		add(filepath.Join(home, f), false)
	}
	envDirs, err := sensitivepath.EnvCredentialDirs(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errIsolationUnavailable, err)
	}
	envFiles, err := sensitivepath.EnvCredentialFiles(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errIsolationUnavailable, err)
	}
	for _, d := range envDirs {
		add(d, true)
	}
	for _, f := range envFiles {
		add(f, false)
	}
	return out, nil
}

const (
	// maxHomeSocketDepth bounds how deep the home walk goes, and
	// maxHomeSocketEntries bounds how many entries it visits. Passing the entry
	// bound fails closed, so the command is refused rather than run with a socket
	// unmasked; passing the depth bound only ends the walk, so a socket deeper than
	// that is not masked.
	maxHomeSocketDepth   = 4
	maxHomeSocketEntries = 20000
)

// homeSocketPaths lists the Unix sockets under the user's home directory, up to a
// bounded depth and entry count. The home path is resolved first, so a symlinked home
// is walked rather than treated as a single entry. Directory symlinks are followed
// (for example a runner's ~/.ghcup pointing at a toolchain outside home) with a
// visited set so a link cycle terminates; a symlink to a socket masks its real target,
// and a dangling symlink is skipped. Any other error, or a walk past the bounds, fails
// closed.
// ponytail: a socket created after the scan, or deeper than the depth bound, is not
// masked. Upgrade: mask at connect time, or hide the home tree.
func homeSocketPaths() ([]credentialPath, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("%w: cannot resolve home for socket masking", errIsolationUnavailable)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve home for socket masking: %v", errIsolationUnavailable, err)
	}
	var out []credentialPath
	entries := 0
	visited := map[string]bool{}
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth >= maxHomeSocketDepth {
			return nil
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		if visited[real] {
			return nil // a symlink cycle, or the same tree reached twice
		}
		visited[real] = true
		des, err := os.ReadDir(real)
		if err != nil {
			return err
		}
		for _, d := range des {
			entries++
			if entries > maxHomeSocketEntries {
				return fmt.Errorf("home has more than %d entries; socket masking cannot complete", maxHomeSocketEntries)
			}
			p := filepath.Join(real, d.Name())
			switch {
			case d.Type()&fs.ModeSocket != 0:
				out = append(out, credentialPath{path: p, dir: false})
			case d.IsDir():
				if err := walk(p, depth+1); err != nil {
					return err
				}
			case d.Type()&fs.ModeSymlink != 0:
				target, err := filepath.EvalSymlinks(p)
				if err != nil {
					continue // dangling: no target to connect to yet
				}
				info, err := os.Stat(target)
				if err != nil {
					return err
				}
				if info.IsDir() {
					if err := walk(target, depth+1); err != nil {
						return err
					}
					continue
				}
				if info.Mode()&fs.ModeSocket != 0 {
					out = append(out, credentialPath{path: target, dir: false})
				}
			}
		}
		return nil
	}
	if err := walk(home, 0); err != nil {
		return nil, fmt.Errorf("%w: home socket scan: %v", errIsolationUnavailable, err)
	}
	return out, nil
}

// bubblewrapArgs builds the Linux view: the host read-only, the writable dirs
// bound read-write, a private /tmp and an empty /run (hiding host sockets such
// as Docker's), the SOP state dir read-only, and a private network namespace
// in which only the sandbox's own loopback exists.
// ponytail: a Unix socket outside /tmp and /run that the bounded home scan
// does not reach (too deep, or outside $HOME) stays connectable; a deny-default
// view would close that.
func bubblewrapArgs(root string, writable []string, denyRead []credentialPath) []string {
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
	// Hidden after the root and writable binds so the empty directory (or
	// /dev/null) wins. Only existing entries are passed, and they exist in the
	// read-only root view, so the mount point is always present.
	for _, c := range denyRead {
		if c.dir {
			args = append(args, "--tmpfs", c.path)
		} else {
			args = append(args, "--ro-bind", "/dev/null", c.path)
		}
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
// re-allowed for loopback and the writable dirs. Credential reads are denied
// outright: because no file-read allow follows, the deny wins over the default
// for the command that could print, copy, or hardlink the content. The SOP
// state deny comes last so it wins over the root allow.
func seatbeltProfile(root string, writable []string, denyRead []credentialPath) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny network*)\n")
	// The login keychain is reached through the SecurityServer system service,
	// not by a file read. Deny the lookup so a sandboxed `security` cannot read
	// stored tokens. Certificate verification uses trustd instead, so it is
	// unaffected (measured with a public chain under this rule).
	b.WriteString("(deny mach-lookup (global-name \"com.apple.SecurityServer\"))\n")
	// (local ip ...) on network* would also match outbound sockets, so only
	// bind/inbound are keyed on the local address.
	b.WriteString("(allow network-bind network-inbound (local ip \"localhost:*\"))\n(allow network* (remote ip \"localhost:*\"))\n")
	// (deny network*) also denies AF_UNIX bind, which a sandboxed build
	// legitimately needs for its own test listeners, caches and dev servers; the
	// Linux backend allows it inside its private /tmp. Re-allow bind only for
	// socket paths under a writable dir. Outbound AF_UNIX stays denied, so a host
	// socket outside those dirs is still unreachable.
	for _, d := range writable {
		fmt.Fprintf(&b, "(allow network-bind (subpath %q))\n", d)
	}
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
	for _, c := range denyRead {
		if c.dir {
			fmt.Fprintf(&b, "(deny file-read* (subpath %q))\n", c.path)
		} else {
			fmt.Fprintf(&b, "(deny file-read* (literal %q))\n", c.path)
		}
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
