package toolharness

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// needSandbox skips unless this host can actually start the platform sandbox
// (sandbox-exec cannot nest; bwrap needs unprivileged user namespaces).
func needSandbox(t *testing.T) {
	t.Helper()
	name, bin := backendFor(runtime.GOOS)
	if name == "" {
		t.Skipf("no sandbox backend on %s", runtime.GOOS)
	}
	probe := map[string][]string{
		"seatbelt":   {bin, "-p", "(version 1)(allow default)", "true"},
		"bubblewrap": {bin, "--ro-bind", "/", "/", "--unshare-net", "true"},
	}[name]
	if probe == nil {
		t.Skipf("no start probe for backend %q", name)
	}
	if err := exec.Command(probe[0], probe[1:]...).Run(); err != nil {
		t.Skipf("%s cannot start here: %v", probe[0], err)
	}
}

// ncTimeout is the netcat connect-timeout flag (BSD nc on macOS, OpenBSD nc on Linux).
func ncTimeout() string {
	if runtime.GOOS == "darwin" {
		return "-G 2"
	}
	return "-w 2"
}

// sandboxed runs a shell snippet inside the run_command sandbox for root.
func sandboxed(t *testing.T, root, script string) error {
	t.Helper()
	_, err := sandboxedOutput(t, root, script)
	return err
}

// sandboxedOutput is sandboxed with the child's combined output.
func sandboxedOutput(t *testing.T, root, script string) (string, error) {
	t.Helper()
	argv, env, _, cleanup, err := isolate(root, []string{"sh", "-c", script})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestRunCommandIsolationOffKeepsExecCapturedPath(t *testing.T) {
	t.Setenv(IsolationEnv, "off")
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	if _, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."}); err != nil {
		t.Fatal(err)
	}
	if d := log.Records()[0].Detail; d != "" {
		t.Fatalf("isolation off: audit detail = %q, want empty", d)
	}
}

// The startup notice exists so an operator can see that the environment removed
// the control. An unrecognized value must not disable isolation.
func TestIsolationPolicyNotice(t *testing.T) {
	t.Setenv(IsolationEnv, "")
	if got := IsolationPolicyNotice(); got != "" {
		t.Fatalf("notice with the default in place = %q, want empty", got)
	}
	for _, v := range []string{"off", "0", "false", "no", "OFF"} {
		t.Setenv(IsolationEnv, v)
		got := IsolationPolicyNotice()
		if !hasBackend(runtime.GOOS) {
			// B1: the override has no effect where no backend exists.
			if got != "" {
				t.Fatalf("notice for %q on %s = %q, want none: the override cannot disable isolation here", v, runtime.GOOS, got)
			}
			continue
		}
		if !strings.Contains(got, IsolationEnv) || !strings.Contains(got, "policy checks only") {
			t.Fatalf("notice for %q = %q, want it to name the variable and the effect", v, got)
		}
	}
	for _, v := range []string{"bogus", "maybe", "2"} {
		t.Setenv(IsolationEnv, v)
		if got := IsolationPolicyNotice(); got != "" {
			t.Fatalf("an unrecognized value %q produced a notice: %q", v, got)
		}
	}
}

// Isolation is required by default: with no configuration and no override, a
// command runs inside the backend the platform provides.
func TestRunCommandIsolationIsOnByDefault(t *testing.T) {
	t.Setenv(IsolationEnv, "")
	needSandbox(t)
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if err != nil {
		t.Fatalf("the default run_command did not execute: %v (%s)", err, out)
	}
	if d := log.Records()[0].Detail; !strings.Contains(d, "isolate=") {
		t.Fatalf("the default run was not sandboxed: audit detail = %q", d)
	}
}

// Windows B1: a platform with no backend denies the command instead of running it
// unsandboxed. The empty PATH reproduces the unsupported-platform case on any host.
func TestRunCommandIsolationDefaultsToFailClosed(t *testing.T) {
	t.Setenv(IsolationEnv, "")
	t.Setenv("PATH", t.TempDir()) // no sandbox backend
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	// `go build` carries no path argument, so the isolation refusal is what this
	// exercises on every platform rather than command-policy path resolution.
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go build"})
	if !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
	if r := log.Records()[0]; r.Action != ActionDeny {
		t.Fatalf("audit action = %s, want deny", r.Action)
	}
}

