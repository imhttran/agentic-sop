package shadow

import (
	"encoding/json"
	"testing"

	"github.com/imhttran/sop-decision-adapters/decision"
)

func f(v float64) *float64 { return &v }

func testQuestion(id string) decision.Question {
	return decision.Question{ID: id, Type: decision.QuestionChoice, Criteria: "pick one", Choices: []string{"a", "b"}}
}

// sheet builds a run-sheet exercising agreement, disagreement, timeout,
// malformed, indeterminate, invalid-choice, and a non-comparable gate.
func sheet() RunSheet {
	return RunSheet{
		Sources: []RunSheetSource{
			{Name: "clef", NonComparableCaseIDs: []string{"c4"}},
			{Name: "nimble"},
		},
		Cases: []RunSheetCase{
			{
				ID: "c1", Question: testQuestion("q1"), Primary: "a",
				Reference: &Reference{QuestionID: "q1", Choice: "a"},
				Observations: []RunSheetObservation{
					{Source: "clef", Choice: "a", Confidence: f(0.9), LatencyMS: 12},
					{Source: "nimble", Choice: "a", LatencyMS: 20},
				},
			},
			{
				ID: "c2", Question: testQuestion("q1"), Primary: "b",
				Reference: &Reference{QuestionID: "q1", Choice: "a"},
				Observations: []RunSheetObservation{
					{Source: "clef", Choice: "b", Confidence: f(0.3), LatencyMS: 40},
					{Source: "nimble", Choice: "a", LatencyMS: 5},
				},
			},
			{
				ID: "c3", Question: testQuestion("q1"), Primary: "a", Reference: &Reference{QuestionID: "q1", Choice: "a"},
				Observations: []RunSheetObservation{
					{Source: "clef", Outcome: "timeout", LatencyMS: 30},
					{Source: "nimble", Outcome: "malformed", LatencyMS: 7},
				},
			},
			{
				ID: "c4", Question: testQuestion("q1"), Primary: "a", Reference: &Reference{QuestionID: "q1", Choice: "a"},
				Observations: []RunSheetObservation{
					{Source: "clef", Choice: "z", LatencyMS: 9},
					{Source: "nimble", Choice: "", LatencyMS: 1},
				},
			},
		},
	}
}

func recordsBy(t *testing.T, ev Evidence, caseID, source string) ObservationRecord {
	t.Helper()
	for _, r := range ev.Cases {
		if r.CaseID == caseID && r.Source == source {
			return r
		}
	}
	t.Fatalf("no record for %s/%s", caseID, source)
	return ObservationRecord{}
}

func TestProduceEvidenceDeterministicAndVersioned(t *testing.T) {
	ev, err := ProduceEvidence(sheet())
	if err != nil {
		t.Fatalf("ProduceEvidence: %v", err)
	}
	if ev.SchemaVersion != EvidenceSchemaVersion {
		t.Fatalf("schema version = %d, want %d", ev.SchemaVersion, EvidenceSchemaVersion)
	}
	a, err := MarshalEvidence(ev)
	if err != nil {
		t.Fatal(err)
	}
	ev2, err := ProduceEvidence(sheet())
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalEvidence(ev2)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("evidence is not deterministic:\n%s\n%s", a, b)
	}
	// Round-trip through the reader.
	back, err := ReadEvidence(a)
	if err != nil {
		t.Fatalf("ReadEvidence: %v", err)
	}
	if len(back.Cases) != len(ev.Cases) {
		t.Errorf("round-trip lost cases: %d != %d", len(back.Cases), len(ev.Cases))
	}
}

func TestEvidenceOutcomesAndComparability(t *testing.T) {
	ev, err := ProduceEvidence(sheet())
	if err != nil {
		t.Fatal(err)
	}
	if got := recordsBy(t, ev, "c1", "clef"); got.Outcome != string(LabelAgreement) || got.Comparable != true {
		t.Errorf("c1/clef = %+v, want agreement comparable", got)
	}
	if got := recordsBy(t, ev, "c2", "clef"); got.Outcome != string(LabelDisagreement) {
		t.Errorf("c2/clef = %+v, want disagreement", got)
	}
	if got := recordsBy(t, ev, "c3", "clef"); got.Outcome != string(LabelTimeout) {
		t.Errorf("c3/clef = %+v, want timeout", got)
	}
	if got := recordsBy(t, ev, "c3", "nimble"); got.Outcome != string(LabelMalformed) {
		t.Errorf("c3/nimble = %+v, want malformed", got)
	}
	if got := recordsBy(t, ev, "c4", "clef"); got.Outcome != string(LabelSkippedNotComparable) || got.Comparable {
		t.Errorf("c4/clef = %+v, want skipped_not_comparable and not comparable", got)
	}
	if got := recordsBy(t, ev, "c4", "nimble"); got.Outcome != string(LabelIndeterminate) {
		t.Errorf("c4/nimble = %+v, want indeterminate", got)
	}
	// Provenance, decision, confidence, and latency are persisted per record.
	if got := recordsBy(t, ev, "c1", "clef"); got.Primary != "a" || got.Shadow != "a" || got.Reference != "a" || got.LatencyMS != 12 || got.Confidence == nil || *got.Confidence != 0.9 {
		t.Errorf("c1/clef = %+v, want full provenance/decision/confidence/latency", got)
	}
}

func TestEvidenceValidateRejectsMalformed(t *testing.T) {
	if _, err := ReadEvidence([]byte("{not json")); err == nil {
		t.Error("decode of malformed JSON must fail")
	}
	if _, err := ReadEvidence([]byte("{\"schema_version\": 999, \"cases\": []}")); err == nil {
		t.Error("unknown schema version must be rejected")
	}
	if _, err := ReadEvidence([]byte("{\"schema_version\": 1, \"cases\": [{\"case_id\": \"\", \"source\": \"clef\"}]}")); err == nil {
		t.Error("missing case_id must be rejected")
	}
	if _, err := ReadEvidence([]byte("{\"schema_version\": 1, \"cases\": [{\"case_id\": \"c1\", \"source\": \"\"}]}")); err == nil {
		t.Error("missing source provenance must be rejected")
	}
}

func TestProduceEvidenceFailsClosed(t *testing.T) {
	rs := sheet()
	rs.Cases[1].ID = "c1" // duplicate
	if _, err := ProduceEvidence(rs); err == nil {
		t.Error("duplicate case id must fail closed")
	}
	rs = sheet()
	rs.Cases[0].Observations[0].Source = "unknown"
	if _, err := ProduceEvidence(rs); err == nil {
		t.Error("undeclared observation source must fail closed")
	}
	rs = sheet()
	rs.Cases[0].Question = decision.Question{ID: "", Type: decision.QuestionChoice}
	if _, err := ProduceEvidence(rs); err == nil {
		t.Error("invalid question must fail closed")
	}
}

func TestEvidenceJSONShape(t *testing.T) {
	ev, err := ProduceEvidence(sheet())
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalEvidence(ev)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "cases"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("evidence JSON missing key %q", key)
		}
	}
	var cases []map[string]json.RawMessage
	if err := json.Unmarshal(raw["cases"], &cases); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"case_id", "source", "comparable", "outcome", "latency_ms"} {
		if _, ok := cases[0][key]; !ok {
			t.Errorf("case JSON missing key %q", key)
		}
	}
}
