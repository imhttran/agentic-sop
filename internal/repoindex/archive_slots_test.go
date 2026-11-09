package repoindex

import "testing"

// TestReadSOPMetadataNewestArchiveSlotWins proves a plan source archived in several
// slots takes the disposition of the most recently recorded slot, even when that
// slot's name sorts first (archive/plan.10 lists before archive/plan.2).
func TestReadSOPMetadataNewestArchiveSlotWins(t *testing.T) {
	root := t.TempDir()
	for _, slot := range []struct{ name, disp, at string }{
		{"plan", "SUPERSEDED", "2026-01-01T00:00:00Z"},
		{"plan.2", "SUPERSEDED", "2026-01-02T00:00:00Z"},
		{"plan.10", "COMPLETE", "2026-01-10T00:00:00Z"},
	} {
		write(t, root, ".agent-sdlc/archive/"+slot.name+"/plan.meta.json", `{"source":"PLAN.md"}`)
		write(t, root, ".agent-sdlc/archive/"+slot.name+"/lifecycle.json",
			`{"disposition":"`+slot.disp+`","recorded_at":"`+slot.at+`"}`)
	}
	if got := readSOPMetadata(root, ".agent-sdlc").archived["PLAN.md"]; got != "complete" {
		t.Fatalf("archived disposition = %q, want complete (newest slot)", got)
	}
}
