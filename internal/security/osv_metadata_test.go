package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeOSVMetadata writes the optional acquisition sidecar beside an existing
// all.zip. The caller supplies the recorded fields so a test can describe a
// copied, stale, or corrupt acquisition without downloading anything.
func writeOSVMetadata(t *testing.T, archive string, metadata map[string]any) {
	t.Helper()
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive+".metadata.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func osvDatabaseArchive(t *testing.T, root, ecosystem string, modified time.Time) string {
	t.Helper()
	writeOSVDatabase(t, root, []string{ecosystem}, modified)
	return filepath.Join(root, "osv-scalibr", ecosystem, "all.zip")
}

// TestOSVDatabaseFreshnessUsesAcquisitionTime reproduces a copied database that
// looks fresh because the copy rewrote its mtime. The sidecar's downloaded_at is
// the acquisition evidence and must decide freshness.
func TestOSVDatabaseFreshnessUsesAcquisitionTime(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	archive := osvDatabaseArchive(t, root, "Go", now)

	digest, err := rulesIdentity(archive)
	if err != nil || digest == "" {
		t.Fatalf("database digest: %q, %v", digest, err)
	}
	acquired := now.Add(-120 * 24 * time.Hour).Truncate(time.Second)
	writeOSVMetadata(t, archive, map[string]any{
		"ecosystem":     "Go",
		"source":        "https://osv-vulnerabilities.storage.googleapis.com/Go/all.zip",
		"sha256":        digest,
		"size":          0,
		"downloaded_at": acquired.Format(time.RFC3339),
	})

	db, err := inspectOSVDatabase("Go", archive)
	if err != nil {
		t.Fatal(err)
	}
	if !osvDatabaseStale([]osvDatabase{db}, now) {
		t.Fatal("database with a 120-day-old acquisition time was treated as fresh because its mtime was current")
	}
}

// TestOSVDatabaseMalformedMetadataFailsClosed proves a damaged sidecar is not
// silently ignored in favor of a fresh mtime.
func TestOSVDatabaseMalformedMetadataFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := osvDatabaseArchive(t, root, "Go", time.Now().UTC())
	if err := os.WriteFile(archive+".metadata.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectOSVDatabase("Go", archive); err == nil {
		t.Fatal("malformed acquisition metadata was accepted")
	}
}

// TestOSVDatabaseUnreadableAcquisitionTimeFailsClosed proves an unusable
// downloaded_at cannot fall back to mtime.
func TestOSVDatabaseUnreadableAcquisitionTimeFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := osvDatabaseArchive(t, root, "Go", time.Now().UTC())
	digest, err := rulesIdentity(archive)
	if err != nil {
		t.Fatal(err)
	}
	writeOSVMetadata(t, archive, map[string]any{
		"ecosystem":     "Go",
		"sha256":        digest,
		"downloaded_at": "yesterday-ish",
	})
	if _, err := inspectOSVDatabase("Go", archive); err == nil {
		t.Fatal("unreadable downloaded_at was accepted")
	}
}

// TestOSVDatabaseWithoutMetadataUsesModified keeps the documented fallback for a
// database provisioned without the optional sidecar.
func TestOSVDatabaseWithoutMetadataUsesModified(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	fresh := osvDatabaseArchive(t, root, "Go", now)
	freshDB, err := inspectOSVDatabase("Go", fresh)
	if err != nil {
		t.Fatal(err)
	}
	if osvDatabaseStale([]osvDatabase{freshDB}, now) {
		t.Fatal("fresh database without metadata was treated as stale")
	}

	oldRoot := t.TempDir()
	old := osvDatabaseArchive(t, oldRoot, "Go", now.Add(-120*24*time.Hour))
	oldDB, err := inspectOSVDatabase("Go", old)
	if err != nil {
		t.Fatal(err)
	}
	if !osvDatabaseStale([]osvDatabase{oldDB}, now) {
		t.Fatal("old database without metadata was treated as fresh")
	}
}

// TestOSVDatabaseDigestMismatchFailsClosed proves a sidecar that names a
// different archive cannot be accepted as provenance.
func TestOSVDatabaseDigestMismatchFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := osvDatabaseArchive(t, root, "Go", time.Now().UTC())
	writeOSVMetadata(t, archive, map[string]any{
		"ecosystem":     "Go",
		"sha256":        "0000000000000000000000000000000000000000000000000000000000000000",
		"downloaded_at": time.Now().UTC().Format(time.RFC3339),
	})
	if _, err := inspectOSVDatabase("Go", archive); err == nil {
		t.Fatal("a sidecar digest that disagrees with the archive was accepted")
	}
}

// TestOSVDatabaseUnreadableMetadataPathFailsClosed proves a sidecar that cannot
// be read for a reason other than absence (for example a directory at the
// sidecar path) is not silently replaced by the mtime fallback.
func TestOSVDatabaseUnreadableMetadataPathFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := osvDatabaseArchive(t, root, "Go", time.Now().UTC())
	if err := os.Mkdir(archive+".metadata.json", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectOSVDatabase("Go", archive); err == nil {
		t.Fatal("an unreadable acquisition sidecar was accepted")
	}
}

// TestOSVDatabaseSizeMismatchFailsClosed proves a sidecar that records a
// different archive size cannot be accepted as provenance.
func TestOSVDatabaseSizeMismatchFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := osvDatabaseArchive(t, root, "Go", time.Now().UTC())
	digest, err := rulesIdentity(archive)
	if err != nil {
		t.Fatal(err)
	}
	writeOSVMetadata(t, archive, map[string]any{
		"ecosystem":     "Go",
		"sha256":        digest,
		"size":          -1,
		"downloaded_at": time.Now().UTC().Format(time.RFC3339),
	})
	if _, err := inspectOSVDatabase("Go", archive); err == nil {
		t.Fatal("a sidecar size that disagrees with the archive was accepted")
	}
}