func TestRunCommandIsolationOnFailsClosedWithoutBackend(t *testing.T) {
	t.Setenv(IsolationEnv, "1")
	t.Setenv("PATH", t.TempDir()) // no sandbox-exec on PATH
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go build"})
	if !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
	if r := log.Records()[0]; r.Action != ActionDeny {
		t.Fatalf("audit action = %s, want deny", r.Action)
	}
}

func TestRunCommandIsolationDoesNotFallBackWhenBackendFails(t *testing.T) {
	// A backend that exists but cannot start (a broken profile, a kernel that
	// denies unprivileged namespaces, AppArmor on Ubuntu 24.04) must fail the
	// command, never fall back to the unsandboxed path. The fake backend is first
	// on PATH; the real command is still resolvable, so a fallback would visibly
	// print its output.
	fake := t.TempDir()
	name := "sandbox-exec"
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		name = "bwrap"
	default:
		t.Skipf("no sandbox backend on %s", runtime.GOOS)
	}
	script := "#!/bin/sh\necho fake-sandbox-refused >&2\nexit 71\n"
	if err := os.WriteFile(filepath.Join(fake, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	// A fake HOME keeps the Linux home-socket scan off the developer's real tree.
	t.Setenv("HOME", t.TempDir())

	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	log := NewAuditLog(0)
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module fallbackprobe\n\ngo 1.21\n")
	writeRepoFile(t, root, "cred/cred.go", "package cred\n")
	h := New(root, cfg, log)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./cred"})
	if err != nil {
		t.Fatalf("the run_command tool returned an error instead of the backend failure: %v (%s)", err, out)
	}
	if strings.Contains(out, "fallbackprobe/cred") {
		t.Fatalf("the command ran unsandboxed after the backend failed: %q", out)
	}
	if !strings.Contains(out, "fake-sandbox-refused") {
		t.Fatalf("the backend failure was not surfaced: %q", out)
	}
}

func TestRunCommandIsolationOnWrapsArgvWithSandboxExec(t *testing.T) {
	needSandbox(t)
	log := NewAuditLog(0)
	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	h := New(t.TempDir(), cfg, log)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if err != nil || !strings.Contains(out, "exit") {
		t.Fatalf("isolated go version: %q, %v", out, err)
	}
	if d := log.Records()[0].Detail; (!strings.HasPrefix(d, "isolate=seatbelt backend=") && !strings.HasPrefix(d, "isolate=bubblewrap backend=")) || !strings.Contains(d, "profile=") {
		t.Fatalf("audit detail = %q, want the seatbelt backend and profile digest", d)
	}
}

func TestRunCommandIsolationDeniesStateDirWrite(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	if err := sandboxed(t, root, "echo x > ok.txt"); err != nil {
		t.Fatalf("root write was refused: %v", err)
	}
	// The write must land in the real root, not in a sandbox-only copy.
	if b, err := os.ReadFile(filepath.Join(root, "ok.txt")); err != nil || string(b) != "x\n" {
		t.Fatalf("root write did not reach the host: %q, %v", b, err)
	}
	if err := sandboxed(t, root, "echo forged > .agent-sdlc/state.db"); err == nil {
		t.Fatal("a subprocess write to .agent-sdlc succeeded")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".agent-sdlc/state.db")); string(b) != "state" {
		t.Fatalf("state.db changed: %q", b)
	}
	// Replacing the state dir itself (move it aside, plant a forged one) fails.
	if err := sandboxed(t, root, "mv .agent-sdlc moved && mkdir .agent-sdlc && echo forged > .agent-sdlc/state.db"); err == nil {
		t.Fatal("the state dir was replaced from inside the sandbox")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".agent-sdlc/state.db")); string(b) != "state" {
		t.Fatalf("state.db after replacement attempt: %q", b)
	}
	// A symlinked state dir is denied at its real path too.
	linked := t.TempDir()
	writeRepoFile(t, linked, "real-state/state.db", "state")
	if err := os.Symlink("real-state", filepath.Join(linked, ".agent-sdlc")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		// bubblewrap refuses a symlinked state dir outright.
		if _, _, _, _, err := isolate(linked, []string{"true"}); !errors.Is(err, errIsolationUnavailable) {
			t.Fatalf("symlinked state dir: err = %v, want errIsolationUnavailable", err)
		}
	} else if err := sandboxed(t, linked, "echo forged > real-state/state.db"); err == nil {
		t.Fatal("a write to the symlinked state dir target succeeded")
	}
	home, _ := os.UserHomeDir()
	if err := sandboxed(t, root, "touch "+filepath.Join(home, ".sop-isolation-probe")); err == nil {
		os.Remove(filepath.Join(home, ".sop-isolation-probe"))
		t.Fatal("a write outside the writable dirs succeeded")
	}
}

