// Deterministic summarizer for recorded rag-eval runs.
//
// Summarize reads only files: the dataset at Dir and the results.json files
// of each recorded run. It never runs a producer, a model, or a database, and
// it computes every number the decision report uses so no number is worked
// out by hand.

package rageval

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PostHocHeading marks the sections that sit outside the §4.1 decision rule.
const PostHocHeading = "POST-HOC: not part of the §4.1 decision rule"

// §4.1 thresholds from docs/specs/RAG-EVALUATION.md.
const (
	thresholdValue        = 0.10
	thresholdBreadth      = 5
	thresholdCodeRegress  = 1
	thresholdLatencyP95MS = 500
	controlTopK           = 10
)

// Subset names.
const (
	SubsetHistorical = "historical"
	SubsetCode       = "code"
	SubsetMultiHop   = "multi_hop"
	SubsetReworded   = "reworded"
)

// Subsets lists the subset names in a stable order.
var Subsets = []string{SubsetHistorical, SubsetCode, SubsetMultiHop, SubsetReworded}

// historicalCategories are the categories in the historical subset.
var historicalCategories = map[Category]bool{
	CategoryArchitectureHistory:  true,
	CategoryDesignDecision:       true,
	CategoryFailureInvestigation: true,
	CategoryMultiHop:             true,
}

// SummaryConfig names the inputs and output of a summarize call.
type SummaryConfig struct {
	// Dir is the dataset root, loaded with Load.
	Dir string
	// Runs are the primary run directories, each holding results.json.
	Runs []string
	// Control is the control run directory, recorded with -top-k 10.
	Control string
	// Out is the directory that receives summary.json and summary.md.
	Out string
}

// RunSummary is the computed summary for one run.
type RunSummary struct {
	// Dir is the run directory the numbers came from.
	Dir string
	// Status is the run header status.
	Status Status
	// MeanRecallAt5 maps subset -> arm -> mean recall@5.
	MeanRecallAt5 map[string]map[string]float64
}

// Section41 holds the §4.1 checks computed from the first run.
type Section41 struct {
	// RunDir is the run the checks came from.
	RunDir string
	// Status is the run header status.
	Status Status
	// Value is mean arm B minus mean arm A on historical.
	Value float64
	// Breadth counts historical questions where B recall@5 beats A.
	Breadth int
	// CodeRegressions counts code questions where B recall@5 is below A.
	CodeRegressions int
	// LatencyP95MS is the nearest-rank p95 of arm B numeric latency_ms.
	LatencyP95MS float64
}

// PostHoc is the post-hoc full-context score from the first run.
type PostHoc struct {
	// RunDir is the run the score came from.
	RunDir string
	// MeanFullContext maps subset -> mean RecallAtK at full depth for arm B.
	MeanFullContext map[string]float64
	// MeanDistinctDocs is the mean number of distinct documents in arm B context.
	MeanDistinctDocs float64
	// HistoricalImproved counts historical questions where B full-context
	// recall beats A recall@5.
	HistoricalImproved int
	// HistoricalRegressed counts historical questions where B full-context
	// recall is below A recall@5.
	HistoricalRegressed int
}

// ControlSummary holds the control means from the control run.
type ControlSummary struct {
	// RunDir is the control run directory.
	RunDir string
	// MeanRecallAt10 maps subset -> arm -> mean RecallAtK(..., 10).
	MeanRecallAt10 map[string]map[string]float64
}

// Summary is the whole computed summary.
type Summary struct {
	Runs          []RunSummary
	Section41     Section41
	ArmAIdentical bool
	PostHoc       PostHoc
	Control       ControlSummary
}

// subsetsOf returns the subset names a question belongs to. Questions with
// no expected sources belong to no subset.
func subsetsOf(q Question) []string {
	if len(expectedIDs(q)) == 0 {
		return nil
	}
	var out []string
	if historicalCategories[q.Category] {
		out = append(out, SubsetHistorical)
	}
	if q.Category == CategoryCode {
		out = append(out, SubsetCode)
	}
	if q.Category == CategoryMultiHop {
		out = append(out, SubsetMultiHop)
	}
	if q.RewordedFrom != "" {
		out = append(out, SubsetReworded)
	}
	return out
}

// loadRun reads one results.json written by rag-eval run.
func loadRun(dir string) (JSONResults, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "results.json"))
	if err != nil {
		return JSONResults{}, fmt.Errorf("rageval: read %s: %w", dir, err)
	}
	var res JSONResults
	if err := json.Unmarshal(raw, &res); err != nil {
		return JSONResults{}, fmt.Errorf("rageval: parse %s: %w", dir, err)
	}
	return res, nil
}

