package rageval

// Four-arm comparison runner.
//
// For each question the runner evaluates four arms:
//   - Arm A: BM25-only over the indexed corpus, through internal/repoindex and
//     internal/retrieval. This is the same ranking SOP uses for documents.
//   - Arm B: BM25 plus RAG. The arm A hits are merged with the RAG evidence via
//     the SOP context merge path, so it measures what SOP actually does end to
//     end.
//   - Arm C: RAG-only, diagnostic. The RAG evidence only, no lexical fusion.
//   - Arm D: full-text retrieval over internal/retrieval, diagnostic. Its rows
//     never feed a threshold, the hard-blocker check, or the run status.
//
// Row fields that the RAG path supplies are documented on Row. If a provider
// does not supply a field the runner marks it explicitly as unavailable instead
// of inventing a value.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// Arm labels. The order of Arms is the report order.
const (
	ArmBM25   = "A"
	ArmHybrid = "B"
	ArmRAG    = "C"
	ArmFull   = "D"
)

// Arms lists the arm labels in a stable order.
var Arms = []string{ArmBM25, ArmHybrid, ArmRAG, ArmFull}

// Status is the overall run status.
type Status string

const (
	// StatusPass means no hard-blocker failure occurred.
	StatusPass Status = "pass"
	// StatusHardBlocker means a cross-corpus item appeared in some arm, or
	// another acceptance condition failed hard.
	StatusHardBlocker Status = "hard-blocker"
)

// Unavailable marks a row field the RAG path did not supply.
const Unavailable = "unavailable"

// RAGProvider returns the RAG evidence for one query. The frozen-fixture
// provider and the real ragclient-backed provider both implement it.
type RAGProvider interface {
	// Retrieve returns evidence for query scoped to corpus.
	Retrieve(ctx context.Context, corpus, query string, topK int) (RAGResult, error)
}

// RAGResult is the runner's view of one RAG call.
//
// ByteLength, Truncated, FallbackReason, and Latency come from the RAG/context
// merge path. A field the path does not report keeps its zero value and the row
// records Unavailable for it.
type RAGResult struct {
	// Evidence is the retrieved evidence in rank order.
	Evidence []Retrieved
	// RawEvidence is the ragclient evidence the provider returned. Arm B builds
	// its knowledge items from it through sopctx.KnowledgeItems.
	RawEvidence []ragclient.Evidence
	// AssembledContext is the context the merge path assembled, used to test
	// whether injected text reached the context.
	AssembledContext string
	// ContextBytes is the size of the assembled context, when known.
	ContextBytes int
	// ContextBytesKnown reports whether ContextBytes was supplied.
	ContextBytesKnown bool
	// Truncated reports whether the merge path truncated the context.
	Truncated bool
	// Latency is the wall time of the RAG call.
	Latency time.Duration
	// FallbackReason is why the RAG path fell back to lexical-only, when it did.
	FallbackReason string
}

// RagClientProvider adapts a *ragclient.Client to RAGProvider. Arm B and arm C
// both use it, so they measure the real SOP retrieval path.
type RagClientProvider struct {
	Client *ragclient.Client
}

// Retrieve implements RAGProvider through ragclient.
func (p RagClientProvider) Retrieve(ctx context.Context, corpus, query string, topK int) (RAGResult, error) {
	start := time.Now()
	res, err := p.Client.Retrieve(ctx, ragclient.Request{
		Query:   query,
		TopK:    topK,
		Filters: ragclient.Filters{Corpus: corpus},
	})
	latency := time.Since(start)
	out := RAGResult{Latency: latency, Truncated: res.Truncated, RawEvidence: res.Evidence}
	for i, ev := range res.Evidence {
		rank := ev.Rank
		if rank <= 0 {
			rank = i + 1
		}
		out.Evidence = append(out.Evidence, Retrieved{ID: evidenceID(ev), Rank: rank, Corpus: ev.Corpus})
	}
	var b strings.Builder
	for _, ev := range res.Evidence {
		b.WriteString(ev.Text)
		b.WriteString("\n")
	}
	out.AssembledContext = b.String()
	out.ContextBytes = len(out.AssembledContext)
	out.ContextBytesKnown = true
	if err != nil {
		out.FallbackReason = string(ragclient.CodeOf(err))
	}
	return out, err
}

