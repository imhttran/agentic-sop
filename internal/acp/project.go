package acp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// canonicalProjectDir validates one session/new cwd and returns its canonical
// form: absolute and symlink-resolved. It never changes the process-global
// working directory (no os.Chdir anywhere in this package); resolution is per
// session and per call, so concurrent sessions on different projects cannot
// observe each other's directories.
func canonicalProjectDir(cwd string) (string, error) {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return "", fmt.Errorf("cwd is required: pass the working directory the session operates in")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve cwd %q: %w", trimmed, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project directory %q does not exist: create it, or pass an existing directory, before opening a session", cwd)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory: pass the project directory, not a file", cwd)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project directory %q: %w", cwd, err)
	}
	return resolved, nil
}

// projectIDFor derives a stable project identity from the canonical project
// directory. Two differently spelled paths that resolve to the same directory
// receive the same identity; sessions on different directories never do. The
// identity is a digest, not the raw path, so it is safe to echo in protocol
// results without leaking absolute local paths.
func projectIDFor(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return "proj_" + hex.EncodeToString(sum[:8])
}