func TestRunCommandIsolationDeniesNetwork(t *testing.T) {
	needSandbox(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	root := t.TempDir()
	// Loopback stays allowed (go test servers); a public address does not.
	// On Linux the sandbox has its own network namespace, so the host's
	// loopback listener is unreachable by design and only macOS checks it.
	if runtime.GOOS == "darwin" {
		if err := sandboxed(t, root, "nc -z "+ncTimeout()+" 127.0.0.1 "+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)); err != nil {
			t.Fatalf("loopback connect refused: %v", err)
		}
	}
	if err := sandboxed(t, root, "nc -z "+ncTimeout()+" 1.1.1.1 443"); err == nil {
		t.Fatal("an outbound connection succeeded")
	}
	// A host Unix socket (e.g. a Docker or agent socket) is not reachable.
	sock := filepath.Join(os.TempDir(), "sop-iso-"+strconv.Itoa(os.Getpid())+".sock")
	ul, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ul.Close()
	if err := sandboxed(t, root, "nc -z -U "+sock); err == nil {
		t.Fatal("a Unix-socket connection succeeded")
	}
}

// The sandbox hides the fixed home-relative credential locations: a subpath
// deny for directories, a literal deny for files, and both the path and its
// real path. Bubblewrap replaces a directory with an empty tmpfs and a file
// with /dev/null after the root bind.
func TestSandboxHidesHomeCredentials(t *testing.T) {
	creds := []credentialPath{
		{path: "/home/u/.ssh", dir: true},
		{path: "/home/u/.netrc", dir: false},
	}
	profile := seatbeltProfile("/repo", []string{"/repo"}, creds)
	for _, want := range []string{
		`(deny file-read* (subpath "/home/u/.ssh"))`,
		`(deny file-read* (literal "/home/u/.netrc"))`,
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile missing %q:\n%s", want, profile)
		}
	}
	args := strings.Join(bubblewrapArgs("/repo", nil, creds), " ")
	for _, want := range []string{"--tmpfs /home/u/.ssh", "--ro-bind /dev/null /home/u/.netrc"} {
		if !strings.Contains(args, want) {
			t.Fatalf("bwrap args missing %q:\n%s", want, args)
		}
	}
	if strings.Index(args, "--ro-bind / /") > strings.Index(args, "--tmpfs /home/u/.ssh") {
		t.Fatalf("credential mounts must come after the root bind:\n%s", args)
	}
}

// The fixed list resolves against the user's home, skips entries that do not
// exist, and adds the real path of each existing entry.
func TestHomeCredentialsResolveExistingEntries(t *testing.T) {
	home := t.TempDir()
	writeRepoFile(t, home, ".ssh/probe", "x")
	writeRepoFile(t, home, ".netrc", "x")
	t.Setenv("HOME", home)

	got, err := homeCredentials()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, c := range got {
		have[c.path] = true
	}
	for _, rel := range []string{".ssh", ".netrc"} {
		p := filepath.Join(home, rel)
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{p, real} {
			if !have[want] {
				t.Errorf("missing %q in %v", want, got)
			}
		}
	}
	if have[filepath.Join(home, ".aws")] {
		t.Errorf("a missing credential directory was not skipped: %v", got)
	}
}

// A home directory that cannot be resolved fails closed instead of leaving the
// command unsandboxed.
func TestHomeCredentialsFailsClosedWithoutHome(t *testing.T) {
	// Cover the Unix and Windows home variables so the failure is not
	// platform-specific.
	for _, key := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH"} {
		t.Setenv(key, "")
	}
	if _, err := homeCredentials(); !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
}

