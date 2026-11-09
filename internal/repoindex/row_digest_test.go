package repoindex

import (
	"errors"
	"testing"
)

func fileByPath(idx Index, path string) *File {
	for i := range idx.Files {
		if idx.Files[i].Path == path {
			return &idx.Files[i]
		}
	}
	return nil
}

// GAP-4 (PROPOSAL-REPOINDEX-ROW-DIGESTS.md option A): every File and Document
// row carries its own digest, VerifyRows recomputes it, and malformed, mismatched
// and pre-v2 indexes are distinguishable.
func TestRowDigests(t *testing.T) {
	root := fixture(t)
	idx := build(t, root)
	if idx.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2", idx.SchemaVersion)
	}
	if err := VerifyRows(root, idx); err != nil {
		t.Fatalf("fresh index: %v", err)
	}
	if again := build(t, root); fileByPath(again, "docs/plans/ACTIVE.md").Digest != fileByPath(idx, "docs/plans/ACTIVE.md").Digest {
		t.Fatal("identical trees must produce identical row digests")
	}

	// A one-byte content change changes that row and is caught on the old index.
	write(t, root, "docs/plans/ACTIVE.md", "# Active plan!\n")
	if err := VerifyRows(root, idx); !errors.Is(err, ErrRowDigestMismatch) {
		t.Fatalf("stale index after content change: err = %v, want mismatch", err)
	}
	changed := build(t, root)
	if fileByPath(changed, "docs/plans/ACTIVE.md").Digest == fileByPath(idx, "docs/plans/ACTIVE.md").Digest {
		t.Fatal("a one-byte change must change the file row digest")
	}

	// A metadata-only authority change moves the document row, not any file row.
	write(t, root, ".agent-sdlc/plan.meta.json", `{"source":"docs/plans/PLAN-Future.md","plan_id":"future","source_kind":"plan"}`)
	meta := build(t, root)
	if docByPath(meta, "docs/plans/PLAN-Future.md").Digest == docByPath(changed, "docs/plans/PLAN-Future.md").Digest {
		t.Fatal("an authority change must change the document row digest")
	}
	for i := range meta.Files {
		if meta.Files[i] != changed.Files[i] {
			t.Fatalf("a metadata-only change moved file row %s", meta.Files[i].Path)
		}
	}

	// A hand-edited row is a mismatch; an absent or malformed digest is distinct.
	tampered := meta
	tampered.Documents = append([]Document(nil), meta.Documents...)
	tampered.Documents[0].Authority = AuthorityHistorical
	if err := VerifyRows(root, tampered); !errors.Is(err, ErrRowDigestMismatch) {
		t.Fatalf("tampered row: err = %v, want mismatch", err)
	}
	tampered.Documents[0] = meta.Documents[0]
	tampered.Documents[0].Digest = ""
	if err := VerifyRows(root, tampered); !errors.Is(err, ErrRowDigestMalformed) {
		t.Fatalf("absent digest: err = %v, want malformed", err)
	}

	// A v1 index is unverified, never valid.
	meta.SchemaVersion = 1
	if err := VerifyRows(root, meta); !errors.Is(err, ErrRowsUnverified) {
		t.Fatalf("v1 index: err = %v, want unverified", err)
	}
}
