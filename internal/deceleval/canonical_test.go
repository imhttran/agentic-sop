package deceleval

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestCanonicalIgnoresLatency proves the reproducibility projection is independent
// of wall-clock timing: two results that differ only in measured latency must
// produce identical canonical output, while the full artifact still reports that
// latency.
func TestCanonicalIgnoresLatency(t *testing.T) {
	c := loadRepoCorpus(t)
	fast := RunBaseline(c, "x")
	slow := RunBaseline(c, "x")
	for i := range slow.Records {
		slow.Records[i].LatencyMS = 137 + i
	}
	slow.Metrics.LatencyMedianMS = 137
	slow.Metrics.LatencyP95MS = 9001

	canonFast, err := MarshalCanonical(fast)
	if err != nil {
		t.Fatal(err)
	}
	canonSlow, err := MarshalCanonical(slow)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonFast, canonSlow) {
		t.Errorf("canonical projection differs under latency:\n%s\n%s", canonFast, canonSlow)
	}

	fullFast, err := MarshalBaseline(fast)
	if err != nil {
		t.Fatal(err)
	}
	fullSlow, err := MarshalBaseline(slow)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(fullFast, fullSlow) {
		t.Error("the full artifact must still differ, since it reports measured latency")
	}
	if !bytes.Contains(fullSlow, []byte(`"latency_ms": 137`)) {
		t.Error("the full artifact no longer reports per-record latency")
	}
	if !bytes.Contains(fullSlow, []byte(`"latency_median_ms": 137`)) {
		t.Error("the full artifact no longer reports aggregate latency")
	}
}

// TestCanonicalDetectsSemanticChange proves the projection still sees every
// decision-bearing field: changing any one of them must change the canonical hash.
func TestCanonicalDetectsSemanticChange(t *testing.T) {
	c := loadRepoCorpus(t)
	want, err := CanonicalHash(RunBaseline(c, "x"))
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name   string
		mutate func(*BaselineResult)
	}{
		{"record action", func(r *BaselineResult) { r.Records[0].Action = flippedAction(r.Records[0].Action) }},
		{"record kind", func(r *BaselineResult) { r.Records[0].Kind += "X" }},
		{"record disposition", func(r *BaselineResult) { r.Records[0].Disposition += "X" }},
		{"record confidence", func(r *BaselineResult) { r.Records[0].Confidence += "X" }},
		{"record category", func(r *BaselineResult) { r.Records[0].Category += "X" }},
		{"record source", func(r *BaselineResult) { r.Records[0].Source += "X" }},
		{"record expected", func(r *BaselineResult) { r.Records[0].Expected = flippedAction(r.Records[0].Expected) }},
		{"record evidence hash", func(r *BaselineResult) { r.Records[0].EvidenceHash = "deadbeef" }},
		{"record fixture error", func(r *BaselineResult) { r.Records[0].FixtureError = "unmappable" }},
		{"corpus sha", func(r *BaselineResult) { r.CorpusSHA256 = "deadbeef" }},
		{"metric correct", func(r *BaselineResult) { r.Metrics.Correct++ }},
		{"metric incorrect", func(r *BaselineResult) { r.Metrics.Incorrect++ }},
		{"metric unsafe missed escalation", func(r *BaselineResult) { r.Metrics.UnsafeMissedEscalation++ }},
		{"metric false escalation", func(r *BaselineResult) { r.Metrics.FalseEscalation++ }},
		{"metric fixture failures", func(r *BaselineResult) { r.Metrics.FixtureFailures++ }},
		{"second label agreement", func(r *BaselineResult) { r.SecondLabel.Agreements++ }},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			changed := RunBaseline(c, "x")
			m.mutate(&changed)
			got, err := CanonicalHash(changed)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Errorf("canonical projection did not change after mutating %s", m.name)
			}
		})
	}
}

// TestCanonicalDoesNotMutateResult proves canonicalization copies rather than
// edits in place: the caller's result, its measured latency, and its record order
// are all left untouched.
func TestCanonicalDoesNotMutateResult(t *testing.T) {
	c := loadRepoCorpus(t)
	res := RunBaseline(c, "x")
	for i := range res.Records {
		res.Records[i].LatencyMS = i + 1
	}
	res.Metrics.LatencyMedianMS = 42
	res.Metrics.LatencyP95MS = 99

	ids := make([]string, len(res.Records))
	latency := make([]int, len(res.Records))
	for i, r := range res.Records {
		ids[i], latency[i] = r.ScenarioID, r.LatencyMS
	}
	before, err := MarshalBaseline(res)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := MarshalCanonical(res); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalHash(res); err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalBaseline(before); err != nil {
		t.Fatal(err)
	}

	after, err := MarshalBaseline(res)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("canonicalization mutated the result:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	for i, r := range res.Records {
		if r.ScenarioID != ids[i] || r.LatencyMS != latency[i] {
			t.Errorf("record %d changed: %s/%d -> %s/%d", i, ids[i], latency[i], r.ScenarioID, r.LatencyMS)
		}
	}
	if res.Metrics.LatencyMedianMS != 42 || res.Metrics.LatencyP95MS != 99 {
		t.Error("canonicalization mutated the latency metrics")
	}
}

// TestCanonicalRetainsEveryField guards the projection's completeness: it must
// carry every BaselineRecord and Metrics field except the measured latency, and
// invent none. Without this guard, a future semantic field could be added to the
// records and silently drop out of drift detection.
func TestCanonicalRetainsEveryField(t *testing.T) {
	latency := map[string]bool{
		"latency_ms":        true, // per-record measured latency
		"latency_median_ms": true, // aggregate measured latency
		"latency_p95_ms":    true, // aggregate measured latency
	}
	cases := []struct {
		name  string
		src   any
		canon any
	}{
		{"BaselineRecord", BaselineRecord{}, CanonicalRecord{}},
		{"Metrics", Metrics{}, CanonicalMetrics{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := jsonFieldNames(tc.src)
			canon := jsonFieldNames(tc.canon)
			for f := range src {
				if latency[f] {
					if canon[f] {
						t.Errorf("canonical %s must not carry latency field %q", tc.name, f)
					}
					continue
				}
				if !canon[f] {
					t.Errorf("canonical %s drops semantic field %q; add it to the projection", tc.name, f)
				}
			}
			for f := range canon {
				if !src[f] {
					t.Errorf("canonical %s invents field %q", tc.name, f)
				}
			}
		})
	}
}

// jsonFieldNames returns the JSON names of a struct's fields.
func jsonFieldNames(v any) map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}
