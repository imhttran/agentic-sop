package dqharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file adds focused regression coverage for the Linux bubblewrap and macOS
// Seatbelt replay isolation backends. Backend selection and argument construction are
// pure and run on every host. Runtime confinement tests require an isolation tool.
// The macOS Seatbelt path is validated by command construction only; it is not
// runtime-tested on a Linux host.

func TestIsolationBackendSelection(t *testing.T) {
	tests := []struct {
		goos    string
		tool    string
		backend IsolationBackend
	}{
		{"linux", "bwrap", BackendBubblewrap},
		{"darwin", "sandbox-exec", BackendSeatbelt},
		{"windows", "", BackendNone},
		{"plan9", "", BackendNone},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			iso := detectIsolationFor(tc.goos)
			if iso.Backend != tc.backend {
				t.Fatalf("backend = %q, want %q", iso.Backend, tc.backend)
			}
			if iso.Tool != tc.tool {
				t.Fatalf("tool = %q, want %q", iso.Tool, tc.tool)
			}
			if tc.backend == BackendNone && iso.Available {
				t.Fatal("an unsupported GOOS must not report available isolation")
			}
		})
	}
}

func TestBubblewrapArgumentsExcludeSeatbelt(t *testing.T) {
	dir := t.TempDir()
	_, args, err := bubblewrapCommand(dir, []string{"true"})
	if err != nil {
		t.Fatalf("bubblewrapCommand: %v", err)
	}
	for _, a := range args {
		if a == "-p" {
			t.Fatalf("bwrap args contain the Seatbelt -p flag: %v", args)
		}
		if strings.Contains(a, "(deny file-write*)") || strings.Contains(a, "(version 1)") {
			t.Fatalf("bwrap args contain a Seatbelt profile: %q", a)
		}
	}
}

func TestBubblewrapArgumentsShape(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "replay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bound, args, err := bubblewrapCommand(dir, []string{"true"})
	if err != nil {
		t.Fatalf("bubblewrapCommand: %v", err)
	}
	if bound == "" {
		t.Fatal("empty bound path")
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--ro-bind / /") {
		t.Fatalf("bwrap args must not bind the host root read-only: %v", args)
	}
	for _, forbidden := range []string{"/run", "/var/run", "/home", "/root", "/opt", "/srv", "/mnt", "/media"} {
		if strings.Contains(joined, "--bind-try "+forbidden) || strings.Contains(joined, "--bind "+forbidden+" ") {
			t.Fatalf("bwrap args bind forbidden host path %q: %v", forbidden, args)
		}
	}
	want := []string{
		"--ro-bind-try /usr /usr",
		"--ro-bind-try /bin /bin",
		"--ro-bind-try /sbin /sbin",
		"--ro-bind-try /lib /lib",
		"--ro-bind-try /lib64 /lib64",
		"--ro-bind-try /etc /etc",
		"--ro-bind-try /sys /sys",
		"--proc /proc",
		"--dev /dev",
		"--tmpfs /dev/shm",
		"--remount-ro /dev/shm",
		"--bind " + bound + " /replay",
		"--chdir /replay",
		"--unshare-net",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--setenv PATH /usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"--setenv HOME /replay",
		"--setenv TMPDIR /replay/.tmp",
		"--setenv GOTOOLCHAIN local",
	}
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Fatalf("bwrap args missing %q: %v", w, args)
		}
	}
	sep := -1
	target := -1
	for i, a := range args {
		if a == "--" && sep == -1 {
			sep = i
		}
		if a == "true" && sep != -1 && target == -1 {
			target = i
		}
	}
	if sep == -1 || target == -1 {
		t.Fatalf("missing -- separator or target: %v", args)
	}
	if target != sep+1 {
		t.Fatalf("target does not immediately follow --: sep=%d target=%d args=%v", sep, target, args)
	}
}

func TestSeatbeltArgumentsShape(t *testing.T) {
	dir := t.TempDir()
	args := seatbeltCommand(dir, []string{"true"})
	if len(args) < 3 {
		t.Fatalf("too few args: %v", args)
	}
	if args[0] != "-p" {
		t.Fatalf("first arg = %q, want -p", args[0])
	}
	profile := args[1]
	for _, want := range []string{"(deny network*)", "(deny file-write*)", "(subpath " + `"` + dir + `"` + ")"} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile missing %q:\n%s", want, profile)
		}
	}
	if args[2] != "true" {
		t.Fatalf("target not appended after the profile: %v", args)
	}
}

