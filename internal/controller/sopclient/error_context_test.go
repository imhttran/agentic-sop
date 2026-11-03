package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// A state database that cannot be reached is not an uninitialized project: the
// path and the cause are reported, so a permission or path error is actionable.
//
// A file where .agent-sdlc belongs makes os.Stat fail with ENOTDIR, an I/O error
// that is not fs.ErrNotExist.
func TestOpenStoreReportsUnreachableState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := OpenStore(root)
	if err == nil {
		t.Fatal("OpenStore = nil error, want the path failure")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want an I/O failure rather than a missing project", err)
	}
	if !strings.Contains(err.Error(), "state.db") {
		t.Fatalf("err = %q, want it to name the state database path", err)
	}
	if strings.Contains(err.Error(), "initialized SOP project") {
		t.Fatalf("err = %q, want a path error not presented as an uninitialized project", err)
	}
}

// A genuinely missing database keeps the guidance operators already know.
func TestOpenStoreMissingStateKeepsGuidance(t *testing.T) {
	_, err := OpenStore(t.TempDir())
	if err == nil {
		t.Fatal("OpenStore = nil error, want a missing state database")
	}
	if !strings.Contains(err.Error(), "is this an initialized SOP project?") {
		t.Fatalf("err = %q, want the uninitialized-project guidance kept", err)
	}
}

// A failed command names the project it ran for, and still exposes the cause.
func TestExecNamesTheProjectAndKeepsTheCause(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, refusingSop(t, "sop: provider credentials are not configured", 1), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id := config.ProjectID(root)

	err = c.Run(context.Background(), id)
	if err == nil {
		t.Fatal("err = nil, want SOP's refusal")
	}
	if !strings.Contains(err.Error(), `project "`+id+`"`) {
		t.Fatalf("err = %q, want the failing project named", err)
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v (%T), want *CommandError still reachable", err, err)
	}
	if !sopRefused(err) {
		t.Fatalf("sopRefused(%v) = false, want the refusal still identifiable", err)
	}
}

// Wrapping the project around a command error does not hide a timeout, on either
// the classification sentinel or the context cause.
func TestExecProjectWrapKeepsTimeout(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, slowSop(t, "5"), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Approvals(context.Background(), config.ProjectID(root))
	if !errors.Is(err, ErrApprovalsUnavailable) {
		t.Fatalf("err = %v, want errors.Is(err, ErrApprovalsUnavailable)", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
	}
	if !strings.Contains(err.Error(), `project "`+config.ProjectID(root)+`"`) {
		t.Fatalf("err = %q, want the failing project named", err)
	}
}
