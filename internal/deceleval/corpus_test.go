package deceleval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpusPaths() (corpus, freeze string) {
	base := filepath.Join("..", "..", "evals", "decision-model")
	return filepath.Join(base, "corpus.json"), filepath.Join(base, "CORPUS.sha256")
}

// TestRepoCorpusLoadsValidatesAndIsFrozen proves the committed corpus is
// well-formed, sized within the plan's 50-100 range, covers every category, mixes
// real and synthetic sources, and still matches its frozen hash.
func TestRepoCorpusLoadsValidatesAndIsFrozen(t *testing.T) {
	corpusPath, freezePath := corpusPaths()
	c, err := Load(corpusPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := VerifyFreeze(corpusPath, freezePath); err != nil {
		t.Fatalf("VerifyFreeze: %v", err)
	}
	byCat, bySplit, real, synthetic := c.Counts()
	if n := len(c.Scenarios); n < 50 || n > 100 {
		t.Errorf("corpus size %d, want 50-100", n)
	}
	if real == 0 || synthetic == 0 {
		t.Errorf("expected both real and synthetic sources, got real=%d synthetic=%d", real, synthetic)
	}
	for cat := range categories {
		if byCat[cat] == 0 {
			t.Errorf("category %q has no scenarios", cat)
		}
	}
	if bySplit["eval"] == 0 || bySplit["tuning"] == 0 {
		t.Errorf("split must include both eval and tuning: %v", bySplit)
	}
	// Every label carries a rationale (independently authored).
	for _, s := range c.Scenarios {
		if strings.TrimSpace(s.Expected.Rationale) == "" {
			t.Errorf("scenario %q has no rationale", s.ID)
		}
	}
}

// TestCorpusRejectsMalformed proves the loader/validator fail closed.
func TestCorpusRejectsMalformed(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Unknown field rejected.
	if _, err := Load(write("unknown.json", `{"version":1,"scenarios":[],"bogus":1}`)); err == nil {
		t.Error("unknown field must be rejected")
	}

	mk := func(scn string) string {
		return `{"version":1,"description":"d","actions":["AUTO_FIX","NEEDS_HUMAN"],"scenarios":[` + scn + `]}`
	}
	valid := `{"id":"s1","category":"benign_impl","source":"synthetic","observable":"o","expected":{"action":"AUTO_FIX","rationale":"r"},"split":"eval"}`

	// Unknown action.
	if err := Load2(t, write("badact.json", mk(strings.Replace(valid, `"AUTO_FIX"`, `"MAYBE"`, 1)))).Validate(); err == nil {
		t.Error("unknown action must be rejected")
	}
	// Unknown category.
	if err := Load2(t, write("badcat.json", mk(strings.Replace(valid, `"benign_impl"`, `"misc"`, 1)))).Validate(); err == nil {
		t.Error("unknown category must be rejected")
	}
	// Missing rationale.
	if err := Load2(t, write("norat.json", mk(strings.Replace(valid, `"rationale":"r"`, `"rationale":""`, 1)))).Validate(); err == nil {
		t.Error("missing rationale must be rejected")
	}
	// Bad split.
	if err := Load2(t, write("badsplit.json", mk(strings.Replace(valid, `"split":"eval"`, `"split":"other"`, 1)))).Validate(); err == nil {
		t.Error("bad split must be rejected")
	}
	// Duplicate id.
	dup := `{"version":1,"description":"d","actions":["AUTO_FIX"],"scenarios":[` + valid + `,` + valid + `]}`
	if err := Load2(t, write("dup.json", dup)).Validate(); err == nil {
		t.Error("duplicate id must be rejected")
	}
	// Bad source.
	if err := Load2(t, write("badsrc.json", mk(strings.Replace(valid, `"synthetic"`, `"prod"`, 1)))).Validate(); err == nil {
		t.Error("bad source must be rejected")
	}

	// Freeze mismatch is detected.
	if err := VerifyFreeze(write("c.json", mk(valid)), write("c.sha", "deadbeef  corpus.json")); err == nil {
		t.Error("freeze mismatch must be detected")
	}
}

// Load2 is a small helper: load a corpus path and fail the test on error.
func Load2(t *testing.T, path string) *Corpus {
	t.Helper()
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return c
}
