package planacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/outcome"
)

// EvidenceError classifies why a reference is not verified evidence.
type EvidenceError struct {
	Kind string
	Path string
	Err  error
}

func (e *EvidenceError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("evidence %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("evidence %s: %s: %v", e.Kind, e.Path, e.Err)
}

func (e *EvidenceError) Unwrap() error { return e.Err }

// VerifyEvidenceRefs reads every referenced artifact under root and recomputes
// its SHA-256. A supplied hash is never trusted without recomputation. Missing,
// changed, unreadable, escaping, or malformed references are rejected.
func VerifyEvidenceRefs(root string, refs []outcome.EvidenceRef) error {
	if len(refs) == 0 {
		return &EvidenceError{Kind: "missing", Err: errors.New("at least one verified evidence reference is required")}
	}
	for _, ref := range refs {
		if err := VerifyEvidenceRef(root, ref); err != nil {
			return err
		}
	}
	return nil
}

// VerifyEvidenceRef verifies one reference. It is used both when recording and
// when reporting, so evidence that later changes stops counting as accepted.
func VerifyEvidenceRef(root string, ref outcome.EvidenceRef) error {
	path := strings.TrimSpace(ref.Path)
	want := strings.ToLower(strings.TrimSpace(ref.SHA256))
	if path == "" {
		return &EvidenceError{Kind: "invalid", Err: errors.New("evidence path is empty")}
	}
	if len(want) != 64 {
		return &EvidenceError{Kind: "invalid", Path: path, Err: errors.New("evidence sha256 must be 64 hex characters")}
	}
	if _, err := hex.DecodeString(want); err != nil {
		return &EvidenceError{Kind: "invalid", Path: path, Err: fmt.Errorf("evidence sha256 is not hex: %w", err)}
	}
	abs, err := resolveUnderRoot(root, path)
	if err != nil {
		return &EvidenceError{Kind: "unavailable", Path: path, Err: err}
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return &EvidenceError{Kind: "missing", Path: path, Err: err}
	}
	if err != nil {
		return &EvidenceError{Kind: "unavailable", Path: path, Err: err}
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return &EvidenceError{Kind: "changed", Path: path, Err: fmt.Errorf("sha256 is %s, not the reviewed %s", got, want)}
	}
	if section := strings.TrimSpace(ref.Section); section != "" {
		text := string(data)
		for _, part := range strings.FieldsFunc(section, func(r rune) bool { return r == ';' }) {
			part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "§"))
			if part == "" {
				continue
			}
			if !strings.Contains(text, part) {
				return &EvidenceError{Kind: "section-missing", Path: path, Err: fmt.Errorf("section %q was not found", part)}
			}
		}
	}
	return nil
}

func resolveUnderRoot(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("evidence path must be repository-relative")
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("evidence path escapes the repository root")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, clean)
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(rootReal, real)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", errors.New("evidence path escapes the repository root through a symlink")
	}
	return real, nil
}