// evidenceID builds a stable identity for one evidence item. The evidence
// identity is the corpus-relative source path, which matches
// retrieval.Result.Path and the question source in questions.json. A section
// suffix is appended only when present, and expectedIDs applies the same rule.
func evidenceID(ev ragclient.Evidence) string {
	if ev.Source == "" {
		return ev.ID
	}
	id := ev.Source
	if ev.Section != "" {
		id += "#" + ev.Section
	}
	if ev.Page > 0 {
		id += ":p" + fmt.Sprintf("%d", ev.Page)
	}
	return id
}

// FrozenProvider replays fixed RAG results. It is the test provider: it makes
// two runs over the same inputs produce identical rows.
type FrozenProvider struct {
	// Results maps a query to its frozen result.
	Results map[string]RAGResult
}

// Retrieve implements RAGProvider from frozen data.
func (p FrozenProvider) Retrieve(_ context.Context, _, query string, _ int) (RAGResult, error) {
	return p.Results[query], nil
}

// Config is the effective runner configuration, recorded in the report header.
type Config struct {
	// TopK is the number of results each arm requests.
	TopK int
	// Corpus is the corpus allowlist the run is scoped to.
	Corpus string
	// RAGContextBudgetBytes bounds the RAG knowledge items arm B includes.
	RAGContextBudgetBytes int
}

// Row is one question evaluated by one arm.
type Row struct {
	QuestionID string   `json:"question_id"`
	Category   Category `json:"category"`
	Arm        string   `json:"arm"`

	// Retrieved lists identities and ranks, best rank first.
	Retrieved []Retrieved `json:"retrieved"`
	// RecallAt5 is a pointer so a no_answer row can carry no score at all.
	RecallAt5 *float64 `json:"recall_at_5,omitempty"`
	// PrecisionAt5 is a pointer for the same reason.
	PrecisionAt5 *float64 `json:"precision_at_5,omitempty"`
	// CitationCorrect and ProvenanceCorrect are pointers so an unscored row can
	// omit them.
	CitationCorrect   *bool `json:"citation_correct,omitempty"`
	ProvenanceCorrect *bool `json:"provenance_correct,omitempty"`

	// ContextBytes records the assembled context size, or Unavailable.
	ContextBytes string `json:"context_bytes"`
	// Truncated reports whether the context was truncated.
	Truncated bool `json:"truncated"`
	// LatencyMS is the RAG latency in milliseconds, or Unavailable.
	LatencyMS string `json:"latency_ms"`
	// FallbackReason records why RAG fell back, or Unavailable.
	FallbackReason string `json:"fallback_reason"`

	// CrossCorpus lists identities that fell outside the permitted corpus.
	// A non-empty list flags the row and makes the run a hard-blocker failure.
	CrossCorpus []string `json:"cross_corpus,omitempty"`

	// Returned records what the arm returned for a no_answer question instead
	// of scoring recall.
	Returned []string `json:"returned,omitempty"`

	// InjectionReached reports whether injected text reached the assembled
	// context. InjectionBelowTask reports whether the injected text stayed
	// below the task and policy content.
	InjectionReached   *bool `json:"injection_reached,omitempty"`
	InjectionBelowTask *bool `json:"injection_below_task,omitempty"`
}

// RunResult is the whole run: header data plus every row.
type RunResult struct {
	Status       Status
	Rows         []Row
	HardBlockers []string
}

// Runner executes the four arms over one dataset. IndexA is built by
// BuildArmAIndex; IndexD is built by BuildArmDIndex when Docs is set.
type Runner struct {
	Dataset  *Dataset
	Docs     []CorpusDoc
	IndexA   *retrieval.Index
	IndexD   *retrieval.Index
	Provider RAGProvider
	Config   Config
}

