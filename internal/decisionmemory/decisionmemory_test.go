package decisionmemory

import "testing"

func rec(source string) Record {
	return Record{
		Decision:   "Postgres is canonical storage",
		Scope:      ScopeArchitecture,
		Reason:     "the controller delegates lifecycle authority to agentic-sop",
		Evidence:   []string{"ADR-001"},
		Repository: "repo-1",
		Source:     source,
	}
}

func TestAddValidatesRecord(t *testing.T) {
	if _, err := New(4).Add(Record{}); err == nil {
		t.Error("empty record must be rejected")
	}
	bad := rec("t1")
	bad.Scope = "user"
	if _, err := New(4).Add(bad); err == nil {
		t.Error("invalid scope must be rejected")
	}
	noReason := rec("t1")
	noReason.Reason = ""
	if _, err := New(4).Add(noReason); err == nil {
		t.Error("missing reason must be rejected")
	}
	noSource := rec("t1")
	noSource.Source = ""
	if _, err := New(4).Add(noSource); err == nil {
		t.Error("missing source must be rejected")
	}
	if _, err := New(4).Add(rec("t1")); err != nil {
		t.Errorf("valid record rejected: %v", err)
	}
}

func TestIDStableAndSourceSensitive(t *testing.T) {
	if rec("t1").ID() != rec("t1").ID() {
		t.Fatal("ID is not stable")
	}
	if rec("t1").ID() == rec("t2").ID() {
		t.Error("ID must depend on source")
	}
}

func TestApplicableExcludesOtherRepositories(t *testing.T) {
	s := New(8)
	if _, err := s.Add(rec("t1")); err != nil {
		t.Fatal(err)
	}
	other := rec("t2")
	other.Repository = "repo-2"
	if _, err := s.Add(other); err != nil {
		t.Fatal(err)
	}
	got := s.Applicable(ScopeArchitecture, "repo-1")
	if len(got) != 1 || got[0].Source != "t1" {
		t.Errorf("applicable = %+v, want only repo-1's decision", got)
	}
	stale := s.Stale(ScopeArchitecture, "repo-1")
	if len(stale) != 1 || stale[0].Repository != "repo-2" {
		t.Errorf("stale = %+v, want repo-2's decision", stale)
	}
}

func TestBoundedEviction(t *testing.T) {
	s := New(2)
	r1, r2, r3 := rec("t1"), rec("t2"), rec("t3")
	for _, r := range []Record{r1, r2} {
		if _, err := s.Add(r); err != nil {
			t.Fatal(err)
		}
		if s.Len() > 2 {
			t.Fatalf("N exceeded limit: %d", s.Len())
		}
	}
	if _, err := s.Add(r3); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 2 {
		t.Fatalf("len = %d, want 2 after eviction", s.Len())
	}
	smallest := ""
	for _, id := range []string{r1.ID(), r2.ID(), r3.ID()} {
		if smallest == "" || id < smallest {
			smallest = id
		}
	}
	for _, r := range s.All() {
		if r.ID() == smallest {
			t.Errorf("smallest-ID record was not evicted")
		}
	}
}

func TestMarshalDeterministicAndRoundTrip(t *testing.T) {
	a := New(8)
	for _, src := range []string{"t2", "t1"} {
		if _, err := a.Add(rec(src)); err != nil {
			t.Fatal(err)
		}
	}
	b := New(8)
	for _, src := range []string{"t1", "t2"} {
		if _, err := b.Add(rec(src)); err != nil {
			t.Fatal(err)
		}
	}
	da, _ := a.Marshal()
	db, _ := b.Marshal()
	if string(da) != string(db) {
		t.Errorf("marshal is not order-independent:\n%s\n%s", da, db)
	}
	loaded, err := Unmarshal(da)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.Len() != 2 || len(loaded.Applicable(ScopeArchitecture, "repo-1")) != 2 {
		t.Errorf("round-trip lost records: %+v", loaded.All())
	}
}

func TestUnmarshalRejectsIncompatibleSchema(t *testing.T) {
	c, err := Unmarshal([]byte(`{"schema":"other","limit":4,"records":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Errorf("incompatible schema must load empty, got %d", c.Len())
	}
}

func TestUnmarshalSkipsInvalidRecords(t *testing.T) {
	s, err := Unmarshal([]byte(`{"schema":"decisionmemory/1","limit":4,"records":[{"decision":"","scope":"architecture","reason":"r","repository":"repo-1","source":"t1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 0 {
		t.Errorf("invalid record must be skipped, got %d", s.Len())
	}
}

func TestRemove(t *testing.T) {
	s := New(4)
	r, _ := s.Add(rec("t1"))
	if !s.Remove(r.ID()) {
		t.Error("Remove returned false for an existing record")
	}
	if s.Remove(r.ID()) || s.Len() != 0 {
		t.Error("record was not removed")
	}
}
