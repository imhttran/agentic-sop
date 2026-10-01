// Package skill validates the shipped SOP agent skill (skills/sop) as an artifact.
//
// The skill is a thin client of the `sop` CLI: it must call `sop prompt` and must
// not reproduce SOP policy, invoke a provider directly, or instruct an agent to
// mutate the repository itself. These checks are structural (they read the shipped
// files), so a change to the skill that breaks the contract fails the build.
package skill

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillDir is the shipped skill, relative to this package (internal/skill).
const skillDir = "../../skills/sop"

// canonicalCapabilities is the operator-facing capability vocabulary the CLI
// accepts (internal/cli/prompt.go). Kept here so a new example using a non-canonical
// name fails the build.
var canonicalCapabilities = map[string]bool{
	"plan":             true,
	"design_tests":     true,
	"diagnose_failure": true,
	"review":           true,
	"implement":        true,
}

// providerBinaries are provider runtimes the skill must never invoke directly:
// SOP resolves the provider and model.
var providerBinaries = []string{"ollama", "llama-server", "llama.cpp", "llamacpp", "openrouter", "mlx_lm"}

// policyKnobs are SOP configuration settings that belong in the reference docs, not
// the skill: their presence would mean the skill is restating SOP policy.
var policyKnobs = []string{
	"SOP_MODEL_ROUTING_ENABLED",
	"SOP_MODEL_ESCALATION_ENABLED",
	"SOP_MODEL_MAX_ESCALATIONS",
	"max_fix_cycles",
	"fail_on",
}

// mutationCommands are direct-mutation instructions the skill must not contain: an
// IMPLEMENT request goes through `sop prompt --capability implement`, not the shell.
var mutationCommands = []string{"git commit", "git push", "git add", "sed -i", "rm -rf"}

var capabilityArgRE = regexp.MustCompile(`--capability[= ]([A-Za-z_]+)`)

func skillFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", skillDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no skill markdown found under %s", skillDir)
	}
	return files
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestSkillExists proves the skill entry point exists and documents itself.
func TestSkillExists(t *testing.T) {
	body := read(t, filepath.Join(skillDir, "SKILL.md"))
	if !strings.HasPrefix(body, "---") {
		t.Error("SKILL.md should start with YAML frontmatter (name/description)")
	}
	for _, want := range []string{"name: sop", "description:", "sop prompt"} {
		if !strings.Contains(body, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
}

// TestSkillExamplesExist proves every documented capability has an example.
func TestSkillExamplesExist(t *testing.T) {
	for _, name := range []string{"plan.md", "review.md", "diagnose.md", "implement.md"} {
		path := filepath.Join(skillDir, "examples", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing example %s: %v", name, err)
		}
	}
}

// TestSkillUsesCanonicalCapabilities proves every documented --capability value is a
// canonical CLI capability.
func TestSkillUsesCanonicalCapabilities(t *testing.T) {
	for _, file := range skillFiles(t) {
		body := read(t, file)
		for _, m := range capabilityArgRE.FindAllStringSubmatch(body, -1) {
			if !canonicalCapabilities[m[1]] {
				t.Errorf("%s: non-canonical capability %q", file, m[1])
			}
		}
	}
}

// TestSkillBashBlocksInvokeSOPOnly proves every shell example drives the `sop` CLI,
// never a provider directly.
func TestSkillBashBlocksInvokeSOPOnly(t *testing.T) {
	// A provider binary invoked as a command anywhere in the skill (not just in a
	// `sop ...` line) is a boundary violation.
	providerCommandRE := regexp.MustCompile(`(?m)^\s*(` + strings.Join(providerBinaries, "|") + `)\s`)
	for _, file := range skillFiles(t) {
		body := read(t, file)
		if m := providerCommandRE.FindString(body); m != "" {
			t.Errorf("%s: skill appears to invoke a provider directly: %q", file, strings.TrimSpace(m))
		}
		for _, line := range bashCommandLines(body) {
			if !strings.HasPrefix(line, "sop ") {
				t.Errorf("%s: shell example does not invoke sop: %q", file, line)
			}
		}
	}
}

// TestSkillDoesNotRestatePolicyOrMutate proves the skill does not carry SOP policy
// settings or direct-mutation instructions.
func TestSkillDoesNotRestatePolicyOrMutate(t *testing.T) {
	for _, file := range skillFiles(t) {
		body := read(t, file)
		for _, knob := range policyKnobs {
			if strings.Contains(body, knob) {
				t.Errorf("%s: skill restates a SOP policy setting %q", file, knob)
			}
		}
		for _, cmd := range mutationCommands {
			if strings.Contains(body, cmd) {
				t.Errorf("%s: skill contains a direct-mutation instruction %q", file, cmd)
			}
		}
	}
}

// bashCommandLines returns the command lines of every fenced bash/sh block: the
// first line of each command (continuation lines, which end in a backslash, are
// folded away).
func bashCommandLines(body string) []string {
	var out []string
	inFence := false
	prevContinued := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			if !inFence {
				inFence = lang == "bash" || lang == "sh"
				prevContinued = false
				continue
			}
			inFence = false
			continue
		}
		if !inFence || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if prevContinued {
			prevContinued = strings.HasSuffix(trimmed, "\\")
			continue
		}
		out = append(out, trimmed)
		prevContinued = strings.HasSuffix(trimmed, "\\")
	}
	return out
}