// Run evaluates every question in dataset order, arm by arm, and returns the
// rows in that stable order. Any cross-corpus item in arm A, B, or C sets the
// run status to StatusHardBlocker. Arm D rows are diagnostic and never change
// the status. A knowledge-rag citation that does not split back into its
// evidence's own Corpus and Source makes Run return an error naming the
// citation.
//
// Runner.IndexA is a required input built by repoindex.Build through
// BuildArmAIndex. Run has no fallback that builds candidates by hand, so arm A
// always measures exactly what SOP retrieves.
func (r *Runner) Run(ctx context.Context) (RunResult, error) {
	if r.Dataset == nil {
		return RunResult{}, fmt.Errorf("dataset is required: Runner.Dataset must not be nil")
	}
	if r.Provider == nil {
		return RunResult{}, fmt.Errorf("provider is required: Runner.Provider must not be nil")
	}
	if r.IndexA == nil {
		res := RunResult{Status: StatusHardBlocker}
		res.HardBlockers = append(res.HardBlockers, "arm A index is required: build it with repoindex.Build through BuildArmAIndex")
		return res, fmt.Errorf("arm A index is required: build it with repoindex.Build through BuildArmAIndex")
	}
	if r.IndexD == nil {
		r.IndexD = BuildArmDIndex(r.Docs)
	}
	res := RunResult{Status: StatusPass}
	for _, q := range r.Dataset.Questions {
		for _, arm := range Arms {
			row, err := r.evaluate(ctx, q, arm)
			if err != nil {
				res.Status = StatusHardBlocker
				return res, fmt.Errorf("%s/%s: %w", q.ID, arm, err)
			}
			if arm != ArmFull {
				if len(row.CrossCorpus) > 0 {
					res.Status = StatusHardBlocker
					res.HardBlockers = append(res.HardBlockers, fmt.Sprintf("%s/%s: cross-corpus %v", row.QuestionID, row.Arm, row.CrossCorpus))
				}
				if row.InjectionBelowTask != nil && !*row.InjectionBelowTask {
					res.Status = StatusHardBlocker
					res.HardBlockers = append(res.HardBlockers, fmt.Sprintf("%s/%s: injected text above task or policy content", row.QuestionID, row.Arm))
				}
			}
			res.Rows = append(res.Rows, row)
		}
	}
	return res, nil
}

func (r *Runner) evaluate(ctx context.Context, q Question, arm string) (Row, error) {
	row := Row{
		QuestionID:     q.ID,
		Category:       q.Category,
		Arm:            arm,
		ContextBytes:   Unavailable,
		LatencyMS:      Unavailable,
		FallbackReason: Unavailable,
	}
	terms := retrieval.Tokenize(q.Question)
	var lex []Retrieved
	if r.IndexA != nil {
		lex = searchResults(r.IndexA.Search(retrieval.Query{Terms: terms}, r.Config.TopK), q.PermittedCorpus)
	}
	var rag RAGResult
	if arm == ArmHybrid || arm == ArmRAG {
		var err error
		if r.Provider != nil {
			rag, err = r.Provider.Retrieve(ctx, q.PermittedCorpus, q.Question, r.Config.TopK)
		}
		if err != nil && rag.FallbackReason == "" {
			rag.FallbackReason = err.Error()
		}
	}
	var assembled string
	switch arm {
	case ArmBM25:
		row.Retrieved = lex
	case ArmHybrid:
		ctxB, err := r.buildArmBContext(q, lex, rag)
		if err != nil {
			return row, err
		}
		ids, err := identitiesFromContext(ctxB, q.PermittedCorpus)
		if err != nil {
			return row, err
		}
		row.Retrieved = ids
		row.ContextBytes = fmt.Sprintf("%d", ctxB.Bytes())
		row.Truncated = ctxB.Truncated()
		assembled = ctxB.Render()
	case ArmRAG:
		row.Retrieved = ragOnly(rag.Evidence, r.Config.TopK)
		assembled = rag.AssembledContext
	case ArmFull:
		if r.IndexD != nil {
			row.Retrieved = searchResults(r.IndexD.Search(retrieval.Query{Terms: terms}, r.Config.TopK), q.PermittedCorpus)
		}
	}
	if arm != ArmBM25 && arm != ArmFull {
		if arm == ArmRAG && rag.ContextBytesKnown {
			row.ContextBytes = fmt.Sprintf("%d", rag.ContextBytes)
		}
		if arm == ArmRAG {
			row.Truncated = rag.Truncated
		}
		// LatencyMS records the RAG call wall time when the provider supplied
		// one. A zero latency (a frozen provider) records Unavailable so two
		// runs with the same inputs still produce identical rows.
		if rag.Latency > 0 {
			row.LatencyMS = fmt.Sprintf("%d", rag.Latency.Milliseconds())
		}
		if rag.FallbackReason != "" {
			row.FallbackReason = rag.FallbackReason
		}
	}
	row.CrossCorpus = CrossCorpus(q.PermittedCorpus, row.Retrieved)

	if q.Category == CategoryNoAnswer {
		row.Returned = idsOf(row.Retrieved)
		return row, nil
	}
	expected := expectedIDs(q)
	if v, ok := RecallAt5(expected, row.Retrieved); ok {
		row.RecallAt5 = &v
	}
	if v, ok := PrecisionAt5(expected, row.Retrieved); ok {
		row.PrecisionAt5 = &v
	}
	if v, ok := CitationCorrect(expected, row.Retrieved); ok {
		row.CitationCorrect = &v
	}
	if v, ok := ProvenanceCorrect(q.PermittedCorpus, row.Retrieved); ok {
		row.ProvenanceCorrect = &v
	}
	if q.Category == CategoryInjection {
		reached, below, applicable := injectionPlacement(q, assembled, arm)
		row.InjectionReached = &reached
		if applicable {
			row.InjectionBelowTask = &below
		}
	}
	return row, nil
}

