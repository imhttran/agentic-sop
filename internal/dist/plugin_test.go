package dist

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/skill"
)

// The packaged Claude Code plugin, and the marketplace that lists it. Both live in the
// repository so Claude Code can register the checkout directly (see the official plugin
// and marketplace references), and the plugin's skill files are a generated mirror of
// the canonical skills/ tree.
const (
	pluginDir       = "../../integrations/claude"
	marketplacePath = "../../.claude-plugin/marketplace.json"
	pluginName      = "sop"
	marketplaceName = "agentic-sop"
)

// claudeManifest is the subset of the official plugin.json schema SOP declares.
type claudeManifest struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	License string `json:"license"`
}

// claudeMarketplace is the subset of the official marketplace.json schema SOP declares.
type claudeMarketplace struct {
	Name  string `json:"name"`
	Owner struct {
		Name string `json:"name"`
	} `json:"owner"`
	Plugins []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"plugins"`
}

func manifest(t *testing.T) claudeManifest {
	t.Helper()
	var m claudeManifest
	raw := read(t, filepath.Join(pluginDir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("plugin.json does not parse: %v", err)
	}
	return m
}

// pluginFiles returns every file in the packaged plugin.
func pluginFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", pluginDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("the plugin package is empty: %s", pluginDir)
	}
	return files
}

// TestClaudePluginManifestIsValid proves the package satisfies the official manifest
// requirements: the manifest exists, parses, and carries the metadata the validator
// expects (a kebab-case name, a description, a version, and an author).
func TestClaudePluginManifestIsValid(t *testing.T) {
	path := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the plugin manifest is missing: %v", err)
	}
	m := manifest(t)
	if m.Name != pluginName {
		t.Errorf("manifest name = %q, want %q", m.Name, pluginName)
	}
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(m.Name) {
		t.Errorf("manifest name %q is not kebab-case", m.Name)
	}
	for _, field := range []struct{ name, value string }{
		{"description", m.Description},
		{"version", m.Version},
		{"author.name", m.Author.Name},
		{"license", m.License},
	} {
		if strings.TrimSpace(field.value) == "" {
			t.Errorf("manifest %s is empty", field.name)
		}
	}
}

// TestClaudeMarketplaceListsThePlugin proves the repository is a valid marketplace for
// the plugin, and that the names agree: the official documentation requires an entry
// name to match the plugin's manifest name, and a relative source without "..".
func TestClaudeMarketplaceListsThePlugin(t *testing.T) {
	var mk claudeMarketplace
	if err := json.Unmarshal([]byte(read(t, marketplacePath)), &mk); err != nil {
		t.Fatalf("marketplace.json does not parse: %v", err)
	}
	if strings.TrimSpace(mk.Name) == "" || strings.TrimSpace(mk.Owner.Name) == "" {
		t.Errorf("marketplace needs a name and an owner: %+v", mk)
	}
	if mk.Name != marketplaceName {
		t.Errorf("marketplace name = %q, want %q", mk.Name, marketplaceName)
	}
	if len(mk.Plugins) != 1 {
		t.Fatalf("marketplace lists %d plugins, want exactly the SOP plugin", len(mk.Plugins))
	}
	entry := mk.Plugins[0]
	if entry.Name != manifest(t).Name {
		t.Errorf("marketplace entry name %q must match the manifest name %q", entry.Name, manifest(t).Name)
	}
	if !strings.HasPrefix(entry.Source, "./") || strings.Contains(entry.Source, "..") {
		t.Errorf("marketplace entry source %q must be a relative path without \"..\"", entry.Source)
	}
	// The source must resolve to the plugin root, which is the directory the marketplace
	// root points at (the marketplace root is the directory holding .claude-plugin/).
	root := filepath.Join(filepath.Dir(filepath.Dir(marketplacePath)), filepath.FromSlash(entry.Source))
	if _, err := os.Stat(filepath.Join(root, ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("marketplace source %q does not resolve to the plugin: %v", entry.Source, err)
	}
}

// TestClaudePluginMirrorsTheCanonicalSkills proves the plugin's skill files are the
// canonical skills/, generated rather than hand-maintained. It runs the generator's own
// check, so drift in either direction fails here.
//
// The generator is a POSIX shell script, so the check runs where sh is available; the
// package's structure and content are validated on every platform regardless.
func TestClaudePluginMirrorsTheCanonicalSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts/build-claude-plugin.sh needs a POSIX shell; run this on Linux/macOS")
	}
	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "build-claude-plugin.sh"), "--check")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the plugin package has drifted from skills/: %v\n%s", err, out)
	}

	// The mirror is exactly the canonical command surface: no strays, nothing missing.
	entries, err := os.ReadDir(filepath.Join(pluginDir, "skills"))
	if err != nil {
		t.Fatalf("read the plugin skills: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			got[e.Name()] = true
		}
	}
	for _, c := range skill.Commands {
		if !got[c.Name] {
			t.Errorf("the plugin is missing the %q command", c.Name)
		}
		delete(got, c.Name)
	}
	for name := range got {
		t.Errorf("the plugin exposes an unexpected command %q", name)
	}
}

// TestClaudePluginExposesTheCommandSurface proves every packaged command delegates to
// `sop prompt` with the right capability, and that a command which fixes no capability
// does not embed one.
func TestClaudePluginExposesTheCommandSurface(t *testing.T) {
	for _, c := range skill.Commands {
		body := read(t, filepath.Join(pluginDir, "skills", c.Name, "SKILL.md"))
		if !strings.HasPrefix(body, "---") {
			t.Errorf("%s: SKILL.md must start with YAML frontmatter", c.Name)
		}
		if !strings.Contains(body, "name: "+c.Name) {
			t.Errorf("%s: frontmatter name must match the folder", c.Name)
		}
		if !strings.Contains(body, c.Delegation) {
			t.Errorf("%s: must delegate with %q", c.Name, c.Delegation)
		}
		if c.Capability == "" {
			if c.Name != "sop" && strings.Contains(body, "sop prompt --capability") {
				t.Errorf("%s: fixes no capability, so it must not embed one", c.Name)
			}
			continue
		}
		if !strings.Contains(body, "--capability "+c.Capability) {
			t.Errorf("%s: must fix the %q capability", c.Name, c.Capability)
		}
	}
}

// TestClaudePluginCarriesNoPolicyOrModelAccess proves the packaged adapters stay thin:
// no provider invocation, no SOP policy setting, no model name, and no direct-mutation
// instruction. The plugin is an interface to SOP, never a second engine.
func TestClaudePluginCarriesNoPolicyOrModelAccess(t *testing.T) {
	providerCommand := regexp.MustCompile(`(?m)^\s*(` + strings.Join(skill.ProviderBinaries, "|") + `)\s`)
	for _, file := range pluginFiles(t) {
		body := read(t, file)
		if m := providerCommand.FindString(body); m != "" {
			t.Errorf("%s: appears to invoke a provider directly: %q", file, strings.TrimSpace(m))
		}
		for _, knob := range skill.PolicyKnobs {
			if strings.Contains(body, knob) {
				t.Errorf("%s: restates the SOP policy setting %q", file, knob)
			}
		}
		for _, literal := range skill.ModelLiterals {
			if strings.Contains(strings.ToLower(body), literal) {
				t.Errorf("%s: names a model (%q); only SOP selects a model", file, literal)
			}
		}
		for _, cmd := range skill.MutationCommands {
			if strings.Contains(body, cmd) {
				t.Errorf("%s: contains a direct-mutation instruction %q", file, cmd)
			}
		}
	}
}

// TestClaudePluginImplementIsGoverned proves the one mutating command is exactly that:
// it maps only to implement, it states that SOP's governed lifecycle runs, and it is
// hidden from Claude's autonomous catalog so a repository change is always an explicit
// operator choice.
func TestClaudePluginImplementIsGoverned(t *testing.T) {
	body := read(t, filepath.Join(pluginDir, "skills", "sop-implement", "SKILL.md"))
	if !strings.Contains(body, "disable-model-invocation: true") {
		t.Error("sop-implement must be hidden from the autonomous catalog")
	}
	if !strings.Contains(body, "governed implementation lifecycle") {
		t.Error("sop-implement must state that SOP's governed implementation lifecycle runs")
	}
	if !strings.Contains(body, "--capability implement") {
		t.Error("sop-implement must invoke `sop prompt --capability implement`")
	}
	// No other command mutates: implement is the capability of exactly one command, and
	// it is sop-implement. A capability-specific command mentions no other capability, so
	// a read-only command can never be routed to implementation.
	implement := 0
	capabilityRE := regexp.MustCompile(`--capability[= ]([a-z_]+)`)
	for _, c := range skill.Commands {
		if c.Capability == "implement" {
			implement++
			if c.Name != "sop-implement" {
				t.Errorf("capability implement is mapped by %s, want sop-implement", c.Name)
			}
		}
		if c.Capability == "" {
			continue // the canonical entry point documents the whole surface by design
		}
		other := read(t, filepath.Join(pluginDir, "skills", c.Name, "SKILL.md"))
		for _, m := range capabilityRE.FindAllStringSubmatch(other, -1) {
			if m[1] != c.Capability {
				t.Errorf("%s fixes %q but mentions --capability %s", c.Name, c.Capability, m[1])
			}
		}
	}
	if implement != 1 {
		t.Errorf("the command surface maps implement %d times, want exactly once", implement)
	}
}

// TestClaudePluginFailsClosedWithoutSOP proves every packaged command tells the agent to
// stop rather than answer the request itself when the sop CLI is unavailable.
func TestClaudePluginFailsClosedWithoutSOP(t *testing.T) {
	for _, c := range skill.Commands {
		body := read(t, filepath.Join(pluginDir, "skills", c.Name, "SKILL.md"))
		if !strings.Contains(body, "PATH") {
			t.Errorf("%s: must instruct the agent to fail closed when `sop` is not on PATH", c.Name)
		}
	}
}
