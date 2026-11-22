package egmmanifest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func TestCheckOutside(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "sealed")
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := egmmanifest.CheckOutside(inside, root); err == nil {
		t.Fatal("path inside the forbidden root was accepted")
	}
	if err := egmmanifest.CheckOutside(outside, root); err != nil {
		t.Fatalf("outside path rejected: %v", err)
	}
}

func TestCheckOutsideSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	root := t.TempDir()
	outsideDir := t.TempDir()
	link := filepath.Join(outsideDir, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlink unsupported:", err)
	}
	if err := egmmanifest.CheckOutside(link, root); err == nil {
		t.Fatal("symlink to the forbidden root was accepted")
	}
}

func TestCheckFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not meaningful on Windows")
	}
	p := filepath.Join(t.TempDir(), "f")
	writeFile(t, p, "x")
	if err := egmmanifest.CheckFileMode(p, 0); err != nil {
		t.Fatalf("0600 file rejected: %v", err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := egmmanifest.CheckFileMode(p, 0); err == nil {
		t.Fatal("0644 file accepted")
	}
	if err := os.Chmod(p, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := egmmanifest.CheckFileMode(p, 0o400); err != nil {
		t.Fatalf("0400 file rejected: %v", err)
	}
	if err := egmmanifest.CheckFileMode(p, 0o600); err == nil {
		t.Fatal("mode mismatch accepted")
	}
}

func TestCheckDirMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not meaningful on Windows")
	}
	d := filepath.Join(t.TempDir(), "sealed")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := egmmanifest.CheckDirMode(d, 0o700); err != nil {
		t.Fatalf("0700 directory rejected: %v", err)
	}
	if err := os.Chmod(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := egmmanifest.CheckDirMode(d, 0); err == nil {
		t.Fatal("0755 directory accepted")
	}
}

func TestCheckFileModeRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	p := filepath.Join(t.TempDir(), "f")
	writeFile(t, p, "x")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(p, link); err != nil {
		t.Skip("symlink unsupported:", err)
	}
	if err := egmmanifest.CheckFileMode(link, 0); err == nil {
		t.Fatal("symlink accepted")
	}
}
