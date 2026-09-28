package agentbin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envMap returns a lookup over a fixed map, so tests never touch the real
// process environment.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolvePrefersOverride(t *testing.T) {
	dir := t.TempDir()
	override := filepath.Join(dir, "custom-agent")
	if err := os.WriteFile(override, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(envMap(map[string]string{EnvOverride: override}))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != override {
		t.Errorf("Resolve = %q, want the override %q", got, override)
	}
}

func TestResolveUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, BinaryName)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(envMap(map[string]string{EnvHome: home}))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != bin {
		t.Errorf("Resolve = %q, want %q", got, bin)
	}
}

func TestResolveMissingIsAnActionableError(t *testing.T) {
	home := t.TempDir() // empty: no binary installed
	_, err := Resolve(envMap(map[string]string{EnvHome: home}))
	if err == nil {
		t.Fatal("Resolve returned nil, want an error when the binary is missing")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, want ErrNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "install-sop-ollama-agent.sh") {
		t.Errorf("err = %v, want an actionable install hint", err)
	}
}

func TestResolveSkipsDirectories(t *testing.T) {
	home := t.TempDir()
	// A directory named like the binary must not be resolved as the binary.
	if err := os.Mkdir(filepath.Join(home, BinaryName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(envMap(map[string]string{EnvHome: home})); err == nil {
		t.Fatal("Resolve returned a directory, want an error")
	}
}

// TestResolveSkipsNonExecutable proves resolution agrees with the bootstrap
// script: a file the script would refuse to run (not executable) is not
// reported as the resolved binary.
func TestResolveSkipsNonExecutable(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, BinaryName)
	if err := os.WriteFile(bin, []byte("not executable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve(envMap(map[string]string{EnvHome: home}))
	if err == nil {
		t.Fatalf("Resolve returned %q, want an error for a non-executable binary", bin)
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, want ErrNotInstalled", err)
	}
}

// TestResolveFallsBackPastNonExecutableOverride proves a non-executable override
// does not shadow a valid executable in the configured home directory.
func TestResolveFallsBackPastNonExecutableOverride(t *testing.T) {
	dir := t.TempDir()
	override := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(override, []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, BinaryName)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(envMap(map[string]string{EnvOverride: override, EnvHome: home}))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got != bin {
		t.Errorf("Resolve = %q, want the executable %q", got, bin)
	}
}

func TestCandidatesOrder(t *testing.T) {
	got := Candidates(envMap(map[string]string{EnvOverride: "/o/bin", EnvHome: "/h/bin"}))
	want := []string{"/o/bin", "/h/bin/" + BinaryName}
	if len(got) != len(want) {
		t.Fatalf("Candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Candidates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultHomeIsOutsideProjectTree(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(DefaultHome(), wd) {
		t.Errorf("DefaultHome = %q, must not be inside the working tree %q", DefaultHome(), wd)
	}
}

func TestBinaryPathIsAbsolute(t *testing.T) {
	got, err := BinaryPath()
	if err != nil {
		t.Fatalf("BinaryPath failed: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("BinaryPath = %q, want an absolute path", got)
	}
}