// rowRecall returns the row recall@5 and whether the row carries one.
func rowRecall(r Row) (float64, bool) {
	if r.RecallAt5 == nil {
		return 0, false
	}
	return *r.RecallAt5, true
}

// meanRecallAt5 computes subset -> arm -> mean recall@5 for one run.
func meanRecallAt5(ds *Dataset, res JSONResults) map[string]map[string]float64 {
	questions := map[string]Question{}
	for _, q := range ds.Questions {
		questions[q.ID] = q
	}
	sums := map[string]map[string]float64{}
	counts := map[string]map[string]int{}
	for _, row := range res.Rows {
		q, ok := questions[row.QuestionID]
		if !ok {
			continue
		}
		v, ok := rowRecall(row)
		if !ok {
			continue
		}
		for _, sub := range subsetsOf(q) {
			if sums[sub] == nil {
				sums[sub] = map[string]float64{}
				counts[sub] = map[string]int{}
			}
			sums[sub][row.Arm] += v
			counts[sub][row.Arm]++
		}
	}
	out := map[string]map[string]float64{}
	for sub, arms := range sums {
		m := map[string]float64{}
		for arm, s := range arms {
			if counts[sub][arm] > 0 {
				m[arm] = s / float64(counts[sub][arm])
			}
		}
		out[sub] = m
	}
	return out
}

