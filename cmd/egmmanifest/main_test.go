package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const h64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// unsignedManifest is a synthetic one-case manifest with sealed_sha256 cleared.
func unsignedManifest() []byte {
	return []byte(`{"schema_version":1,"corpus":"egm-independent-design-review","cases":[{"id":"S-01","stratum":"seeded","artifact_path":"artifact.md","artifact_sha256":"` + h64 + `","exported_tree_sha256":"` + h64 + `","revision_pin":"synthetic-pin","instruction_block_sha256":"` + h64 + `","seeded_defect":"SYNTHETIC-DEFECT-CLASS","determinability_rationale":"SYNTHETIC-RATIONALE-TEXT","expected_finding":{"location":"SYNTHETIC-LOCATION","severity":"HIGH"},"leak_checks":{"artifact_contains_answer":false,"permitted_reference_contains_answer":false,"later_evidence_in_tree":false},"sealed_sha256":""}]}`)
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunSealReportsOnlyPathDigestStatus(t *testing.T) {
	in := writeTemp(t, "unsigned.json", unsignedManifest())
	out := filepath.Join(t.TempDir(), "sealed.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"seal", "-input", in, "-output", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout should be one line, got %d: %q", len(lines), stdout.String())
	}
	for _, want := range []string{"sealed=" + out, "sha256=", "verify=ok"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("stdout %q missing %q", lines[0], want)
		}
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if !strings.Contains(lines[0], hex.EncodeToString(sum[:])) {
		t.Fatal("reported digest does not match the written file")
	}
	for _, secret := range []string{"SYNTHETIC-DEFECT-CLASS", "SYNTHETIC-RATIONALE-TEXT", "SYNTHETIC-LOCATION"} {
		if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatalf("output leaks %q", secret)
		}
	}
}

func TestRunSealRequiresBothFlags(t *testing.T) {
	in := writeTemp(t, "unsigned.json", unsignedManifest())
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seal", "-input", in}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-input and -output") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunSealFailureLeavesNoOutput(t *testing.T) {
	in := writeTemp(t, "invalid.json", []byte("{"))
	out := filepath.Join(t.TempDir(), "sealed.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seal", "-input", in, "-output", out}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on failure, got %q", stdout.String())
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatal("a failed seal left an output file")
	}
}

func TestRunSealRefusesExistingOutput(t *testing.T) {
	in := writeTemp(t, "unsigned.json", unsignedManifest())
	out := writeTemp(t, "sealed.json", []byte("keep me"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seal", "-input", in, "-output", out}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me" {
		t.Fatalf("existing output changed to %q", got)
	}
}

func TestRunVerifyAcceptsSealedOutput(t *testing.T) {
	in := writeTemp(t, "unsigned.json", unsignedManifest())
	out := filepath.Join(t.TempDir(), "sealed.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seal", "-input", in, "-output", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("seal exit = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"verify", "-manifest", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("verify exit = %d, stderr = %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "cases=1 verified=1 problems=0" {
		t.Fatalf("verify output = %q", got)
	}
}

func TestRunUsageAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("no args: exit = %d, want 2", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"bogus"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help: exit = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "egmmanifest seal") {
		t.Fatalf("help does not document seal: %q", stdout.String())
	}
}
