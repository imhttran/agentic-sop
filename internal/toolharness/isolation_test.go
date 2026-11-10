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
)

// needSandbox skips unless this host can actually start the platform sandbox
// (sandbox-exec cannot nest; bwrap needs unprivileged user namespaces).
func needSandbox(t *testing.T) {
	t.Helper()
	probe := map[string][]string{
		"darwin": {"sandbox-exec", "-p", "(version 1)(allow default)", "true"},
		"linux":  {"bwrap", "--ro-bind", "/", "/", "--unshare-net", "true"},
	}[runtime.GOOS]
	if probe == nil {
		t.Skipf("no sandbox backend on %s", runtime.GOOS)
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
	t.Setenv(IsolationEnv, "")
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	if _, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."}); err != nil {
		t.Fatal(err)
	}
	if d := log.Records()[0].Detail; d != "" {
		t.Fatalf("isolation off: audit detail = %q, want empty", d)
	}
}

func TestRunCommandIsolationOnFailsClosedWithoutBackend(t *testing.T) {
	t.Setenv(IsolationEnv, "1")
	t.Setenv("PATH", t.TempDir()) // no sandbox-exec on PATH
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
	if r := log.Records()[0]; r.Action != ActionDeny {
		t.Fatalf("audit action = %s, want deny", r.Action)
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
