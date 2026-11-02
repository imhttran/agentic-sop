package rageval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDataset copies testdata/valid into dir and gives a handful of
// questions expected sources and one a reworded_from marker, so the summary
// has questions with expected sources to score.
func fixtureDataset(t *testing.T, dir string) *Dataset {
	t.Helper()
	ds, err := Load(filepath.Join("testdata", "valid"))
	if err != nil {
		t.Fatalf("load testdata/valid: %v", err)
	}
	byID := map[string]int{}
	for i, q := range ds.Questions {
		byID[q.ID] = i
	}
	setExpected := func(id, source string) {
		i, ok := byID[id]
		if !ok {
			t.Fatalf("question %s not in testdata/valid", id)
		}
		ds.Questions[i].Expected = []Expected{{Source: source}}
	}
	setExpected("Q-ARCH-01", "docs/a.md")
	setExpected("Q-ARCH-02", "docs/a.md")
	setExpected("Q-FAIL-01", "docs/b.md")
	setExpected("Q-MULTI-01", "docs/c.md")
	setExpected("Q-CODE-01", "docs/d.md")
	setExpected("Q-CODE-02", "docs/e.md")
	ds.Questions[byID["Q-MULTI-01"]].RewordedFrom = "Q-MULTI-00"

	if err := os.MkdirAll(filepath.Join(dir, "corpus"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ds.Questions)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "questions.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	srcCorpus := filepath.Join("testdata", "valid", "corpus")
	err = filepath.Walk(srcCorpus, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcCorpus, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, "corpus", rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func row(qid string, cat Category, arm string, ids []string, recall float64, latency string) Row {
	items := make([]Retrieved, len(ids))
	for i, id := range ids {
		items[i] = Retrieved{ID: id, Rank: i + 1, Corpus: "fixture"}
	}
	return Row{
		QuestionID: qid,
		Category:   cat,
		Arm:        arm,
		Retrieved:  items,
		RecallAt5:  &recall,
		LatencyMS:  latency,
	}
}

func writeRun(t *testing.T, dir string, rows []Row) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(JSONResults{Header: Header{Status: StatusPass}, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// primaryRows builds one primary run. Both primary runs are identical, so
// arm_a_identical is true.
func primaryRows() []Row {
	type spec struct {
		qid  string
		cat  Category
		aIDs []string
		bIDs []string
		aRec float64
		bRec float64
		bLat string
	}
	specs := []spec{
		{"Q-ARCH-01", CategoryArchitectureHistory, []string{"docs/a.md"}, []string{"docs/a.md", "docs/x.md"}, 1.0, 1.0, "100"},
		{"Q-ARCH-02", CategoryArchitectureHistory, []string{"docs/x.md"}, []string{"docs/a.md"}, 0.0, 1.0, "200"},
		{"Q-FAIL-01", CategoryFailureInvestigation, []string{"docs/b.md", "docs/y.md"}, []string{"docs/b.md", "docs/z.md"}, 0.5, 1.0, "300"},
		{"Q-MULTI-01", CategoryMultiHop, []string{"docs/c.md"}, []string{"docs/z.md"}, 1.0, 0.0, Unavailable},
		{"Q-CODE-01", CategoryCode, []string{"docs/d.md"}, []string{"docs/x.md"}, 1.0, 0.0, "400"},
		{"Q-CODE-02", CategoryCode, []string{"docs/e.md"}, []string{"docs/e.md#s1", "docs/e.md#s2"}, 1.0, 1.0, "500"},
	}
	var rows []Row
	for _, s := range specs {
		rows = append(rows, row(s.qid, s.cat, ArmBM25, s.aIDs, s.aRec, Unavailable))
		rows = append(rows, row(s.qid, s.cat, ArmHybrid, s.bIDs, s.bRec, s.bLat))
		rows = append(rows, row(s.qid, s.cat, ArmRAG, s.bIDs, s.bRec, s.bLat))
		rows = append(rows, row(s.qid, s.cat, ArmFull, s.aIDs, s.aRec, Unavailable))
	}
	return rows
}

func controlRows() []Row {
	fill := func(expect string, include bool) []string {
		ids := []string{}
		if include {
			ids = append(ids, expect)
		}
		for len(ids) < 10 {
			ids = append(ids, "docs/filler.md")
		}
		return ids
	}
	type spec struct {
		qid    string
		cat    Category
		expect string
	}
	specs := []spec{
		{"Q-ARCH-01", CategoryArchitectureHistory, "docs/a.md"},
		{"Q-ARCH-02", CategoryArchitectureHistory, "docs/a.md"},
		{"Q-FAIL-01", CategoryFailureInvestigation, "docs/b.md"},
		{"Q-MULTI-01", CategoryMultiHop, "docs/c.md"},
		{"Q-CODE-01", CategoryCode, "docs/d.md"},
		{"Q-CODE-02", CategoryCode, "docs/e.md"},
	}
	var rows []Row
	for _, s := range specs {
		rows = append(rows, row(s.qid, s.cat, ArmBM25, fill(s.expect, true), 1, Unavailable))
		rows = append(rows, row(s.qid, s.cat, ArmFull, fill(s.expect, s.qid != "Q-ARCH-01" && s.qid != "Q-CODE-01"), 0, Unavailable))
	}
	return rows
}

func approxEq(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSummarizeFixture(t *testing.T) {
	tmp := t.TempDir()
	fixtureDataset(t, filepath.Join(tmp, "dataset"))
	run1 := filepath.Join(tmp, "run1")
	run2 := filepath.Join(tmp, "run2")
	control := filepath.Join(tmp, "control")
	out := filepath.Join(tmp, "out")
	writeRun(t, run1, primaryRows())
	writeRun(t, run2, primaryRows())
	writeRun(t, control, controlRows())

	sum, err := Summarize(SummaryConfig{
		Dir:     filepath.Join(tmp, "dataset"),
		Runs:    []string{run1, run2},
		Control: control,
		Out:     out,
	})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON Summary
	if err := json.Unmarshal(raw, &fromJSON); err != nil {
		t.Fatal(err)
	}
	if len(fromJSON.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(fromJSON.Runs))
	}
	if fromJSON.Runs[0].Status != StatusPass || fromJSON.Runs[1].Status != StatusPass {
		t.Fatalf("status = %s/%s, want pass/pass", fromJSON.Runs[0].Status, fromJSON.Runs[1].Status)
	}

	// §4.1 checks from the first run, hand-computed:
	// historical A mean = (1+0+0.5+1)/4 = 0.625, B mean = (1+1+1+0)/4 = 0.75.
	c := sum.Section41
	if c.RunDir != run1 {
		t.Fatalf("section 4.1 run dir = %s, want %s", c.RunDir, run1)
	}
	approxEq(t, c.Value, 0.125)
	if c.Breadth != 2 {
		t.Fatalf("breadth = %d, want 2", c.Breadth)
	}
	if c.CodeRegressions != 1 {
		t.Fatalf("code_regressions = %d, want 1", c.CodeRegressions)
	}
	if c.LatencyP95MS != 500 {
		t.Fatalf("latency_p95_ms = %v, want 500", c.LatencyP95MS)
	}

	// Per-arm per-subset means on the first run.
	approxEq(t, sum.Runs[0].MeanRecallAt5[SubsetHistorical][ArmBM25], 0.625)
	approxEq(t, sum.Runs[0].MeanRecallAt5[SubsetHistorical][ArmHybrid], 0.75)
	approxEq(t, sum.Runs[0].MeanRecallAt5[SubsetCode][ArmHybrid], 0.5)

	if !sum.ArmAIdentical {
		t.Fatal("arm_a_identical = false, want true for two identical runs")
	}

	// Post-hoc full-context score from the first run:
	// B full-context recall: arch01=1, arch02=1, fail01=1, multi01=0,
	// code01=0, code02=1 (two sections of docs/e.md count as one document).
	approxEq(t, sum.PostHoc.MeanFullContext[SubsetHistorical], 0.75)
	approxEq(t, sum.PostHoc.MeanFullContext[SubsetCode], 0.5)
	approxEq(t, sum.PostHoc.MeanFullContext[SubsetMultiHop], 0.0)
	approxEq(t, sum.PostHoc.MeanFullContext[SubsetReworded], 0.0)
	// Distinct documents in each arm B context: 2,1,2,1,1,1 -> mean 8/6.
	approxEq(t, sum.PostHoc.MeanDistinctDocs, 8.0/6.0)
	if sum.PostHoc.HistoricalImproved != 2 || sum.PostHoc.HistoricalRegressed != 1 {
		t.Fatalf("post-hoc historical improved/regressed = %d/%d, want 2/1",
			sum.PostHoc.HistoricalImproved, sum.PostHoc.HistoricalRegressed)
	}

	// Controls from the -control run (top-k 10): arm A recall@10 = 1.0 on
	// every subset; arm D historical = (0+1+1+1)/4 = 0.75, code = (0+1)/2 = 0.5.
	approxEq(t, sum.Control.MeanRecallAt10[SubsetHistorical][ArmBM25], 1.0)
	approxEq(t, sum.Control.MeanRecallAt10[SubsetHistorical][ArmFull], 0.75)
	approxEq(t, sum.Control.MeanRecallAt10[SubsetCode][ArmFull], 0.5)

	md, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "POST-HOC: not part of the §4.1 decision rule") {
		t.Fatal("summary.md missing the POST-HOC heading")
	}
}
