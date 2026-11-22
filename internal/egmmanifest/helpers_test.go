package egmmanifest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

const (
	syntheticArtifact = "# Synthetic plan\n\nA synthetic artifact for tests.\n"
	syntheticRef      = "# Synthetic reference\n"
	syntheticAnswer   = "SYNTHETIC-ANSWER-DO-NOT-USE"
	syntheticLeak     = "SYNTHETIC-LEAK-TERM-ALPHA"
)

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newFixture returns a manifest with one seeded and one control case over a synthetic
// artifact root. The tree hash covers the artifact root and is shared by both cases.
func newFixture(t *testing.T) (*egmmanifest.Manifest, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "artifact.md"), syntheticArtifact)
	writeFile(t, filepath.Join(dir, "refs", "ref.md"), syntheticRef)
	tree, err := egmmanifest.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	c1 := egmmanifest.Case{
		ID:                       "S-01",
		Stratum:                  egmmanifest.StratumSeeded,
		ArtifactPath:             "artifact.md",
		ArtifactSHA256:           hashOf(syntheticArtifact),
		ExportedTreeSHA256:       tree,
		RevisionPin:              "synthetic-pin",
		PermittedReferences:      []egmmanifest.Ref{{Path: "refs/ref.md", BlobSHA256: hashOf(syntheticRef)}},
		InstructionBlockSHA256:   hashOf("synthetic instruction"),
		SeededDefect:             "missing-failure-mode",
		DeterminabilityRationale: syntheticAnswer,
		ExpectedFinding:          &egmmanifest.ExpectedFinding{Location: syntheticAnswer, Severity: "HIGH"},
		Distractors:              []string{"synthetic distractor"},
		AuthoredBy:               "seeder",
	}
	if _, err := egmmanifest.Seal(&c1); err != nil {
		t.Fatal(err)
	}
	c2 := egmmanifest.Case{
		ID:                     "C-01",
		Stratum:                egmmanifest.StratumNoKnownDefect,
		ArtifactPath:           "artifact.md",
		ArtifactSHA256:         hashOf(syntheticArtifact),
		ExportedTreeSHA256:     tree,
		RevisionPin:            "synthetic-pin",
		InstructionBlockSHA256: hashOf("synthetic instruction"),
		SeededDefect:           "none",
		ControlCheck:           "independent check recorded",
		AuthoredBy:             "seeder",
	}
	if _, err := egmmanifest.Seal(&c2); err != nil {
		t.Fatal(err)
	}
	return &egmmanifest.Manifest{
		SchemaVersion: egmmanifest.SchemaVersion,
		Corpus:        egmmanifest.CorpusName,
		Cases:         []egmmanifest.Case{c1, c2},
	}, dir
}
