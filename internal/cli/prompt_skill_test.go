package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillExamplesMapToPromptCommand proves the documented skill invocations map
// onto the real `sop prompt` command: every `sop prompt ...` command in the shipped
// examples parses as valid CLI arguments with a canonical capability, and one runs
// end to end through the dispatcher.
func TestSkillExamplesMapToPromptCommand(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{plan: "p", review: "r"}

	commands := documentedPromptCommands(t)
	if len(commands) == 0 {
		t.Fatal("no documented `sop prompt` commands found in skills/sop/examples")
	}
	for _, cmd := range commands {
		args := tokenizeCommand(cmd)
		if len(args) < 2 || args[0] != "sop" || args[1] != "prompt" {
			t.Fatalf("documented command is not `sop prompt`: %q", cmd)
		}
		opts, ok := parsePromptArgs(args[2:], io.Discard)
		if !ok {
			t.Errorf("documented command does not parse: %q", cmd)
			continue
		}
		if _, err := parsePromptCapability(opts.capability); err != nil {
			t.Errorf("documented command has a non-canonical capability: %q (%v)", cmd, err)
		}
	}

	// Execute the first read-only example through the same path the skill uses.
	first := tokenizeCommand(commands[0])
	code, _, stderr := runCLIWithAgent(t, dir, a, first[1:]...)
	if code != exitOK {
		t.Fatalf("documented command failed to run: code=%d stderr=%s", code, stderr)
	}
}

// documentedPromptCommands extracts the `sop prompt ...` command lines (joining
// backslash continuations) from the shipped skill examples.
func documentedPromptCommands(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "..", "skills", "sop", "examples", "*.md"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no skill examples found: %v", err)
	}
	var out []string
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, joinContinuations(string(data))...)
	}
	return out
}

// joinContinuations returns the logical command lines of a markdown file: lines
// ending in a backslash are joined with the following line.
func joinContinuations(body string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if cur.Len() == 0 && !strings.HasPrefix(trimmed, "sop ") {
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			cur.WriteString(strings.TrimSuffix(trimmed, "\\"))
			cur.WriteString(" ")
			continue
		}
		cur.WriteString(trimmed)
		out = append(out, strings.Join(strings.Fields(cur.String()), " "))
		cur.Reset()
	}
	return out
}

// tokenizeCommand splits a command line into arguments, honoring double quotes.
func tokenizeCommand(cmd string) []string {
	var args []string
	var cur strings.Builder
	inQuote := false
	for _, r := range cmd {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}