// A credential-shaped value is dropped even when its variable name is ordinary:
// DATABASE_URL with URL userinfo is not passed to the sandbox, while the same
// URL without a credential, GOFLAGS, PATH, and HOME are.
func TestSandboxEnvDropsCredentialValues(t *testing.T) {
	has := func(env []string, name string) bool {
		for _, kv := range env {
			if strings.HasPrefix(kv, name+"=") {
				return true
			}
		}
		return false
	}
	t.Setenv("GOFLAGS", "-mod=mod")

	t.Setenv("DATABASE_URL", "postgres://user:pass@host/db")
	env, _ := sandboxEnv("cache")
	if has(env, "DATABASE_URL") {
		t.Fatalf("DATABASE_URL with userinfo reached the sandbox")
	}

	t.Setenv("DATABASE_URL", "postgres://host/db")
	env, _ = sandboxEnv("cache")
	if !has(env, "DATABASE_URL") {
		t.Fatalf("DATABASE_URL without a credential was dropped")
	}
	for _, name := range []string{"GOFLAGS", "PATH", "HOME"} {
		if !has(env, name) {
			t.Errorf("%s was dropped", name)
		}
	}
}

// The fixed list also covers the credential locations environment variables
// name, and a set relative value fails closed.
func TestHomeCredentialsIncludeEnvOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", fakeHomeUnder(t, root))
	dir := filepath.Join(root, "override-gpg")
	writeRepoFile(t, root, "override-gpg/secret", "x")
	one, two := filepath.Join(root, "kube-one"), filepath.Join(root, "kube-two")
	writeRepoFile(t, root, "kube-one", "x")
	writeRepoFile(t, root, "kube-two", "x")
	t.Setenv("GNUPGHOME", dir)
	t.Setenv("KUBECONFIG", one+string(os.PathListSeparator)+two)

	got, err := homeCredentials()
	if err != nil {
		t.Fatal(err)
	}
	// Overrides resolve symlinks first (on macOS /var is /private/var), so the
	// expected entries are the real paths.
	want := map[string]bool{}
	for p, isDir := range map[string]bool{dir: true, one: false, two: false} {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		want[real] = isDir
	}
	for _, c := range got {
		if dirKind, ok := want[c.path]; ok && dirKind != c.dir {
			t.Errorf("%s dir = %v, want %v", c.path, c.dir, dirKind)
		}
		delete(want, c.path)
	}
	for p := range want {
		t.Errorf("missing env credential path %s in %v", p, got)
	}
}

func TestHomeCredentialsFailsClosedOnRelativeOverride(t *testing.T) {
	t.Setenv("HOME", fakeHomeUnder(t, t.TempDir()))
	t.Setenv("GNUPGHOME", "relative/gnupg")
	if _, err := homeCredentials(); !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
}

// A sandboxed command cannot read a credential file, and an ordinary file under
// the same home still reads. The home is a temp dir, never the real one.
// fakeHomeUnder places the fake HOME inside the repository root. On Linux the
// sandbox mounts a private tmpfs over /tmp and binds only the root back, so a
// home elsewhere under /tmp would be hidden before credential masking applies,
// and the positive control could not see it either.
func fakeHomeUnder(t *testing.T, root string) string {
	t.Helper()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestRunCommandIsolationDeniesCredentialRead(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", "sop-credential-canary")
	writeRepoFile(t, home, "notes.txt", "ordinary-home-content")
	t.Setenv("HOME", home)

	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(filepath.Join(home, ".ssh", "probe")))
	if err == nil {
		t.Fatalf("reading a credential file succeeded: %q", out)
	}
	if strings.Contains(out, "sop-credential-canary") {
		t.Fatalf("credential canary leaked: %q", out)
	}

	// Positive control: an ordinary home file is still readable.
	out, err = sandboxedOutput(t, root, "cat "+strconv.Quote(filepath.Join(home, "notes.txt")))
	if err != nil || !strings.Contains(out, "ordinary-home-content") {
		t.Fatalf("ordinary home read failed: out=%q err=%v", out, err)
	}
}

// A sandboxed command must not turn a credential into a readable alias. This
// measures whether Seatbelt checks the link source: either ln is refused, or the
// created alias cannot be read. The result is logged so the evidence records
// which behavior this host exhibits.
func TestRunCommandIsolationDeniesCredentialHardlink(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", "sop-credential-canary")
	t.Setenv("HOME", home)
	alias := filepath.Join(root, "alias.txt")

	if out, err := sandboxedOutput(t, root, "ln "+strconv.Quote(filepath.Join(home, ".ssh", "probe"))+" "+strconv.Quote(alias)); err != nil {
		t.Logf("os.Link refused inside the sandbox (%v); output: %q", err, out)
		return
	}
	t.Logf("os.Link succeeded inside the sandbox; the alias must still be unreadable")
	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(alias))
	if err == nil || strings.Contains(out, "sop-credential-canary") {
		t.Fatalf("hardlink alias readable: out=%q err=%v", out, err)
	}
}

