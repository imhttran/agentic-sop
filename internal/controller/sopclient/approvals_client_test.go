package sopclient

// This file covers the Client.Approvals error contract (the fix for the
// silent-empty-listing defect): a genuine backend failure must be surfaced as a
// non-nil error, never swallowed into an unreported empty listing that would be
// indistinguishable from "SOP reported no gates".

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// fakeStdoutSop writes an executable "sop" that prints stdout for every
// invocation and exits 0, so the Client.Approvals fallback path (no persisted
// approvals artifact) can be exercised without a real binary.
func fakeStdoutSop(t *testing.T, stdout string) string {
	t.Helper()
	dir := t.TempDir()
	payload := filepath.Join(dir, "stdout")
	bin := filepath.Join(dir, "sop")
	if err := os.WriteFile(payload, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat " + payload + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// A successful `sop approvals --json` invocation with a well-formed listing is
// returned verbatim with a nil error.
func TestClientApprovalsReturnsListing(t *testing.T) {
	root := newProject(t)
	bin := fakeStdoutSop(t, approvalsFixture)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	listing, err := c.Approvals(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatalf("Approvals err = %v, want nil", err)
	}
	if !listing.Reported || len(listing.Entries) != 3 {
		t.Fatalf("listing = %+v, want a reported listing with 3 entries", listing)
	}
}

// A genuine backend failure (no persisted artifact AND the verb exits non-zero)
// is surfaced as a non-nil error wrapping ErrApprovalsUnavailable, NOT swallowed
// into an unreported empty listing. This is the precise defect the fix closes: a
// failing SOP must never be indistinguishable from a quiet one.
func TestClientApprovalsSurfacesBackendFailure(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeFailingSop(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	listing, err := c.Approvals(context.Background(), config.ProjectID(root))
	if err == nil {
		t.Fatal("Approvals err = nil on a failed backend; a failing SOP must not look like an empty listing")
	}
	if !errors.Is(err, ErrApprovalsUnavailable) {
		t.Errorf("err = %v, want ErrApprovalsUnavailable", err)
	}
	if listing.Reported || len(listing.Entries) != 0 {
		t.Errorf("listing = %+v, want an unreported listing alongside the error", listing)
	}
}

// Unparsable `sop approvals --json` output is likewise surfaced as an error.
func TestClientApprovalsSurfacesUnparsable(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeStdoutSop(t, "this is not JSON"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Approvals(context.Background(), config.ProjectID(root)); !errors.Is(err, ErrApprovalsUnavailable) {
		t.Errorf("err = %v, want ErrApprovalsUnavailable", err)
	}
}

// A genuine empty listing (well-formed JSON with no entries) is NOT an error: it
// returns Reported=true so the caller can distinguish "SOP reported no gates"
// from "SOP could not be read".
func TestClientApprovalsEmptyListingIsNotError(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeStdoutSop(t, `{"approvals":[]}`), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	listing, err := c.Approvals(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatalf("Approvals err = %v, want nil for an empty listing", err)
	}
	if !listing.Reported || len(listing.Entries) != 0 {
		t.Fatalf("listing = %+v, want Reported with no entries", listing)
	}
}

// An unknown project yields ErrProjectNotFound, never a backend-failure error.
func TestClientApprovalsUnknownProject(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeFailingSop(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Approvals(context.Background(), "nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("err = %v, want ErrProjectNotFound", err)
	}
}
