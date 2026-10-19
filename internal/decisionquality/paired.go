package decisionquality

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// PairedRun computes the paired comparison of arm b against arm a.
//
// The unit of analysis is the CASE, never the repeat: three repeats of one case are
// one observation, not three. Each case therefore contributes the mean correctness
// of its evaluated repeats, and the bootstrap resamples cases. Repeated decisions
// from the same case are never treated as independent evidence.
func PairedRun(a, b RunRecord, seed int64) PairedResult {
	perCase := func(r RunRecord) map[string]float64 {
		sum := map[string]float64{}
		n := map[string]int{}
		for _, cr := range r.Results {
			if cr.NotEvaluable {
				continue
			}
			v := 0.0
			if cr.Correct {
				v = 1
			}
			sum[cr.CaseID] += v
			n[cr.CaseID]++
		}
		out := map[string]float64{}
		for id, s := range sum {
			out[id] = s / float64(n[id])
		}
		return out
	}
	am, bm := perCase(a), perCase(b)
	// A case must be evaluated in both arms to contribute a paired difference.
	var diffs []float64
	for id, av := range am {
		bv, ok := bm[id]
		if !ok {
			continue
		}
		diffs = append(diffs, (bv-av)*100)
	}
	sort.Float64s(diffs)
	pr := PairedResult{Base: a.Arm, Arm: b.Arm}
	if len(diffs) == 0 {
		return pr
	}
	var sum float64
	for _, d := range diffs {
		sum += d
	}
	pr.MeanDiffPP = sum / float64(len(diffs))
	pr.VariabilityPP = diffs[len(diffs)-1] - diffs[0]
	pr.CaseCount = len(diffs)
	pr.LowerPP, pr.UpperPP = bootstrapCI(diffs, seed)
	return pr
}

// bootstrapCI returns a deterministic 95% percentile bootstrap interval for the
// mean paired difference. The resampling unit is the case.
func bootstrapCI(diffs []float64, seed int64) (float64, float64) {
	const resamples = 2000
	rng := rand.New(rand.NewSource(seed))
	means := make([]float64, resamples)
	for i := 0; i < resamples; i++ {
		var s float64
		for j := 0; j < len(diffs); j++ {
			s += diffs[rng.Intn(len(diffs))]
		}
		means[i] = s / float64(len(diffs))
	}
	sort.Float64s(means)
	lo := means[int(math.Floor(0.025*float64(resamples-1)))]
	hi := means[int(math.Ceil(0.975*float64(resamples-1)))]
	return lo, hi
}

// InsufficientEvidence reports why a comparison cannot support a conclusion. It
// returns an empty string when the evidence is sufficient.
func InsufficientEvidence(corpus Corpus, a, b RunRecord) string {
	if len(corpus.Cases) == 0 {
		return "no cases"
	}
	evaluated := map[string]bool{}
	for _, cr := range b.Results {
		if !cr.NotEvaluable {
			evaluated[cr.CaseID] = true
		}
	}
	if len(evaluated) < len(corpus.Cases) {
		return fmt.Sprintf("arm %s evaluated %d of %d cases; not-evaluable cases are reported, not imputed", b.Arm, len(evaluated), len(corpus.Cases))
	}
	if len(a.Results) == 0 {
		return "arm A produced no results"
	}
	return ""
}
