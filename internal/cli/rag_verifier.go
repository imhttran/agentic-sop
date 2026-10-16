package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/ragclient"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// attemptRetrieval is the attempt-scoped retrieval result (RAG-SOP-003 Phase 4B-2A2a).
// It carries raw, validated evidence plus status; document bodies are never
// persisted. It is returned explicitly, never stored on a copied deps value.
type attemptRetrieval struct {
	Attempted bool
	Evidence  []ragclient.Evidence
	Err       error
	Corpus    string
}

// rawEvidenceFunc is the injectable RAW-evidence retrieval seam. It is distinct
// from ragKnowledgeFunc (which returns ready context ITEMS): this returns the raw
// []ragclient.Evidence to be validated against the required-evidence contract.
type rawEvidenceFunc func(ctx context.Context, spec *taskfile.Spec, corpus string) ([]ragclient.Evidence, error)

// retrieveAttemptEvidence performs at most ONE retrieval for one task attempt and
// returns it explicitly (no mutation of copied deps). Disabled RAG retrieves
// nothing. Corpus authorization, timeout, top-k, and cancellation are enforced by
// the injected function or the production ragclient.
func retrieveAttemptEvidence(ctx context.Context, cfg config.Config, spec *taskfile.Spec, d deps) attemptRetrieval {
	resolved, err := cfg.RAG.Resolved(nil)
	if err != nil || !resolved.Enabled {
		return attemptRetrieval{}
	}
	corpus := resolved.Corpus
	if d.rawEvidence != nil {
		evidence, err := d.rawEvidence(ctx, spec, corpus)
		return attemptRetrieval{Attempted: true, Evidence: evidence, Err: err, Corpus: corpus}
	}
	client, err := ragclient.New(ragclient.Config{
		Executable:     resolved.Executable,
		AllowedCorpora: resolved.AllowedCorpora,
		Timeout:        resolved.Timeout(),
		MaxResults:     resolved.TopK,
	})
	if err != nil {
		return attemptRetrieval{Attempted: true, Err: err, Corpus: corpus}
	}
	res, err := client.Retrieve(ctx, ragclient.Request{
		Query:   ragQueryFromSpec(spec),
		Filters: ragclient.Filters{Corpus: corpus},
	})
	return attemptRetrieval{Attempted: true, Evidence: res.Evidence, Err: err, Corpus: corpus}
}

// ragKnowledgeFunc is the injectable optional-RAG provider seam (RAG-SOP-003).
// It returns ready context items and is nil in production, where the configured
// ragclient is used instead. It exists so tests can supply deterministic evidence
// without starting the real producer.
type ragKnowledgeFunc func(ctx context.Context, spec *taskfile.Spec) ([]sopctx.Item, error)

// retrievalClient is the slice of ragclient.Client the verifier needs. It is an
// interface so tests can inject a deterministic double; the production value is
// a *ragclient.Client.
type retrievalClient interface {
	Retrieve(ctx context.Context, req ragclient.Request) (ragclient.Response, error)
}

// ragEvidenceVerifier verifies a task's required retrieval evidence against the
// configured single corpus through the retrieval-only client (RAG-SOP-003). It
// never accepts a model assertion, never broadens the corpus, and never falls
// back: every declared requirement must be satisfied (AND).
type ragEvidenceVerifier struct {
	client retrievalClient
	corpus string
}

