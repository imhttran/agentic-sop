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

// skillDir is the shipped canonical skill, relative to this package (internal/skill).
// skillsRoot is the whole shipped Zed skill surface: every direct child is a skill
// folder, and Zed exposes each as a slash command named after the folder.
const skillDir = "../../skills/sop"
const skillsRoot = "../../skills"

var capabilityArgRE = regexp.MustCompile(`--capability[= ]([A-Za-z_]+)`)

func skillFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(skillsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", skillsRoot, err)
	}
	if len(files) == 0 {
		t.Fatalf("no skill markdown found under %s", skillsRoot)
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
			if !Capabilities[m[1]] {
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
	providerCommandRE := regexp.MustCompile(`(?m)^\s*(` + strings.Join(ProviderBinaries, "|") + `)\s`)
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
		for _, knob := range PolicyKnobs {
			if strings.Contains(body, knob) {
				t.Errorf("%s: skill restates a SOP policy setting %q", file, knob)
			}
		}
		for _, cmd := range MutationCommands {
			if strings.Contains(body, cmd) {
				t.Errorf("%s: skill contains a direct-mutation instruction %q", file, cmd)
			}
		}
	}
}

// TestZedSkillCommandsExist proves every documented Zed slash command ships as a
// skill folder whose frontmatter name matches the folder: Zed derives the command
// from that name, and an invalid or mismatched name fails to load.
func TestZedSkillCommandsExist(t *testing.T) {
	for _, c := range Commands {
		path := filepath.Join(skillsRoot, c.Name, "SKILL.md")
		body := read(t, path)
		if !strings.HasPrefix(body, "---") {
			t.Errorf("%s: SKILL.md must start with YAML frontmatter", path)
			continue
		}
		if !strings.Contains(body, "name: "+c.Name) {
			t.Errorf("%s: frontmatter name must match the folder (%q)", path, c.Name)
		}
		if !strings.Contains(body, "description:") {
			t.Errorf("%s: missing description (the catalog entry Zed shows)", path)
		}
	}
}

// TestZedSkillCapabilityMapping proves each command delegates to `sop prompt` and
// fixes exactly the capability it advertises. A command that fixes none must not
// embed a capability: the default belongs to the CLI.
func TestZedSkillCapabilityMapping(t *testing.T) {
	for _, c := range Commands {
		body := read(t, filepath.Join(skillsRoot, c.Name, "SKILL.md"))
		if !strings.Contains(body, c.Delegation) {
			t.Errorf("%s: must delegate with %q", c.Name, c.Delegation)
		}
		if c.Capability == "" && strings.Contains(body, "sop prompt --capability") && c.Name != "sop" {
			t.Errorf("%s: must not fix a capability; the CLI default applies", c.Name)
		}
	}
}

// TestSOPEntryPointListsAliases proves the /sop entry point tells the agent about
// every alias, so a general request can be routed to the right one.
func TestSOPEntryPointListsAliases(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop", "SKILL.md"))
	for _, c := range Commands {
		if c.Name == "sop" {
			continue
		}
		if !strings.Contains(body, "/"+c.Name) {
			t.Errorf("the /sop entry point should list /%s", c.Name)
		}
	}
}

// TestZedSkillsFailClosedWithoutSOP proves every command tells the agent to stop
// rather than answer the request itself when the `sop` CLI is unavailable.
func TestZedSkillsFailClosedWithoutSOP(t *testing.T) {
	for _, c := range Commands {
		body := read(t, filepath.Join(skillsRoot, c.Name, "SKILL.md"))
		if !strings.Contains(body, "PATH") {
			t.Errorf("%s: must instruct the agent to fail closed when `sop` is not on PATH", c.Name)
		}
	}
}

// TestImplementCommandDelegatesToGovernedPrompt is the boundary check for the
// mutating command: /sop-implement must enter SOP's governed implementation
// lifecycle by calling `sop prompt --capability implement`, never by mutating the
// repository (the direct-mutation check above covers the shell escape).
func TestImplementCommandDelegatesToGovernedPrompt(t *testing.T) {
	body := read(t, filepath.Join(skillsRoot, "sop-implement", "SKILL.md"))
	if !strings.Contains(body, "sop prompt --capability implement") {
		t.Error("sop-implement must invoke `sop prompt --capability implement`")
	}
	if !strings.Contains(body, "governed implementation lifecycle") {
		t.Error("sop-implement must state that SOP's governed implementation lifecycle runs")
	}
}

// TestInstallerKnowsEverySkill proves the installer's skill list matches the shipped
// surface: a shell list and the Go catalog drifting apart would install a partial UI.
// The shared installer owns the list; the per-agent wrappers forward to it.
func TestInstallerKnowsEverySkill(t *testing.T) {
	body := read(t, "../../scripts/install-skills.sh")
	for _, c := range Commands {
		if !strings.Contains(body, c.Name) {
			t.Errorf("scripts/install-skills.sh does not know the %q skill", c.Name)
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