// A credential reached through an environment-variable override is denied just
// like one under HOME, while an ordinary neighbour stays readable.
func TestRunCommandIsolationDeniesEnvOverrideCredentials(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	t.Setenv("HOME", fakeHomeUnder(t, root))
	gpg := filepath.Join(root, "override-gpg")
	writeRepoFile(t, root, "override-gpg/secret", "override-canary")
	one, two := filepath.Join(root, "kube-one"), filepath.Join(root, "kube-two")
	writeRepoFile(t, root, "kube-one", "override-canary")
	writeRepoFile(t, root, "kube-two", "override-canary")
	writeRepoFile(t, root, "override-ordinary.txt", "ordinary-override-content")
	t.Setenv("GNUPGHOME", gpg)
	t.Setenv("KUBECONFIG", one+string(os.PathListSeparator)+two)

	for _, p := range []string{filepath.Join(gpg, "secret"), one, two} {
		out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(p))
		if err == nil || strings.Contains(out, "override-canary") {
			t.Fatalf("override credential %s read: out=%q err=%v", p, out, err)
		}
	}
	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(filepath.Join(root, "override-ordinary.txt")))
	if err != nil || !strings.Contains(out, "ordinary-override-content") {
		t.Fatalf("ordinary neighbour read failed: out=%q err=%v", out, err)
	}
}

// The macOS Keychain is reached through a system service, not a file read; a
// temporary keychain proves the mach-lookup deny does the work (the keychain
// file lives under the sandbox root and is not otherwise denied).
func TestRunCommandIsolationDeniesKeychainAccess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the keychain probe uses the macOS security CLI")
	}
	needSandbox(t)
	root := t.TempDir()
	keychain := filepath.Join(root, "probe.keychain")
	const password = "sop-probe-password"
	const canary = "keychain-canary"

	security := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "security", args...).CombinedOutput(); err != nil {
			t.Fatalf("security %v: %v: %s", args, err, out)
		}
	}
	security("create-keychain", "-p", password, keychain)
	defer func() { _ = exec.Command("security", "delete-keychain", keychain).Run() }()
	security("unlock-keychain", "-p", password, keychain)
	security("add-generic-password", "-a", "sop", "-s", "sop-probe", "-w", canary, "-T", "/usr/bin/security", keychain)

	out, err := sandboxedOutput(t, root, "security find-generic-password -s sop-probe -w "+strconv.Quote(keychain))
	if err == nil {
		t.Fatalf("a sandboxed keychain read succeeded: %q", out)
	}
	if strings.Contains(out, canary) {
		t.Fatalf("keychain canary leaked: %q", out)
	}
	// Positive control: the sandbox still runs after the service deny.
	if out, err := sandboxedOutput(t, root, "go version"); err != nil || !strings.Contains(out, "go version") {
		t.Fatalf("sandboxed go version: out=%q err=%v", out, err)
	}
}

func TestSeatbeltProfileDeniesKeychainService(t *testing.T) {
	profile := seatbeltProfile("/repo", []string{"/repo"}, nil)
	if !strings.Contains(profile, `(deny mach-lookup (global-name "com.apple.SecurityServer"))`) {
		t.Fatalf("profile does not deny the keychain service:\n%s", profile)
	}
}

