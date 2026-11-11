package toolharness

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortTempDir makes a directory under /tmp where available, because macOS rejects
// Unix socket paths above roughly 104 bytes and the per-user temp directory can
// already be long.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := "/tmp"
	if _, err := os.Stat(base); err != nil {
		base = ""
	}
	dir, err := os.MkdirTemp(base, "sop-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestHomeSocketsAreMasked proves a Unix socket under $HOME is found up to the depth
// bound and masked in the bubblewrap arguments, and that a deeper one is not scanned.
func TestHomeSocketsAreMasked(t *testing.T) {
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}

	near := filepath.Join(home, "agent.sock")
	ln, err := net.Listen("unix", near)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// home/a/b/c/deep.sock is at the depth bound and must be found.
	deepDir := filepath.Join(home, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0o700); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(deepDir, "deep.sock")
	deepLn, err := net.Listen("unix", deep)
	if err != nil {
		t.Fatal(err)
	}
	defer deepLn.Close()

	// home/a/b/c/d/beyond.sock is past the bound and is a documented limit.
	beyondDir := filepath.Join(deepDir, "d")
	if err := os.MkdirAll(beyondDir, 0o700); err != nil {
		t.Fatal(err)
	}
	beyond := filepath.Join(beyondDir, "beyond.sock")
	beyondLn, err := net.Listen("unix", beyond)
	if err != nil {
		t.Fatal(err)
	}
	defer beyondLn.Close()

	socks, err := homeSocketPaths()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, s := range socks {
		paths = append(paths, s.path)
	}
	if !contains(paths, near) {
		t.Fatalf("socket %s not found; got %v", near, paths)
	}
	if !contains(paths, deep) {
		t.Fatalf("socket at the depth bound was not scanned: %v", paths)
	}
	if contains(paths, beyond) {
		t.Fatalf("socket past the depth bound was scanned: %v", paths)
	}

	args := strings.Join(bubblewrapArgs("/repo", nil, socks), " ")
	if !strings.Contains(args, "--ro-bind /dev/null "+near) {
		t.Fatalf("socket %s is not masked in the bubblewrap arguments", near)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestHomeSymlinkToDirectoryIsFollowed proves a symlinked directory under home, like a
// runner's ~/.ghcup pointing at a toolchain outside home, is scanned instead of
// refusing the sandbox, and the sockets it holds are masked at their real paths.
func TestHomeSymlinkToDirectoryIsFollowed(t *testing.T) {
	home := shortTempDir(t)
	outside := shortTempDir(t)
	outside, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(outside, "linked.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	t.Setenv("HOME", home)
	if err := os.Symlink(outside, filepath.Join(home, "linked")); err != nil {
		t.Fatal(err)
	}
	socks, err := homeSocketPaths()
	if err != nil {
		t.Fatalf("a symlink to a directory must not fail the sandbox: %v", err)
	}
	var paths []string
	for _, s := range socks {
		paths = append(paths, s.path)
	}
	if !contains(paths, sock) {
		t.Fatalf("socket %s behind a directory symlink was not scanned: %v", sock, paths)
	}
	if args := strings.Join(bubblewrapArgs("/repo", nil, socks), " "); !strings.Contains(args, "--ro-bind /dev/null "+sock) {
		t.Fatalf("socket %s behind a directory symlink is not masked", sock)
	}
}

// TestHomeSymlinkCycleTerminates proves a symlink cycle under home does not hang the
// scan or fail the sandbox.
func TestHomeSymlinkCycleTerminates(t *testing.T) {
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	if err := os.Symlink(home, filepath.Join(home, "loop")); err != nil {
		t.Fatal(err)
	}
	if _, err := homeSocketPaths(); err != nil {
		t.Fatalf("a symlink cycle must be skipped, not fail closed: %v", err)
	}
}
