package toolharness

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHomeSocketsAreMasked proves a Unix socket under $HOME is found and masked in the
// bubblewrap arguments, and that one beyond the depth bound is not scanned.
func TestHomeSocketsAreMasked(t *testing.T) {
	home, err := os.MkdirTemp("", "sop-home-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	t.Setenv("HOME", home)

	near := filepath.Join(home, "agent.sock")
	ln, err := net.Listen("unix", near)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	deepDir := filepath.Join(home, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(deepDir, 0o700); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(deepDir, "deep.sock")
	deepLn, err := net.Listen("unix", deep)
	if err != nil {
		t.Fatal(err)
	}
	defer deepLn.Close()

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
	if contains(paths, deep) {
		t.Fatalf("socket beyond the depth bound was scanned: %v", paths)
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