// The bubblewrap view: host read-only first, private /tmp and /run, the state
// dir mounted read-only after the writable root bind (so it wins), and a
// private network namespace. Deterministic, so it runs on every platform.
func TestBubblewrapArgsOrder(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	canon, _ := filepath.EvalSymlinks(root)
	got := strings.Join(bubblewrapArgs(canon, []string{canon, "/tmp/x", "/run/user/1/cache"}, nil), " ")
	state, _ := filepath.EvalSymlinks(filepath.Join(canon, ".agent-sdlc"))
	order := []string{"--ro-bind / /", "--tmpfs /tmp", "--tmpfs /run", "--bind-try " + canon + " " + canon, "--ro-bind " + state + " " + state, "--unshare-net", "--chdir " + canon, "--"}
	at := 0
	for _, want := range order {
		i := strings.Index(got[at:], want)
		if i < 0 {
			t.Fatalf("missing or out of order %q in:\n%s", want, got)
		}
		at += i + len(want)
	}
	if strings.Contains(got, "/tmp/x") || strings.Contains(got, "/run/user") {
		t.Fatalf("a /tmp or /run path must not be bound over the private tmpfs:\n%s", got)
	}
	// A symlinked state dir is refused: the writable root would let the
	// command replace the link.
	linked := t.TempDir()
	writeRepoFile(t, linked, "real/state.db", "state")
	if err := os.Symlink("real", filepath.Join(linked, ".agent-sdlc")); err != nil {
		t.Fatal(err)
	}
	if err := refuseSymlinkedState(linked); !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("symlinked state dir: err = %v, want errIsolationUnavailable", err)
	}
	if err := refuseSymlinkedState(root); err != nil {
		t.Fatalf("real state dir refused: %v", err)
	}
	missing := strings.Join(bubblewrapArgs(t.TempDir(), nil, nil), " ")
	if !strings.Contains(missing, "--remount-ro") {
		t.Fatalf("a missing state dir must get a read-only tmpfs:\n%s", missing)
	}
}

// Each sandboxed run gets its own GOCACHE, removed afterwards, and the shared
// module cache is read-only, so one run cannot poison a later run's caches.
func TestRunCommandIsolationThrowawayGoCache(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	run := func() string {
		if err := sandboxed(t, root, `printf %s "$GOCACHE" > gocache.txt`); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(root, "gocache.txt"))
		if err != nil || len(b) == 0 {
			t.Fatalf("GOCACHE not set in the sandbox: %q, %v", b, err)
		}
		return string(b)
	}
	first, second := run(), run()
	if first == second {
		t.Fatalf("two runs shared GOCACHE %s", first)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("throwaway GOCACHE %s survived its run: %v", first, err)
	}
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	modcache := strings.TrimSpace(string(out))
	if err != nil || modcache == "" {
		t.Skip("no module cache to check")
	}
	if err := os.MkdirAll(modcache, 0o755); err != nil {
		t.Skip("module cache not creatable here")
	}
	probe := filepath.Join(modcache, "sop-isolation-probe")
	if err := sandboxed(t, root, "touch "+probe); err == nil {
		os.Remove(probe)
		t.Fatal("the shared module cache was writable from the sandbox")
	}
}

// The sandboxed child gets the inherited environment without credential-named
// variables, plus this run's GOCACHE override. Ordinary variables and the Go
// toolchain stay, so go test still runs.
func TestIsolateScrubsSecretEnv(t *testing.T) {
	needSandbox(t)
	t.Setenv("SOP_TEST_API_KEY", "sop-env-canary")
	t.Setenv("DATABASE_URL", "postgres://user:pass@host/db")
	t.Setenv("GOFLAGS", "-mod=mod")
	root := t.TempDir()

	argv, env, detail, cleanup, err := isolate(root, []string{"sh", "-c", "env"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandboxed env: %v: %s", err, out)
	}
	text := string(out)
	if strings.Contains(text, "SOP_TEST_API_KEY") || strings.Contains(text, "sop-env-canary") {
		t.Fatalf("a credential-named variable reached the sandbox:\n%s", text)
	}
	if strings.Contains(text, "DATABASE_URL") || strings.Contains(text, "postgres://user:pass") {
		t.Fatalf("a credential-shaped value reached the sandbox:\n%s", text)
	}
	hasEnv := func(name string) bool {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, name+"=") {
				return true
			}
		}
		return false
	}
	for _, name := range []string{"GOFLAGS", "HOME", "PATH", "GOCACHE"} {
		if !hasEnv(name) {
			t.Errorf("sandboxed env missing %s:\n%s", name, text)
		}
	}
	if !strings.Contains(text, "GOFLAGS=-mod=mod\n") {
		t.Errorf("GOFLAGS value changed:\n%s", text)
	}
	// The GOCACHE line must be this run's override, not an inherited value.
	var cacheLine string
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOCACHE=") {
			cacheLine = kv + "\n"
		}
	}
	if cacheLine == "" || !strings.Contains(text, cacheLine) {
		t.Errorf("sandboxed env does not carry the run GOCACHE %q:\n%s", cacheLine, text)
	}
	if !strings.Contains(detail, "scrubbed_env=") {
		t.Errorf("audit detail does not record the dropped count: %q", detail)
	}
	if out, err := sandboxedOutput(t, root, "go version"); err != nil || !strings.Contains(out, "go version") {
		t.Fatalf("sandboxed go version: out=%q err=%v", out, err)
	}
}

