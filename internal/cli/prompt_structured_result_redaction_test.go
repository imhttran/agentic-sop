package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const promptResultRedactionCredential = "AKIAIOSFODNN7EXAMPLE" // synthetic known-format credential fixture

// TestPromptStructuredResultRedactsKnownFormatSecrets is the regression: the
// structured result printed by --json and persisted as metadata.json must not
// expose a known-format credential, just as result.md must not. The two serialized
// documents are checked in independent subtests so a failure in one cannot hide
// the result for the other, and both decoded results must match the sanitized
// result.md content exactly.
func TestPromptStructuredResultRedactsKnownFormatSecrets(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	rawResult := "review output with " + promptResultRedactionCredential
	a := &fakeCapabilityAgent{review: rawResult}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--json", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	runDir := promptRunDirs(t, dir)[0]
	saved, err := os.ReadFile(filepath.Join(runDir, "result.md"))
	if err != nil {
		t.Fatalf("result.md is not readable: %v", err)
	}
	sanitized := string(saved)
	if strings.Contains(sanitized, promptResultRedactionCredential) {
		t.Errorf("result.md contains the synthetic credential fixture")
	}
	if sanitized == rawResult {
		t.Errorf("result.md was not sanitized")
	}

	t.Run("json stdout", func(t *testing.T) {
		if strings.Contains(stdout, promptResultRedactionCredential) {
			t.Errorf("--json stdout contains the synthetic credential fixture")
		}
		var doc promptResultDoc
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &doc); err != nil {
			t.Fatalf("stdout is not JSON: %v", err)
		}
		if doc.Result != sanitized {
			t.Errorf("--json result does not match the sanitized result.md content")
		}
		if doc.ResultPath == "" {
			t.Errorf("--json result_path is empty")
		}
	})

	t.Run("metadata.json", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
		if err != nil {
			t.Fatalf("metadata.json is not readable: %v", err)
		}
		if strings.Contains(string(data), promptResultRedactionCredential) {
			t.Errorf("metadata.json contains the synthetic credential fixture")
		}
		var md promptResultDoc
		if err := json.Unmarshal(data, &md); err != nil {
			t.Fatalf("metadata.json is not JSON: %v", err)
		}
		if md.Result != sanitized {
			t.Errorf("metadata result does not match the sanitized result.md content")
		}
		if md.ResultPath == "" {
			t.Errorf("metadata result_path is empty")
		}
	})
}

// TestPromptStructuredResultPreservesOrdinaryContent is the control: redaction
// must not alter ordinary content, and the result path must stay readable.
func TestPromptStructuredResultPreservesOrdinaryContent(t *testing.T) {
	clearProviderEnv(t)
	const content = "the ordinary review"
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: content}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--json", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	runDir := promptRunDirs(t, dir)[0]
	saved, err := os.ReadFile(filepath.Join(runDir, "result.md"))
	if err != nil {
		t.Fatalf("result.md is not readable: %v", err)
	}
	if string(saved) != content {
		t.Errorf("result.md = %q, want the ordinary content", string(saved))
	}

	t.Run("json stdout", func(t *testing.T) {
		var doc promptResultDoc
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &doc); err != nil {
			t.Fatalf("stdout is not JSON: %v", err)
		}
		if doc.Result != content {
			t.Errorf("--json result = %q, want the ordinary content", doc.Result)
		}
		if doc.ResultPath == "" {
			t.Fatalf("--json result_path is empty")
		}
		readable, err := os.ReadFile(filepath.Join(dir, doc.ResultPath))
		if err != nil {
			t.Fatalf("result_path is not readable: %v", err)
		}
		if string(readable) != content {
			t.Errorf("result.md via result_path = %q, want the ordinary content", string(readable))
		}
	})

	t.Run("metadata.json", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
		if err != nil {
			t.Fatalf("metadata.json is not readable: %v", err)
		}
		var md promptResultDoc
		if err := json.Unmarshal(data, &md); err != nil {
			t.Fatalf("metadata.json is not JSON: %v", err)
		}
		if md.Result != content {
			t.Errorf("metadata result = %q, want the ordinary content", md.Result)
		}
		if md.ResultPath == "" {
			t.Errorf("metadata result_path is empty")
		}
	})
}
