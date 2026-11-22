package egmmanifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func hasKind(rep *egmmanifest.Report, kind string) bool {
	for _, p := range rep.Problems {
		if p.Kind == kind {
			return true
		}
	}
	return false
}

func TestVerifyOK(t *testing.T) {
	m, dir := newFixture(t)
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{ArtifactRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() || rep.Verified != 2 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestVerifyDigestMismatch(t *testing.T) {
	m, _ := newFixture(t)
	m.Cases[0].SeededDefect = "changed-without-reseal"
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "digest-mismatch") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
}

func TestVerifyArtifactMissing(t *testing.T) {
	m, dir := newFixture(t)
	m.Cases[0].ArtifactPath = "missing.md"
	if _, err := egmmanifest.Seal(&m.Cases[0]); err != nil {
		t.Fatal(err)
	}
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{ArtifactRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "artifact-missing") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
}

func TestVerifyArtifactMismatch(t *testing.T) {
	m, dir := newFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "artifact.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{ArtifactRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "artifact-mismatch") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
}

func TestVerifyReferenceMismatch(t *testing.T) {
	m, dir := newFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "refs", "ref.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{ArtifactRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "reference-mismatch") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
}

func TestVerifyLeak(t *testing.T) {
	m, dir := newFixture(t)
	content := syntheticArtifact + syntheticLeak
	if err := os.WriteFile(filepath.Join(dir, "artifact.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Cases[0].ArtifactSHA256 = hashOf(content)
	if _, err := egmmanifest.Seal(&m.Cases[0]); err != nil {
		t.Fatal(err)
	}
	m.Cases[1].ArtifactSHA256 = hashOf(content)
	if _, err := egmmanifest.Seal(&m.Cases[1]); err != nil {
		t.Fatal(err)
	}
	v, _ := egmmanifest.NewVocabulary([]string{syntheticLeak})
	rep, err := egmmanifest.Verify(m, egmmanifest.Options{ArtifactRoot: dir, Vocabulary: v})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "leak") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
	for _, p := range rep.Problems {
		if strings.Contains(egmmanifest.FormatProblem(p), syntheticLeak) {
			t.Fatal("a problem rendered the vocabulary term")
		}
	}
}

func TestVerifyTreeMismatch(t *testing.T) {
	m, dir := newFixture(t)
	single := &egmmanifest.Manifest{SchemaVersion: m.SchemaVersion, Corpus: m.Corpus, Cases: []egmmanifest.Case{m.Cases[0]}}
	rep, err := egmmanifest.Verify(single, egmmanifest.Options{TreeRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("matching tree rejected: %+v", rep.Problems)
	}
	other := t.TempDir()
	rep, err = egmmanifest.Verify(single, egmmanifest.Options{TreeRoot: other})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "tree-mismatch") {
		t.Fatalf("problems = %+v", rep.Problems)
	}
}

func TestVerifyTreeRequiresOneCase(t *testing.T) {
	m, dir := newFixture(t)
	if _, err := egmmanifest.Verify(m, egmmanifest.Options{TreeRoot: dir}); err == nil {
		t.Fatal("expected a one-case error")
	}
}

func TestVerifySchemaFailure(t *testing.T) {
	if _, err := egmmanifest.Verify(&egmmanifest.Manifest{}, egmmanifest.Options{}); err == nil {
		t.Fatal("expected a schema error")
	}
}
