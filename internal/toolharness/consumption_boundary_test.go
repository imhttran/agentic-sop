package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBoundedCaptureKeepsOnlyTheBound proves the read-boundary capture retains
// at most the configured number of bytes while still counting the whole stream,
// so a command cannot allocate its full output before truncation.
func TestBoundedCaptureKeepsOnlyTheBound(t *testing.T) {
	c := newBoundedCapture(8)
	if _, err := c.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if c.buf.Len() != 3 {
		t.Fatalf("retained = %d, want 3", c.buf.Len())
	}
	if _, err := c.Write([]byte("defghij")); err != nil {
		t.Fatal(err)
	}
	if c.buf.Len() != 8 {
		t.Errorf("retained = %d, want the 8-byte bound", c.buf.Len())
	}
	if c.total != 10 {
		t.Errorf("total = %d, want 10", c.total)
	}
	if got, want := c.String(), "abcdefgh\n… [truncated 2 bytes]"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBoundedCaptureBoundaryCases covers just below, at, and above the bound,
// and proves a non-positive bound keeps the built-in default instead of
// disabling the limit.
func TestBoundedCaptureBoundaryCases(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		n     int
		trunc bool
	}{
		{"below", 10, 9, false},
		{"at", 10, 10, false},
		{"above", 10, 11, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newBoundedCapture(tc.limit)
			c.Write([]byte(strings.Repeat("x", tc.n)))
			if got := strings.Contains(c.String(), "truncated"); got != tc.trunc {
				t.Errorf("truncated = %v, want %v", got, tc.trunc)
			}
		})
	}

	defaulted := newBoundedCapture(0)
	defaulted.Write([]byte(strings.Repeat("x", 1000)))
	if strings.Contains(defaulted.String(), "truncated") {
		t.Error("a non-positive bound truncated below the built-in default")
	}
	if defaulted.limit != DefaultConfig().MaxOutputBytes {
		t.Errorf("default limit = %d, want %d", defaulted.limit, DefaultConfig().MaxOutputBytes)
	}
}

// TestExecCapturedBoundsStreamedOutput proves the real command seam keeps only
// the configured output bound even when the child writes far more.
func TestExecCapturedBoundsStreamedOutput(t *testing.T) {
	root := t.TempDir()
	const limit = 1024
	h := New(root, Config{CommandTimeout: 10 * time.Second, MaxOutputBytes: limit}, nil)

	out, err := h.exec(context.Background(), []string{"sh", "-c", `printf "%4096s" ""`})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(out) > limit+128 {
		t.Errorf("result length = %d, want at most %d", len(out), limit+128)
	}
	if !strings.Contains(out, "truncated 3072 bytes") {
		t.Errorf("result missing the exact truncation marker:\n%q", out)
	}
}

// TestReadFileBoundsBeforeAllocation proves read_file never reads a large file
// into memory before applying the configured output bound.
func TestReadFileBoundsBeforeAllocation(t *testing.T) {
	root := t.TempDir()
	const limit = 64
	body := strings.Repeat("a", limit*4)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := New(root, Config{CommandTimeout: time.Second, MaxOutputBytes: limit}, nil)
	out, err := h.readFile(map[string]any{"path": "big.txt"})
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !strings.HasPrefix(out, strings.Repeat("a", limit)) {
		t.Errorf("result does not start with the bounded prefix")
	}
	if len(out) > limit+64 {
		t.Errorf("result length = %d, want at most %d", len(out), limit+64)
	}
	if !strings.Contains(out, "truncated 192 bytes") {
		t.Errorf("result missing the exact truncation marker:\n%q", out)
	}
}