// percentile95 returns the nearest-rank 95th percentile of the values.
func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := make([]float64, len(values))
	copy(ordered, values)
	sort.Float64s(ordered)
	rank := int(math.Ceil(0.95 * float64(len(ordered))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(ordered) {
		rank = len(ordered)
	}
	return ordered[rank-1]
}

// armRows returns the rows of one arm keyed by question id.
func armRows(res JSONResults, arm string) map[string]Row {
	out := map[string]Row{}
	for _, r := range res.Rows {
		if r.Arm == arm {
			out[r.QuestionID] = r
		}
	}
	return out
}

// computeSection41 computes the §4.1 checks from the first run.
func computeSection41(ds *Dataset, dir string, res JSONResults, means map[string]map[string]float64) Section41 {
	s := Section41{RunDir: dir, Status: res.Header.Status}
	hist := means[SubsetHistorical]
	if b, ok := hist[ArmHybrid]; ok {
		if a, ok := hist[ArmBM25]; ok {
			s.Value = b - a
		}
	}
	a := armRows(res, ArmBM25)
	questions := map[string]Question{}
	for _, q := range ds.Questions {
		questions[q.ID] = q
	}
	var latencies []float64
	for _, row := range res.Rows {
		if row.Arm != ArmHybrid {
			continue
		}
		if v, err := strconv.ParseFloat(row.LatencyMS, 64); err == nil {
			latencies = append(latencies, v)
		}
		q, ok := questions[row.QuestionID]
		if !ok || len(expectedIDs(q)) == 0 {
			continue
		}
		ra, okA := rowRecall(a[row.QuestionID])
		rb, okB := rowRecall(row)
		if !okA || !okB {
			continue
		}
		if historicalCategories[q.Category] && rb > ra {
			s.Breadth++
		}
		if q.Category == CategoryCode && rb < ra {
			s.CodeRegressions++
		}
	}
	s.LatencyP95MS = percentile95(latencies)
	return s
}

// armAList returns the arm A (question_id, retrieved IDs in order) list.
func armAList(res JSONResults) []string {
	var out []string
	for _, r := range res.Rows {
		if r.Arm != ArmBM25 {
			continue
		}
		ids := make([]string, 0, len(r.Retrieved))
		for _, it := range stableByRank(r.Retrieved) {
			ids = append(ids, it.ID)
		}
		out = append(out, r.QuestionID+"|"+strings.Join(ids, ","))
	}
	return out
}

// listsEqual reports whether two string slices are equal.
func listsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// computePostHoc computes the post-hoc full-context score from the first run.
func computePostHoc(ds *Dataset, dir string, res JSONResults) PostHoc {
	p := PostHoc{RunDir: dir, MeanFullContext: map[string]float64{}}
	questions := map[string]Question{}
	for _, q := range ds.Questions {
		questions[q.ID] = q
	}
	a := armRows(res, ArmBM25)
	sums := map[string]float64{}
	counts := map[string]int{}
	var distinctSums float64
	var distinctCount int
	for _, row := range res.Rows {
		if row.Arm != ArmHybrid {
			continue
		}
		q, ok := questions[row.QuestionID]
		if !ok || len(expectedIDs(q)) == 0 {
			continue
		}
		full, ok := RecallAtK(expectedIDs(q), row.Retrieved, len(row.Retrieved))
		if !ok {
			continue
		}
		for _, sub := range subsetsOf(q) {
			sums[sub] += full
			counts[sub]++
		}
		distinctSums += float64(len(RankedIDs(row.Retrieved, len(row.Retrieved))))
		distinctCount++
		if historicalCategories[q.Category] {
			if ra, okA := rowRecall(a[row.QuestionID]); okA {
				if full > ra {
					p.HistoricalImproved++
				} else if full < ra {
					p.HistoricalRegressed++
				}
			}
		}
	}
	for sub, s := range sums {
		if counts[sub] > 0 {
			p.MeanFullContext[sub] = s / float64(counts[sub])
		}
	}
	if distinctCount > 0 {
		p.MeanDistinctDocs = distinctSums / float64(distinctCount)
	}
	return p
}

// computeControl computes the control means with RecallAtK(..., 10).
func computeControl(ds *Dataset, dir string, res JSONResults) ControlSummary {
	c := ControlSummary{RunDir: dir, MeanRecallAt10: map[string]map[string]float64{}}
	questions := map[string]Question{}
	for _, q := range ds.Questions {
		questions[q.ID] = q
	}
	sums := map[string]map[string]float64{}
	counts := map[string]map[string]int{}
	for _, row := range res.Rows {
		q, ok := questions[row.QuestionID]
		if !ok || len(expectedIDs(q)) == 0 {
			continue
		}
		v, ok := RecallAtK(expectedIDs(q), row.Retrieved, controlTopK)
		if !ok {
			continue
		}
		if row.Arm != ArmBM25 && row.Arm != ArmFull {
			continue
		}
		for _, sub := range subsetsOf(q) {
			if sums[sub] == nil {
				sums[sub] = map[string]float64{}
				counts[sub] = map[string]int{}
			}
			sums[sub][row.Arm] += v
			counts[sub][row.Arm]++
		}
	}
	for sub, arms := range sums {
		m := map[string]float64{}
		for arm, s := range arms {
			if counts[sub][arm] > 0 {
				m[arm] = s / float64(counts[sub][arm])
			}
		}
		c.MeanRecallAt10[sub] = m
	}
	return c
}

// Summarize computes the whole summary from the recorded runs and writes
// summary.json and summary.md into cfg.Out. It reads only files.
func Summarize(cfg SummaryConfig) (*Summary, error) {
	if len(cfg.Runs) == 0 {
		return nil, fmt.Errorf("rageval: summarize: at least one -runs directory is required")
	}
	ds, err := Load(cfg.Dir)
	if err != nil {
		return nil, err
	}
	s := &Summary{ArmAIdentical: true}
	var first JSONResults
	var firstDir string
	var firstMeans map[string]map[string]float64
	for _, dir := range cfg.Runs {
		res, err := loadRun(dir)
		if err != nil {
			return nil, err
		}
		means := meanRecallAt5(ds, res)
		s.Runs = append(s.Runs, RunSummary{Dir: dir, Status: res.Header.Status, MeanRecallAt5: means})
		if firstDir == "" {
			first, firstDir, firstMeans = res, dir, means
		} else if !listsEqual(armAList(res), armAList(first)) {
			s.ArmAIdentical = false
		}
	}
	s.Section41 = computeSection41(ds, firstDir, first, firstMeans)
	s.PostHoc = computePostHoc(ds, firstDir, first)
	controlRes, err := loadRun(cfg.Control)
	if err != nil {
		return nil, err
	}
	s.Control = computeControl(ds, cfg.Control, controlRes)
	if err := writeSummaryFiles(cfg.Out, s); err != nil {
		return nil, err
	}
	return s, nil
}

// summaryJSON is the summary.json shape.
type summaryJSON struct {
	Runs          []RunSummary   `json:"runs"`
	Section41     Section41      `json:"section_4_1"`
	ArmAIdentical bool           `json:"arm_a_identical"`
	PostHoc       PostHoc        `json:"post_hoc"`
	Control       ControlSummary `json:"control"`
}

// writeSummaryFiles renders summary.json and summary.md into out.
func writeSummaryFiles(out string, s *Summary) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("rageval: create summary dir: %w", err)
	}
	body, err := json.MarshalIndent(summaryJSON{
		Runs:          s.Runs,
		Section41:     s.Section41,
		ArmAIdentical: s.ArmAIdentical,
		PostHoc:       s.PostHoc,
		Control:       s.Control,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("rageval: marshal summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, "summary.json"), append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("rageval: write summary.json: %w", err)
	}
	md := renderSummaryMarkdown(s)
	if err := os.WriteFile(filepath.Join(out, "summary.md"), []byte(md), 0o644); err != nil {
		return fmt.Errorf("rageval: write summary.md: %w", err)
	}
	return nil
}

// renderSummaryMarkdown renders summary.md. Every number names its run
// directory; the post-hoc and control sections carry the POST-HOC heading.
func renderSummaryMarkdown(s *Summary) string {
	var b strings.Builder
	b.WriteString("# RAG evaluation summary\n\n")
	b.WriteString("## Mean recall@5 per run, arm, and subset\n\n")
	for _, r := range s.Runs {
		fmt.Fprintf(&b, "### run %s (status: %s)\n\n", r.Dir, r.Status)
		b.WriteString("| subset | A | B | C | D |\n| --- | --- | --- | --- | --- |\n")
		for _, sub := range Subsets {
			arms := r.MeanRecallAt5[sub]
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", sub,
				fmtMeanOr(arms, ArmBM25), fmtMeanOr(arms, ArmHybrid), fmtMeanOr(arms, ArmRAG), fmtMeanOr(arms, ArmFull))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## §4.1 checks (run %s, status: %s)\n\n", s.Section41.RunDir, s.Section41.Status)
	fmt.Fprintf(&b, "- value (mean B - mean A, historical): %.4f (threshold >= %.2f)\n", s.Section41.Value, thresholdValue)
	fmt.Fprintf(&b, "- breadth (historical questions where B > A): %d (threshold >= %d)\n", s.Section41.Breadth, thresholdBreadth)
	fmt.Fprintf(&b, "- code regressions (code questions where B < A): %d (threshold <= %d)\n", s.Section41.CodeRegressions, thresholdCodeRegress)
	fmt.Fprintf(&b, "- latency p95 ms (arm B, run %s): %.0f (threshold <= %d)\n", s.Section41.RunDir, s.Section41.LatencyP95MS, thresholdLatencyP95MS)
	fmt.Fprintf(&b, "- arm_a_identical across runs: %t\n\n", s.ArmAIdentical)
	fmt.Fprintf(&b, "## %s\n\n", PostHocHeading)
	fmt.Fprintf(&b, "Post-hoc full-context score (run %s):\n\n", s.PostHoc.RunDir)
	b.WriteString("| subset | mean full-context recall (arm B) |\n| --- | --- |\n")
	for _, sub := range Subsets {
		fmt.Fprintf(&b, "| %s | %s |\n", sub, fmtMeanOr(s.PostHoc.MeanFullContext, sub))
	}
	fmt.Fprintf(&b, "\n- mean distinct documents in arm B context: %.4f\n", s.PostHoc.MeanDistinctDocs)
	fmt.Fprintf(&b, "- historical improved vs arm A recall@5: %d\n", s.PostHoc.HistoricalImproved)
	fmt.Fprintf(&b, "- historical regressed vs arm A recall@5: %d\n\n", s.PostHoc.HistoricalRegressed)
	fmt.Fprintf(&b, "## %s\n\n", PostHocHeading)
	fmt.Fprintf(&b, "Controls (run %s, top-k 10), mean RecallAtK(..., 10):\n\n", s.Control.RunDir)
	b.WriteString("| subset | A | D |\n| --- | --- |\n")
	for _, sub := range Subsets {
		arms := s.Control.MeanRecallAt10[sub]
		fmt.Fprintf(&b, "| %s | %s | %s |\n", sub, fmtMeanOr(arms, ArmBM25), fmtMeanOr(arms, ArmFull))
	}
	b.WriteString("\n")
	return b.String()
}

// fmtMean renders a mean, or — when absent.
func fmtMean(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return strconv.FormatFloat(v, 'f', 4, 64)
}

// fmtMeanOr renders the mean for one arm of a subset->arm table, or — when the
// arm has no scored rows in that subset.
func fmtMeanOr(table map[string]float64, arm string) string {
	v, ok := table[arm]
	return fmtMean(v, ok)
}
