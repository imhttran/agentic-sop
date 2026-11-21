package expharness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Attribution is the per-run evidence record. Each field says in its name or in
// Basis what kind of evidence it is; none is a provider attestation.
type Attribution struct {
	RunDir          string `json:"run_dir"` // absolute
	WorkItemID      string `json:"work_item_id"`
	RunDirMatchesID bool   `json:"run_dir_matches_work_item_id"`

	RequestedClass    string `json:"requested_class"`
	RequestedProvider string `json:"requested_provider"` // routing.json, i.e. what SOP selected
	RequestedModel    string `json:"requested_model"`
	EffectiveHarness  string `json:"effective_harness"` // overlay value, valid only if EnvOverrideState is clear
	EnvOverrideState  string `json:"env_override_state"`

	ModelsSentToProvider  []string               `json:"models_sent_to_provider"`     // proxy-observed
	ModelsReturned        []string               `json:"models_returned_by_provider"` // self-reported by the server
	ProviderPaths         []string               `json:"provider_paths"`              // protocol actually spoken
	ProviderRequests      int                    `json:"provider_requests"`           // forwarded, proxy-observed
	RefusedByCeiling      int                    `json:"refused_by_ceiling"`          //
	SuccessfulCalls       int64                  `json:"successful_calls"`            // tokens.calls
	ExtraProviderRequests int                    `json:"extra_provider_requests"`     // forwarded minus successful: retries or failures
	Tokens                map[string]perf.Tokens `json:"tokens"`                      // as SOP recorded
	ProxyInputTokens      int64                  `json:"proxy_input_tokens"`
	ProxyOutputTokens     int64                  `json:"proxy_output_tokens"`
	UnreportedCalls       int64                  `json:"unreported_calls"`

	ToolInvocations []toolharness.AuditRecord `json:"tool_invocations"`
	ToolEvidence    string                    `json:"tool_evidence"` // where they came from, or why absent

	ResultSHA256 string   `json:"result_sha256"`
	Basis        string   `json:"identity_basis"`
	Problems     []string `json:"problems"`
}

const identityBasis = "client-side name plus proxy-observed request; a server-returned model field is " +
	"self-reported, not attestation; provider-side substitution is not detectable"

// metadata is the part of a prompt run's metadata.json this record reads.
type metadata struct {
	WorkItemID string `json:"work_item_id"`
	Routing    struct {
		Class, Provider, Model string
	} `json:"routing"`
	Tokens map[string]perf.Tokens `json:"tokens"`
}

// Attribute assembles the attribution for one finished run. recs are the run's own
// proxy records; auditPath is the SOP_TOOL_AUDIT_LOG file set for that run ("" when
// none was configured). pf is the preflight of the project and environment the run
// used.
func Attribute(runDir string, pf Report, recs []Record, auditPath string) (Attribution, error) {
	a := Attribution{Basis: identityBasis, EffectiveHarness: pf.Harness}
	abs, err := filepath.Abs(runDir)
	if err != nil {
		return a, err
	}
	a.RunDir = abs
	raw, err := os.ReadFile(filepath.Join(abs, "metadata.json"))
	if err != nil {
		return a, fmt.Errorf("run metadata: %w", err)
	}
	var m metadata
	if err := json.Unmarshal(raw, &m); err != nil {
		return a, fmt.Errorf("run metadata: %w", err)
	}
	a.WorkItemID, a.RunDirMatchesID = m.WorkItemID, filepath.Base(abs) == m.WorkItemID
	a.RequestedClass, a.RequestedProvider, a.RequestedModel = m.Routing.Class, m.Routing.Provider, m.Routing.Model
	a.Tokens = m.Tokens
	if !a.RunDirMatchesID {
		a.Problems = append(a.Problems, "run directory name differs from work_item_id")
	}
	if pf.OK() {
		a.EnvOverrideState = "clear: preflight passed for this project and environment"
	} else {
		a.EnvOverrideState = "NOT CLEAR: " + strings.Join(pf.Problems, "; ")
		a.Problems = append(a.Problems, "preflight reported problems; model and provider attribution is invalid")
	}
	if res, err := os.ReadFile(filepath.Join(abs, "result.md")); err == nil {
		a.ResultSHA256 = sha(res)
	} else {
		a.Problems = append(a.Problems, "result.md missing")
	}

	sent, ret, paths := set{}, set{}, set{}
	for _, r := range recs {
		if r.Refused {
			a.RefusedByCeiling++
			continue
		}
		a.ProviderRequests++
		sent.add(r.ModelSent)
		ret.add(r.ModelReturned)
		paths.add(r.Path)
		a.ProxyInputTokens += r.InputTokens
		a.ProxyOutputTokens += r.OutputTokens
	}
	a.ModelsSentToProvider, a.ModelsReturned, a.ProviderPaths = sent.list(), ret.list(), paths.list()
	for k, t := range m.Tokens {
		a.SuccessfulCalls += t.Calls
		a.UnreportedCalls += t.UnreportedCalls
		if k != m.Routing.Model {
			a.Problems = append(a.Problems, fmt.Sprintf("tokens key %q is not routing.model %q", k, m.Routing.Model))
		}
	}
	a.ExtraProviderRequests = a.ProviderRequests - int(a.SuccessfulCalls)
	if len(a.ModelsSentToProvider) != 1 || a.ModelsSentToProvider[0] != m.Routing.Model {
		a.Problems = append(a.Problems, fmt.Sprintf("provider received models %v, routing.model is %q",
			a.ModelsSentToProvider, m.Routing.Model))
	}
	if a.UnreportedCalls > 0 || len(m.Tokens) == 0 {
		a.Problems = append(a.Problems, "token usage missing or unreported: model check and cost criterion INCONCLUSIVE")
	}
	a.ToolInvocations, a.ToolEvidence = readAudit(auditPath)
	sort.Strings(a.Problems)
	return a, nil
}

type set map[string]bool

func (s set) add(v string) {
	if v != "" {
		s[v] = true
	}
}
func (s set) list() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readAudit parses the JSONL tool audit file written through SOP_TOOL_AUDIT_LOG. An
// absent file with a configured path means no tool call was audited (the sink creates
// the file on the first record), which is distinct from "no sink configured".
func readAudit(path string) ([]toolharness.AuditRecord, string) {
	if path == "" {
		return nil, "unobserved: SOP_TOOL_AUDIT_LOG was not set for this run"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "sink configured, file absent: no tool call was audited"
	}
	defer f.Close()
	var out []toolharness.AuditRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r toolharness.AuditRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out, "SOP_TOOL_AUDIT_LOG (SOP-side audit of tool calls executed or denied)"
}
