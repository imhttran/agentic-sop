package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The /sop-rag-integrate skill is a prompt artifact — a SKILL.md plus optional
// references — so its acceptance coverage is deterministic: these tests read the
// shipped skill tree and the fixtures under testdata/rag-integrate and assert the
// published contract. They never contact a provider, a model, or a database, and they
// deploy nothing.

const ragIntegrateDir = "../../skills/sop-rag-integrate"

// ragIntegrateCorpus returns the skill's prose — SKILL.md plus every reference —
// lowercased, so coverage assertions are case-insensitive and stable.
func ragIntegrateCorpus(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(ragIntegrateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", ragIntegrateDir, err)
	}
	return strings.ToLower(b.String())
}

// TestRAGIntegrateSkillStructure proves the skill is a well-formed, self-describing
// adapter: frontmatter that matches the folder, its canonical delegation, a fail-closed
// PATH instruction, and the references it advertises actually exist.
func TestRAGIntegrateSkillStructure(t *testing.T) {
	body := read(t, filepath.Join(ragIntegrateDir, "SKILL.md"))
	if !strings.HasPrefix(body, "---") {
		t.Error("SKILL.md must start with YAML frontmatter")
	}
	for _, want := range []string{
		"name: sop-rag-integrate",
		"description:",
		"sop prompt --capability plan",
		"PATH",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
	for _, ref := range []string{
		"discovery-and-suitability.md",
		"technology-selection.md",
		"evaluation.md",
	} {
		if _, err := os.Stat(filepath.Join(ragIntegrateDir, "references", ref)); err != nil {
			t.Errorf("advertised reference %s is missing: %v", ref, err)
		}
	}
}

// TestRAGIntegrateSkillAcceptanceCriteria proves the skill's mandatory contract: it
// discovers before proposing, reuses established libraries, allows and preserves a
// NO-RAG outcome, does not blindly pick rag-template, produces a scoped plan, keeps the
// SOP approval boundary, and requires reproducible evidence.
func TestRAGIntegrateSkillAcceptanceCriteria(t *testing.T) {
	corpus := ragIntegrateCorpus(t)
	cases := []struct {
		name  string
		wants []string
	}{
		{"discovers before proposing", []string{"discover", "before"}},
		{"reuses established libraries", []string{"existing", "librar"}},
		{"allows an explicit NO-RAG outcome", []string{"no-rag", "first-class outcome"}},
		{"does not blindly select rag-template", []string{"rag-template", "not a requirement"}},
		{"produces a scoped plan", []string{"scoped", "out of scope"}},
		{"preserves the approval boundary", []string{"approv"}},
		{"requires reproducible evidence", []string{"reproducib", "evidence"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range tc.wants {
				if !strings.Contains(corpus, want) {
					t.Errorf("skill corpus does not contain %q", want)
				}
			}
		})
	}
}

// TestRAGIntegrateSkillIsProviderNeutral proves the skill spans ecosystems rather than
// mandating one provider or framework, and states the provider-neutral principle.
func TestRAGIntegrateSkillIsProviderNeutral(t *testing.T) {
	corpus := ragIntegrateCorpus(t)
	markers := []string{
		"spring ai", "langchain4j", "llamaindex", "haystack",
		"postgresql + pgvector", "retrieval service", "javascript", "typescript",
	}
	found := 0
	var missing []string
	for _, m := range markers {
		if strings.Contains(corpus, m) {
			found++
		} else {
			missing = append(missing, m)
		}
	}
	if found < 4 {
		t.Errorf("the skill names only %d ecosystem markers (%v); it must span ecosystems", found, missing)
	}
	if !strings.Contains(corpus, "provider-neutral") {
		t.Error("the skill must state the provider-neutral principle")
	}
}

// TestRAGIntegratePreservesThePOCDecision proves the skill records the completed RAG
// proof of concept and its NO-GO decision instead of silently reversing it.
func TestRAGIntegratePreservesThePOCDecision(t *testing.T) {
	corpus := ragIntegrateCorpus(t)
	for _, want := range []string{"no-go", "rag-sop-004-decision.md", "proof of concept"} {
		if !strings.Contains(corpus, want) {
			t.Errorf("the skill must preserve the POC decision (missing %q)", want)
		}
	}
}

// ragScenario is one acceptance fixture: a target-repository brief plus the guidance
// the skill must publish to handle it.
type ragScenario struct {
	name     string
	expect   string
	compare  bool
	requires []string
}

// parseRAGScenario reads a fixture: `key: value` header lines, a `requires:` list of
// `- token` items, and a `---` separator before the free-form brief.
func parseRAGScenario(t *testing.T, path string) ragScenario {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := ragScenario{name: filepath.Base(path)}
	inRequires := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		switch {
		case strings.HasPrefix(trimmed, "scenario:"):
			s.name = strings.TrimSpace(strings.TrimPrefix(trimmed, "scenario:"))
			inRequires = false
		case strings.HasPrefix(trimmed, "expect:"):
			s.expect = strings.TrimSpace(strings.TrimPrefix(trimmed, "expect:"))
			inRequires = false
		case strings.HasPrefix(trimmed, "compare_existing:"):
			s.compare = strings.TrimSpace(strings.TrimPrefix(trimmed, "compare_existing:")) == "true"
			inRequires = false
		case trimmed == "requires:":
			inRequires = true
		case inRequires && strings.HasPrefix(trimmed, "- "):
			s.requires = append(s.requires, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	if s.name == "" || s.expect == "" || len(s.requires) == 0 {
		t.Fatalf("%s: incomplete scenario (name=%q expect=%q requires=%v)", path, s.name, s.expect, s.requires)
	}
	return s
}

// TestRAGIntegrateAcceptanceScenarios runs the representative target applications. Each
// fixture declares the decision it expects and the guidance the skill must publish to
// reach it; at least five scenarios are required, and at least one must be NO-RAG.
func TestRAGIntegrateAcceptanceScenarios(t *testing.T) {
	corpus := ragIntegrateCorpus(t)
	dir := filepath.Join("testdata", "rag-integrate")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read scenarios: %v", err)
	}
	var scenarios []ragScenario
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		scenarios = append(scenarios, parseRAGScenario(t, filepath.Join(dir, e.Name())))
	}
	if len(scenarios) < 5 {
		t.Fatalf("need at least 5 acceptance scenarios, found %d", len(scenarios))
	}
	noRAG := 0
	for _, s := range scenarios {
		s := s
		t.Run(s.name, func(t *testing.T) {
			switch s.expect {
			case "RAG":
			case "NO-RAG":
				noRAG++
			default:
				t.Fatalf("expect must be RAG or NO-RAG, got %q", s.expect)
			}
			for _, tok := range s.requires {
				if !strings.Contains(corpus, strings.ToLower(tok)) {
					t.Errorf("the skill does not publish the guidance %q that this scenario requires", tok)
				}
			}
			if s.compare && !strings.Contains(corpus, "compare") {
				t.Error("a scenario with an existing search requires the skill to insist on a comparison")
			}
		})
	}
	if noRAG == 0 {
		t.Error("at least one scenario must exercise the NO-RAG outcome")
	}
}