func TestIsolationCommandUnknownBackendFailsClosed(t *testing.T) {
	args, env, err := isolationCommand(IsolationBackend("unknown-backend"), t.TempDir(), []string{"true"})
	if err == nil {
		t.Fatal("unknown backend must fail closed")
	}
	if args != nil || env != nil {
		t.Fatalf("args=%v env=%v, want nil on failure", args, env)
	}
}

func TestBubblewrapCommandCanonicalizationFailsClosed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does", "not", "exist")
	bound, args, err := bubblewrapCommand(missing, []string{"true"})
	if err == nil {
		t.Fatal("bubblewrapCommand must fail when canonicalization fails")
	}
	if bound != "" || args != nil {
		t.Fatalf("bound=%q args=%v, want empty on failure", bound, args)
	}
	msg := err.Error()
	if !strings.Contains(msg, "canonicalize") || !strings.Contains(msg, missing) {
		t.Fatalf("error is not actionable: %q", msg)
	}
}

func TestIsolatedReplayCanonicalizationFailsBeforeLaunch(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing-parent")
	marker := filepath.Join(t.TempDir(), "ran.txt")
	f := isolationFixture("/bin/sh", "-c", "echo ran > "+marker)
	code, _, err := IsolatedReplay(context.Background(), parent, f, 5*time.Second)
	if err == nil || code != -1 {
		t.Fatalf("code=%d err=%v, want a pre-launch error", code, err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("the verification command ran despite a canonicalization failure")
	}
}

// TestIsolatedReplayReplayDirWritesSucceed proves writes inside the replay directory
// succeed under the real backend.
func TestIsolatedReplayReplayDirWritesSucceed(t *testing.T) {
	requireIsolation(t)
	f := isolationFixture("sh", "-c", "echo inside > local.txt && test \"$(cat local.txt)\" = inside")
	code, out, err := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if err != nil || code != 0 {
		t.Fatalf("replay-directory write failed: code=%d out=%q err=%v", code, out, err)
	}
}

// TestIsolatedReplayConfinementTempRoots proves that parent and sibling writes fail for
// replay parents under both /tmp and /dev/shm.
func TestIsolatedReplayConfinementTempRoots(t *testing.T) {
	requireIsolation(t)
	parents := []string{"/tmp"}
	if st, err := os.Stat("/dev/shm"); err == nil && st.IsDir() {
		parents = append(parents, "/dev/shm")
	}
	for _, root := range parents {
		root := root
		t.Run(strings.TrimPrefix(root, "/")+"-parent", func(t *testing.T) {
			parent, err := os.MkdirTemp(root, "confinement-")
			if err != nil {
				t.Fatalf("temp root %q unusable: %v", root, err)
			}
			defer os.RemoveAll(parent)
			escape := filepath.Join(parent, "escape.txt")
			f := isolationFixture("sh", "-c", "echo escaped > "+escape)
			code, out, err := IsolatedReplay(context.Background(), parent, f, 10*time.Second)
			if err == nil && code == 0 {
				t.Fatalf("parent write under %q succeeded: out=%q", root, out)
			}
			if _, statErr := os.Stat(escape); statErr == nil {
				t.Fatalf("a file was written outside the replay directory under %q", root)
			}
		})
		t.Run(strings.TrimPrefix(root, "/")+"-sibling", func(t *testing.T) {
			parent, err := os.MkdirTemp(root, "confinement-")
			if err != nil {
				t.Fatalf("temp root %q unusable: %v", root, err)
			}
			defer os.RemoveAll(parent)
			sibling := filepath.Join(parent, "sibling.txt")
			f := isolationFixture("sh", "-c", "echo sibling > "+sibling)
			code, out, err := IsolatedReplay(context.Background(), parent, f, 10*time.Second)
			if err == nil && code == 0 {
				t.Fatalf("sibling write under %q succeeded: out=%q", root, out)
			}
		})
	}
}

// TestIsolatedReplayDeniesPrivateMountWrite proves a write under /dev/shm fails even
// though it would land in a private sandbox mount.
func TestIsolatedReplayDeniesPrivateMountWrite(t *testing.T) {
	requireIsolation(t)
	probe := filepath.Join("/dev/shm", "private-mount-probe.txt")
	f := isolationFixture("sh", "-c", "echo probe > "+probe)
	code, out, err := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if err == nil && code == 0 {
		t.Fatalf("private-mount write under /dev/shm succeeded: out=%q", out)
	}
	if _, statErr := os.Stat(probe); statErr == nil {
		_ = os.Remove(probe)
		t.Fatal("a file landed on the host /dev/shm")
	}
}