// buildArmBContext assembles arm B's context through the SOP context package,
// exactly as internal/cli/run.go does. It builds one task item for the
// question, one repository item per arm A hit, and knowledge-rag items from
// sopctx.KnowledgeItems over the RAG evidence. sopctx.Build orders and bounds
// the items; no hand-written rank merge happens here. Every knowledge-rag
// citation must split back into the Corpus and Source of the evidence it was
// rendered from; otherwise the run fails with an error naming the citation.
func (r *Runner) buildArmBContext(q Question, lex []Retrieved, rag RAGResult) (sopctx.Context, error) {
	items := []sopctx.Item{{
		Source:   sopctx.SourceTask,
		Identity: q.ID,
		Reason:   "evaluation question",
		Priority: sopctx.PriorityLifecycle,
		Text:     q.Question,
	}}
	for _, hit := range stableByRank(lex) {
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceRepository,
			Identity: hit.ID,
			Reason:   "bm25 hit",
			Priority: sopctx.PriorityRepository,
			Text:     hit.ID,
		})
	}
	budget := r.Config.RAGContextBudgetBytes
	items = append(items, sopctx.KnowledgeItems(rag.RawEvidence, r.Config.TopK, budget)...)
	ctxB := sopctx.Build(items, sopctx.DefaultLimits())
	for _, it := range ctxB.Items() {
		if it.Source != sopctx.SourceKnowledgeRAG {
			continue
		}
		corpus, source, err := splitCitation(it.Identity)
		if err != nil {
			return ctxB, err
		}
		matched := false
		for _, ev := range rag.RawEvidence {
			if ev.Corpus == corpus && ev.Source == source {
				matched = true
				break
			}
		}
		if !matched {
			return ctxB, fmt.Errorf("knowledge-rag citation %q does not split back into its evidence corpus/source %q/%q", it.Identity, corpus, source)
		}
	}
	return ctxB, nil
}

// identitiesFromContext returns the ranked identities of a SOP context: the
// Items() order, keeping only repository and knowledge-rag items. A repository
// item's identity is its Identity, and it belongs to the permitted corpus. A
// knowledge-rag item's identity is the evidence Source; sopctx.KnowledgeItems
// renders that identity as corpus/source[#section][:p<page>], so the corpus and
// source are recovered from it and the section and page suffixes are dropped.
func identitiesFromContext(c sopctx.Context, permitted string) ([]Retrieved, error) {
	var out []Retrieved
	for _, it := range c.Items() {
		var id, corpus string
		switch it.Source {
		case sopctx.SourceRepository:
			id = it.Identity
			corpus = permitted
		case sopctx.SourceKnowledgeRAG:
			var err error
			corpus, id, err = splitCitation(it.Identity)
			if err != nil {
				return nil, err
			}
		default:
			continue
		}
		out = append(out, Retrieved{ID: id, Rank: len(out) + 1, Corpus: corpus})
	}
	return out, nil
}