// newRAGEvidenceVerifier resolves the RAG configuration and builds the verifier.
// It returns (nil, nil) when RAG is disabled, so a task that requires evidence
// fails closed through the existing nil-verifier path. An invalid *enabled*
// configuration is an actionable error.
func newRAGEvidenceVerifier(cfg config.Config) (EvidenceVerifier, error) {
	resolved, err := cfg.RAG.Resolved(nil)
	if err != nil {
		return nil, err
	}
	if !resolved.Enabled {
		return nil, nil
	}
	client, err := ragclient.New(ragclient.Config{
		Executable:     resolved.Executable,
		AllowedCorpora: resolved.AllowedCorpora,
		Timeout:        resolved.Timeout(),
		MaxResults:     resolved.TopK,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: %w", err)
	}
	return &ragEvidenceVerifier{client: client, corpus: strings.TrimSpace(resolved.Corpus)}, nil
}

// VerifyRequiredEvidence implements EvidenceVerifier. An empty RequiredEvidence
// is a no-op. Any retrieval failure or unsatisfied identity returns the exact
// missing identities and a structured error, which the caller maps to BLOCKED.
func (v *ragEvidenceVerifier) VerifyRequiredEvidence(ctx context.Context, task *domain.Task) ([]string, error) {
	if task == nil || len(task.RequiredEvidence) == 0 {
		return nil, nil
	}

	res, err := v.client.Retrieve(ctx, ragclient.Request{
		Query:   ragQuery(task),
		Filters: ragclient.Filters{Corpus: v.corpus},
	})
	if err != nil {
		// The client already enforced the deadline, result-count and
		// response-size limits and returned a typed failure.
		return append([]string{}, task.RequiredEvidence...), err
	}

	return validateRequiredEvidence(task.RequiredEvidence, v.corpus, res.Evidence)
}

// validateRequiredEvidence checks every declared requirement against the trusted
// corpus and the retrieved evidence, WITHOUT performing any retrieval. It returns
// the missing/unauthorized identities (sorted) and a structured error. Corpus
// authorization, source-prefix matching, AND semantics, and behavior on empty or
// malformed evidence are preserved exactly.
func validateRequiredEvidence(required []string, corpus string, evidence []ragclient.Evidence) ([]string, error) {
	missing := make([]string, 0, len(required))
	for _, req := range required {
		kind, id, _ := strings.Cut(req, ":")
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "corpus":
			// The single configured corpus is the only authorized one; a request for
			// another corpus fails closed rather than broadening the search.
			if id != corpus || !hasCorpus(evidence, id) {
				missing = append(missing, req)
			}
		case "source":
			if !hasSourcePrefix(evidence, id) {
				missing = append(missing, req)
			}
		default:
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return missing, fmt.Errorf("required evidence not satisfied: %s", strings.Join(missing, ", "))
	}
	return nil, nil
}

// ragQuery derives a deterministic retrieval query from task metadata (title and
// objective). It consults no model.
func ragQuery(task *domain.Task) string {
	terms := strings.Fields(strings.TrimSpace(task.Title + " " + task.Objective))
	if len(terms) == 0 {
		return task.ID
	}
	q := strings.Join(terms, " ")
	if len(q) > 512 {
		q = q[:512]
	}
	return q
}

// hasCorpus reports whether the response carries usable evidence from corpus.
func hasCorpus(evidence []ragclient.Evidence, corpus string) bool {
	for _, e := range evidence {
		if e.Corpus == corpus && e.Source != "" && e.Text != "" {
			return true
		}
	}
	return false
}

// hasSourcePrefix reports whether the response carries usable evidence whose
// source begins with prefix.
func hasSourcePrefix(evidence []ragclient.Evidence, prefix string) bool {
	for _, e := range evidence {
		if e.Source != "" && e.Text != "" && strings.HasPrefix(e.Source, prefix) {
			return true
		}
	}
	return false
}

// optionalRAGKnowledge returns optional RAG context items for a task that declares
// NO RequiredEvidence (RAG-SOP-003 Phase 4B-1B). Disabled RAG, or any retrieval
// failure, yields nil so the context and the compiled prompt stay byte-identical
// (BM25-only). It performs at most one retrieval, never broadens the corpus, and
// bounds the result by the configured timeouts, top-k and byte budget. Retrieved
// text is untrusted evidence at priority 6.
func optionalRAGKnowledge(ctx context.Context, cfg config.Config, spec *taskfile.Spec, d deps) []sopctx.Item {
	if spec == nil || len(spec.RequiredEvidence) > 0 {
		return nil
	}
	resolved, err := cfg.RAG.Resolved(nil)
	if err != nil || !resolved.Enabled {
		return nil
	}
	// Injectable seam: a test function is used verbatim; production builds the
	// configured client. Disabled RAG and required-evidence tasks never reach here.
	if d.ragKnowledge != nil {
		items, err := d.ragKnowledge(ctx, spec)
		if err != nil {
			return nil
		}
		return items
	}
	client, err := ragclient.New(ragclient.Config{
		Executable:     resolved.Executable,
		AllowedCorpora: resolved.AllowedCorpora,
		Timeout:        resolved.Timeout(),
		MaxResults:     resolved.TopK,
	})
	if err != nil {
		return nil
	}
	res, err := client.Retrieve(ctx, ragclient.Request{
		Query:   ragQueryFromSpec(spec),
		Filters: ragclient.Filters{Corpus: resolved.Corpus},
	})
	if err != nil {
		return nil
	}
	return sopctx.KnowledgeItems(res.Evidence, resolved.TopK, resolved.ContextBudgetBytes)
}

// ragQueryFromSpec derives a deterministic query from task metadata; no model.
func ragQueryFromSpec(spec *taskfile.Spec) string {
	terms := strings.Fields(strings.TrimSpace(spec.Title + " " + spec.Description))
	if len(terms) == 0 {
		return spec.ID
	}
	q := strings.Join(terms, " ")
	if len(q) > 512 {
		q = q[:512]
	}
	return q
}

// enforceRequiredEvidenceOnce blocks a task whose declared required evidence cannot
// be retrieved and validated, using ONE attempt-scoped retrieval (RAG-SOP-003
// 4B-2A2b). It fails closed on disabled RAG and on invalid enabled configuration, and
// performs no retrieval when the task declares no required evidence. On failure the
// exact missing/unauthorized identities are persisted in BlockedDetail and no agent
// runs. It never falls back to optional RAG or to BM25 to satisfy a requirement.
func enforceRequiredEvidenceOnce(ctx context.Context, cfg config.Config, saver taskSaver, task *domain.Task, spec *taskfile.Spec, d deps, stdout, stderr io.Writer) (bool, int, attemptRetrieval) {
	if task == nil || len(task.RequiredEvidence) == 0 {
		return false, 0, attemptRetrieval{}
	}
	resolved, err := cfg.RAG.Resolved(nil)
	if err != nil {
		// Invalid *enabled* configuration is an actionable, fail-closed failure; it is
		// never treated as a successful retrieval or valid evidence.
		b, c := failRequiredEvidence(saver, task, []string{err.Error()}, stdout, stderr)
		return b, c, attemptRetrieval{}
	}
	if !resolved.Enabled {
		// RAG disabled: required evidence fails closed with no retrieval.
		b, c := failRequiredEvidence(saver, task, task.RequiredEvidence, stdout, stderr)
		return b, c, attemptRetrieval{}
	}
	attempt := retrieveAttemptEvidence(ctx, cfg, spec, d)
	if attempt.Err != nil {
		b, c := failRequiredEvidence(saver, task, task.RequiredEvidence, stdout, stderr)
		return b, c, attemptRetrieval{}
	}
	missing, verr := validateRequiredEvidence(task.RequiredEvidence, attempt.Corpus, attempt.Evidence)
	if verr != nil {
		if len(missing) == 0 {
			missing = task.RequiredEvidence
		}
		b, c := failRequiredEvidence(saver, task, missing, stdout, stderr)
		return b, c, attemptRetrieval{}
	}
	return false, 0, attempt
}

// failRequiredEvidence records the deterministic BLOCKED outcome: the exact missing
// identities in the durable BlockedDetail, the existing REQUIRED_EVIDENCE_MISSING
// reason, and a content-free diagnostic. It returns the stop the caller must honour.
func failRequiredEvidence(saver taskSaver, task *domain.Task, missing []string, stdout, stderr io.Writer) (bool, int) {
	detail := strings.Join(missing, ", ")
	task.BlockedDetail = detail
	if err := blockTask(saver, task, domain.REQUIRED_EVIDENCE_MISSING); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return true, exitError
	}
	fmt.Fprintf(stdout, "%s BLOCKED (%s): required evidence unavailable: %s [reason_code=%s]\n", task.ID, domain.REQUIRED_EVIDENCE_MISSING, detail, ragReasonRequired)
	return true, exitError
}

