//go:build windows

package question

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestFileStoreWritesFailClosedOnWindows pins the chosen Windows behavior: because
// the standard library provides no advisory file lock, durable writes (including
// answer submission) fail closed with ErrLockUnsupported rather than risk a lost
// update, while reads remain available.
func TestFileStoreWritesFailClosedOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := NewService(fs, nil).CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q"}); !errors.Is(err, ErrLockUnsupported) {
		t.Fatalf("create err = %v, want ErrLockUnsupported", err)
	}
	if _, err := fs.ListQuestions(StatePending); err != nil {
		t.Fatalf("reads must still work on Windows: %v", err)
	}
}