// splitCitation breaks a sopctx knowledge citation of the form
// corpus/source[#section][:p<page>] into its corpus and source parts. The source
// part is the evidence identity: it drops any "#…" section suffix and any ":p…"
// page suffix, so it equals the evidence Source. A citation without a corpus
// separator or with an empty corpus or source is invalid and returns an error
// naming the citation.
func splitCitation(citation string) (corpus, source string, err error) {
	slash := strings.Index(citation, "/")
	if slash < 0 {
		return "", "", fmt.Errorf("invalid knowledge-rag citation %q: missing corpus separator", citation)
	}
	corpus, source = citation[:slash], citation[slash+1:]
	if hash := strings.Index(source, "#"); hash >= 0 {
		source = source[:hash]
	}
	if page := strings.Index(source, ":p"); page >= 0 {
		source = source[:page]
	}
	if corpus == "" || source == "" {
		return "", "", fmt.Errorf("invalid knowledge-rag citation %q: empty corpus or source", citation)
	}
	return corpus, source, nil
}

// ragOnly returns the RAG evidence in rank order without any lexical fusion.
// Arm C is diagnostic: it shows what the RAG path alone returns.
func ragOnly(evidence []Retrieved, k int) []Retrieved {
	ordered := stableByRank(evidence)
	seen := map[string]bool{}
	out := make([]Retrieved, 0, len(ordered))
	for _, it := range ordered {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		if k > 0 && len(out) >= k {
			break
		}
		out = append(out, Retrieved{ID: it.ID, Corpus: it.Corpus, Rank: len(out) + 1})
	}
	return out
}

// expectedIDs turns the labelled expected references into document-level
// retrieval identities: each expected Source once, without any "#section"
// suffix. Retrieval scoring compares documents, not sections, so the section
// of an expectation never forms part of the expected identity. The order is
// stable and duplicates are dropped.
func expectedIDs(q Question) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(q.Expected))
	for _, e := range q.Expected {
		if seen[e.Source] {
			continue
		}
		seen[e.Source] = true
		ids = append(ids, e.Source)
	}
	return ids
}

func idsOf(items []Retrieved) []string {
	out := make([]string, 0, len(items))
	for _, it := range stableByRank(items) {
		out = append(out, it.ID)
	}
	return out
}

// injectionPlacement reports whether the injected text reached the assembled
// context and whether it stayed below task and policy content. Only arm B
// assembles a SOP context, so only arm B evaluates placement; the result's
// applicable flag is true only for arm B. Arms A, C and D never raise the
// placement hard-blocker: they report applicable=false, so the runner records
// placement as not applicable. They still report reached when the arm's
// evidence carries the injected phrase.
//
// The needle is the exact injected text, never the expected-absent terms.
//
// When the injected text reaches the context, placement is "below task" only if
// the task item's text (the question) also appears in the rendered context and
// precedes the injected text. If the task text is absent, placement cannot be
// confirmed and the result is below=false, so the run flags it rather than
// certifying an unchecked placement.
func injectionPlacement(q Question, assembled, arm string) (reached, below, applicable bool) {
	if arm != ArmHybrid {
		// Only arm B evaluates placement. Arms A, C and D still record
		// whether the injected phrase was retrieved, with placement not
		// applicable and no hard-blocker.
		needle := q.InjectedText
		if needle == "" || assembled == "" {
			return false, true, false
		}
		return strings.Contains(assembled, needle), true, false
	}
	if assembled == "" {
		return false, true, true
	}
	needle := q.InjectedText
	if needle == "" {
		return false, true, true
	}
	idx := strings.Index(assembled, needle)
	if idx < 0 {
		return false, true, true
	}
	// Placement is anchored on the task item's text: the question. Task
	// content must come first. If the question text is absent from the
	// rendered context, placement cannot be confirmed, so the result is
	// below=false and the run flags it.
	taskIdx := strings.Index(assembled, q.Question)
	if taskIdx < 0 {
		return true, false, true
	}
	if idx < taskIdx {
		return true, false, true
	}
	return true, true, true
}

// ManifestHash returns a stable sha256 over the dataset manifest, for the report
// header. It sorts entries by path so the hash does not depend on manifest line
// order.
func ManifestHash(entries []ManifestEntry) string {
	sorted := make([]ManifestEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, e := range sorted {
		fmt.Fprintf(h, "%s  %s\n", e.SHA256, e.Path)
	}
	return hex.EncodeToString(h.Sum(nil))
}