// RAG telemetry (RAG-SOP-003 4B-2B2a): a content-free record of one attempt's
// retrieval. It carries statuses and bounded reason codes only — never document
// bodies, embeddings, credentials, raw prompts, or producer error text.
type ragTelemetry struct {
	Phase         string `json:"phase"`
	TaskID        string `json:"task_id"`
	Attempt       int    `json:"attempt"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	Corpus        string `json:"corpus,omitempty"`
	EvidenceCount int    `json:"evidence_count"`
	EvidenceBytes int    `json:"evidence_bytes"`
	UsedInContext bool   `json:"used_in_context"`
	// PromptIncluded is set only after the compiled prompt is inspected: true when
	// every selected knowledge citation is present, false when any is missing. It is
	// nil until then.
	PromptIncluded *bool  `json:"prompt_included,omitempty"`
	ReasonCode     string `json:"reason_code,omitempty"`
}

const (
	ragModeRequired   = "required"
	ragModeOptional   = "optional"
	ragStatusSucceed  = "succeeded"
	ragStatusFailed   = "failed"
	ragStatusSkipped  = "skipped"
	ragReasonNone     = ""
	ragReasonDisabled = "RAG_DISABLED"
	ragReasonFailure  = "RETRIEVAL_FAILED"
	ragReasonRequired = "REQUIRED_EVIDENCE_MISSING"
	ragReasonFallback = "OPTIONAL_FALLBACK"
)

// writeRAGTelemetry writes the bounded rag.json artifact beside the other run
// artifacts. It is a no-op when the run directory does not exist yet.
func writeRAGTelemetry(rn *runpkg.Run, rec ragTelemetry) {
	if rn == nil {
		return
	}
	rec.Phase = "rag"
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	_ = rn.Write("rag.json", string(append(b, '\n')))
}

// knowledgeCitations returns the stable citation identities of the selected
// knowledge items (their sopctx.Item.Identity). No new identifiers are invented.
func knowledgeCitations(items []sopctx.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Identity) != "" {
			out = append(out, it.Identity)
		}
	}
	return out
}

// updateRAGPromptInclusion refreshes rag.json with prompt_included after the prompt
// compiler has run: true only when at least one citation was selected and every
// selected citation is present in the compiled evidence (exact identity match, not
// a generic marker). It never writes prompt text or retrieved content.
func updateRAGPromptInclusion(rn *runpkg.Run, dir, taskID, evidence string, citations []string) {
	if len(citations) == 0 {
		return
	}
	path := filepath.Join(dir, ".agent-sdlc", "runs", taskID, "rag.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var rec ragTelemetry
	if json.Unmarshal(b, &rec) != nil {
		return
	}
	included := true
	for _, c := range citations {
		if !strings.Contains(evidence, c) {
			included = false
		}
	}
	rec.PromptIncluded = &included
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	if rn != nil {
		_ = rn.Write("rag.json", string(append(out, '\n')))
		return
	}
	_ = os.WriteFile(path, append(out, '\n'), 0o644)
}
