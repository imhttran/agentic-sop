package toolharness

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// unixSocketFixture covers the three socket outcomes: binding inside a writable
// dir is ordinary build work, binding outside one is still a write-boundary
// violation, and reaching a socket the unsandboxed test process created must
// stay denied.
const unixSocketFixture = `package sock

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "s")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "own.sock"))
	if err != nil {
		t.Fatalf("bind own socket: %v", err)
	}
	_ = ln.Close()
	t.Log("bound own socket")
}

func TestBindOutsideWritable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "outside.txt"))
	if err != nil {
		t.Fatalf("read outside path: %v", err)
	}
	p := strings.TrimSpace(string(b))
	ln, err := net.Listen("unix", p)
	if err != nil {
		t.Logf("bind outside denied: %v", err)
		return
	}
	_ = ln.Close()
	t.Fatalf("bound a socket outside the writable dirs: %s", p)
}

func TestHostSocket(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "hostsocket.txt"))
	if err != nil {
		t.Fatalf("read host socket path: %v", err)
	}
	c, err := net.Dial("unix", strings.TrimSpace(string(b)))
	if err != nil {
		t.Logf("host socket denied: %v", err)
		return
	}
	_ = c.Close()
	t.Fatalf("connected to the host socket inside the sandbox")
}
`

// A sandboxed command may create its own Unix socket but must not bind outside a
// writable dir or reach a host socket. macOS denies AF_UNIX bind as a side effect
// of `(deny network*)`, which breaks ordinary test listeners; the profile
// re-allows bind only under a writable dir. Linux already allows it inside its
// private /tmp.
func TestRunCommandIsolationAllowsOwnUnixSocketOnly(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	// Short paths: macOS rejects Unix socket paths above roughly 104 bytes.
	hostDir, err := os.MkdirTemp("/tmp", "sop-hs-")
	if err != nil {
		t.Skipf("no /tmp for a short socket path: %v", err)
	}
	defer os.RemoveAll(hostDir)
	hostPath := filepath.Join(hostDir, "host.sock")
	ln, err := net.Listen("unix", hostPath)
	if err != nil {
		t.Fatalf("listen on the host socket: %v", err)
	}
	defer ln.Close()

	// /var/tmp is outside every writable dir on both backends, so a bind there
	// must still be denied.
	outside := filepath.Join("/var/tmp", "sop-neg-"+strings.ReplaceAll(t.Name(), "/", "_")+".sock")
	_ = os.Remove(outside)
	t.Cleanup(func() { _ = os.Remove(outside) })

	writeRepoFile(t, root, "hostsocket.txt", hostPath)
	writeRepoFile(t, root, "outside.txt", outside)
	writeRepoFile(t, root, "go.mod", "module sock\n\ngo 1.27\n")
	writeRepoFile(t, root, "sock/sock_test.go", unixSocketFixture)

	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	h := New(root, cfg, NewAuditLog(0))
	ctx := context.Background()

	run := func(test string) string {
		t.Helper()
		out, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": "go test ./sock -run " + test + " -v"})
		if err != nil {
			t.Fatalf("%s did not run: %v (%s)", test, err, out)
		}
		return out
	}

	if out := run("TestOwnSocket"); !strings.Contains(out, "bound own socket") {
		t.Fatalf("a sandboxed command could not bind its own Unix socket: %q", out)
	}

	out := run("TestBindOutsideWritable")
	if strings.Contains(out, "bound a socket outside") {
		t.Fatalf("a sandboxed command bound a socket outside the writable dirs: %q", out)
	}
	if !strings.Contains(out, "bind outside denied") {
		t.Fatalf("the outside bind was not denied for the expected reason: %q", out)
	}

	out = run("TestHostSocket")
	if strings.Contains(out, "connected to the host socket") {
		t.Fatalf("a sandboxed command reached a host Unix socket: %q", out)
	}
	if !strings.Contains(out, "host socket denied") {
		t.Fatalf("the host socket was not denied for the expected reason: %q", out)
	}
	// Linux hides the host /tmp behind a private tmpfs, so the denial is ENOENT
	// there; macOS keeps the path visible and denies the connect with EPERM.
	want := "operation not permitted"
	if runtime.GOOS == "linux" {
		want = "no such file or directory"
	}
	if !strings.Contains(out, want) {
		t.Fatalf("the host socket was denied for an unexpected reason, want %q: %q", want, out)
	}
}

// The profile must scope the bind allowance to the writable dirs and keep the
// network deny otherwise, so a future edit cannot turn it into a blanket allow.
func TestSeatbeltProfileScopesUnixSocketBind(t *testing.T) {
	profile := seatbeltProfile("/repo", []string{"/repo", "/private/tmp"}, nil)
	if strings.Contains(profile, "(allow network-bind)\n") {
		t.Fatalf("the profile contains a blanket network-bind allow:\n%s", profile)
	}
	for _, want := range []string{
		"(allow network-bind (subpath \"/repo\"))",
		"(allow network-bind (subpath \"/private/tmp\"))",
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile is missing %q:\n%s", want, profile)
		}
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Fatalf("profile lost the network deny:\n%s", profile)
	}
}
