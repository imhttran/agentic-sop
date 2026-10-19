package decisionquality

import (
	"path/filepath"
	"strings"
	"testing"
)

const pilotDir = "../../evals/decision-memory-pilot"

func loadPilot(t *testing.T, name string) Corpus {
	t.Helper()
	c, err := LoadCorpus(filepath.Join(pilotDir, name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return *c
}

func TestPilotCorpusSatisfiesPilotContract(t *testing.T) {
	tuning := loadPilot(t, "corpus-tuning.json")
	heldout := loadPilot(t, "corpus-heldout.json")
	if err := ValidatePilot(tuning); err != nil {
		t.Fatalf("tuning: %v", err)
	}
	if err := ValidatePilot(heldout); err != nil {
		t.Fatalf("heldout: %v", err)
	}
	if len(heldout.Cases) < 20 {
		t.Fatalf("held-out cases = %d, want at least 20", len(heldout.Cases))
	}
}

func TestPilotFamiliesDoNotStraddleSplits(t *testing.T) {
	tuning := loadPilot(t, "corpus-tuning.json")
	heldout := loadPilot(t, "corpus-heldout.json")
	family := func(id string) string {
		parts := strings.SplitN(id, "-", 3)
		if len(parts) >= 2 {
			return parts[0] + "-" + parts[1]
		}
		return id
	}
	inTuning := map[string]bool{}
	for _, c := range tuning.Cases {
		inTuning[family(c.ID)] = true
	}
	for _, c := range heldout.Cases {
		if inTuning[family(c.ID)] {
			t.Errorf("family %q appears in both tuning and held-out", family(c.ID))
		}
	}
}

func TestPilotFreezeSidecars(t *testing.T) {
	for _, name := range []string{"PROTOCOL.sha256", "CORPUS.sha256"} {
		if err := VerifyFreeze(pilotDir, filepath.Join(pilotDir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPilotEvidenceDoesNotContainTheLabel(t *testing.T) {
	for _, name := range []string{"corpus-tuning.json", "corpus-heldout.json"} {
		c := loadPilot(t, name)
		for _, cs := range c.Cases {
			for _, ev := range cs.PreDecisionEvidence {
				if strings.Contains(strings.ToLower(ev.Text), "disposition") {
					t.Errorf("%s/%s: evidence leaks the disposition: %q", name, cs.ID, ev.Text)
				}
			}
		}
	}
}
