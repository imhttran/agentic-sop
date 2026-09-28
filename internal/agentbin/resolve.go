// Package agentbin resolves the location of the installed known-good
// sop-ollama-agent binary. It is the single Go-side definition of the resolution
// order the bootstrap script also follows, so diagnostics and the invocation path
// agree on which binary ran.
package agentbin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Environment variables governing the installed-binary location.
const (
	// EnvOverride names an explicit sop-ollama-agent binary to invoke. It wins
	// over every other source, so an operator can pin a specific known-good build
	// for one invocation.
	EnvOverride = "SOP_OLLAMA_AGENT_BIN"
	// EnvHome overrides the directory the known-good binary is installed into.
	EnvHome = "SOP_OLLAMA_AGENT_HOME"
)

// BinaryName is the installed binary's file name.
const BinaryName = "sop-ollama-agent"

// ErrNotInstalled reports that no installed known-good binary could be resolved.
var ErrNotInstalled = errors.New("no installed known-good sop-ollama-agent binary found")

// Resolve returns the path of the known-good sop-ollama-agent binary, in a
// deterministic order:
//
//  1. $SOP_OLLAMA_AGENT_BIN, if set (an explicit override),
//  2. $SOP_OLLAMA_AGENT_HOME/sop-ollama-agent, if set,
//  3. the default install directory under the user's home.
//
// A candidate is only accepted when it is a regular, executable file, exactly as
// the bootstrap script requires, so diagnostics never name a binary the bootstrap
// would refuse to run. Unlike the old bootstrap, it never compiles candidate
// working-tree source: a missing binary is a hard error (ErrNotInstalled), not an
// implicit build.
func Resolve(lookup func(string) string) (string, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	for _, candidate := range Candidates(lookup) {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if !isExecutableFile(abs) {
			continue
		}
		return abs, nil
	}
	return "", fmt.Errorf("%w; run scripts/install-sop-ollama-agent.sh to install one", ErrNotInstalled)
}

// isExecutableFile reports whether path names a regular file the caller can
// execute, matching the bootstrap script's `-f`/`-x` checks.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// Candidates lists the locations to try, in resolution order.
func Candidates(lookup func(string) string) []string {
	if lookup == nil {
		lookup = os.Getenv
	}
	var out []string
	if v := lookup(EnvOverride); v != "" {
		out = append(out, v)
	}
	if home := lookup(EnvHome); home != "" {
		out = append(out, filepath.Join(home, BinaryName))
		return out
	}
	out = append(out, filepath.Join(DefaultHome(), BinaryName))
	return out
}

// DefaultHome is the default directory the known-good binary is installed into,
// outside any project working tree.
func DefaultHome() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "sop", "bin")
	}
	return filepath.Join(os.TempDir(), "sop", "bin")
}

// BinaryPath is the resolved path of the running sop-ollama-agent executable, or
// an error if it cannot be determined.
func BinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(exe); err == nil {
		return abs, nil
	}
	return exe, nil
}