// The policy-checked argv follows the sandbox prefix unchanged: nothing (such
// as an env wrapper) sits between them that could reinterpret argv[0].
func TestIsolateKeepsArgvVerbatim(t *testing.T) {
	needSandbox(t)
	argv := []string{"GOFLAGS=-x", "go", "version"}
	wrapped, env, _, cleanup, err := isolate(t.TempDir(), argv)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got := wrapped[len(wrapped)-len(argv):]; strings.Join(got, " ") != strings.Join(argv, " ") || wrapped[len(wrapped)-len(argv)-1] == "env" {
		t.Fatalf("argv not passed verbatim after the sandbox prefix: %q", wrapped)
	}
	if !strings.Contains(strings.Join(env, "\n"), "GOCACHE=") {
		t.Fatal("GOCACHE must be set through the environment")
	}
}

// Every run_command child gets a scrubbed environment, so the operator disabling
// the sandbox does not also hand SOP's own credentials to a model-authored
// command. An ordinary neighbour stays visible, which proves the filter is
// specific rather than "drop everything".
func TestRunCommandScrubsCredentialEnvironment(t *testing.T) {
	fixture := `package fixture

import (
	"fmt"
	"os"
	"testing"
)

func TestPrintEnv(t *testing.T) {
	fmt.Println("ordinary:" + os.Getenv("SOP_TEST_ORDINARY_VAR"))
	fmt.Println("named:" + os.Getenv("SOP_TEST_API_KEY"))
	fmt.Println("shaped:" + os.Getenv("SOP_TEST_NEUTRAL_NAME"))
}
`
	for _, isolate := range []bool{true, false} {
		name := "isolation_off"
		if isolate {
			name = "isolation_on"
		}
		t.Run(name, func(t *testing.T) {
			if isolate {
				needSandbox(t)
				t.Setenv(IsolationEnv, "")
			} else {
				t.Setenv(IsolationEnv, "off")
			}
			root := t.TempDir()
			writeRepoFile(t, root, "go.mod", "module fixture\n\ngo 1.21\n")
			writeRepoFile(t, root, "env_test.go", fixture)
			t.Setenv("SOP_TEST_ORDINARY_VAR", "ordinary-value")
			t.Setenv("SOP_TEST_API_KEY", "canary-named-secret")
			t.Setenv("SOP_TEST_NEUTRAL_NAME", "AKIAIOSFODNN7EXAMPLE") //gitleaks:allow: synthetic credential-shape fixture, not a credential

			log := NewAuditLog(0)
			h := New(root, DefaultConfig(), log)
			out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{
				"command": "go test -run TestPrintEnv -v ./...",
			})
			if err != nil {
				t.Fatalf("run failed: %v (%s)", err, out)
			}
			if !strings.Contains(out, "ordinary:ordinary-value") {
				t.Errorf("ordinary variable was not inherited, so the scrub is too broad: %s", out)
			}
			if strings.Contains(out, "canary-named-secret") {
				t.Errorf("credential-named variable reached the child: %s", out)
			}
			if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
				t.Errorf("credential-shaped value reached the child: %s", out)
			}
			detail := log.Records()[0].Detail
			if isolate && !strings.Contains(detail, "isolate=") {
				t.Errorf("isolation on: audit detail = %q, want a sandboxed run", detail)
			}
			if !isolate && detail != "" {
				t.Errorf("isolation off: audit detail = %q, want an unsandboxed run", detail)
			}
		})
	}
}

// A nil environment means "inherit everything", so the scrub must never return
// nil even when it drops nothing.
func TestScrubbedEnvironmentIsNeverNil(t *testing.T) {
	env, _ := scrubbedEnvironment()
	if env == nil {
		t.Fatal("scrubbedEnvironment returned nil, which would inherit the full environment")
	}
}
