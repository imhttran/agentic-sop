package egmmanifest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

// unsignedBytes marshals m and clears every sealed_sha256, producing sealer input.
func unsignedBytes(t *testing.T, m *egmmanifest.Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	cases, ok := generic["cases"].([]any)
	if !ok {
		t.Fatal("fixture has no cases array")
	}
	for _, c := range cases {
		c.(map[string]any)["sealed_sha256"] = ""
	}
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// reindent re-encodes the same JSON with different whitespace and tab indentation.
func reindent(t *testing.T, b []byte) []byte {
	t.Helper()
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(generic, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeBytes(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestSealBytesDeterministic(t *testing.T) {
	m, _ := newFixture(t)
	in := unsignedBytes(t, m)

	_, out1, err := egmmanifest.SealBytes(in)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	_, out2, err := egmmanifest.SealBytes(in)
	if err != nil {
		t.Fatalf("seal again: %v", err)
	}
	if string(out1) != string(out2) {
		t.Fatal("two seals of identical input differ")
	}
	if sha256Hex(out1) != sha256Hex(out2) {
		t.Fatal("two seals produced different digests")
	}
}

func TestSealComputesCaseDigests(t *testing.T) {
	m, _ := newFixture(t)
	sealed, _, err := egmmanifest.SealBytes(unsignedBytes(t, m))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for i, c := range sealed.Cases {
		want, err := egmmanifest.CaseDigest(c)
		if err != nil {
			t.Fatal(err)
		}
		if c.SealedSHA256 != want {
			t.Fatalf("case %d: sealed_sha256 = %s, want %s", i, c.SealedSHA256, want)
		}
	}
	if sealed.Cases[0].SealedSHA256 == sealed.Cases[1].SealedSHA256 {
		t.Fatal("distinct cases share a digest")
	}
}

func TestSealTrailingNewlineAndCanonicalBytes(t *testing.T) {
	m, _ := newFixture(t)
	in := unsignedBytes(t, m)
	sealed, out, err := egmmanifest.SealBytes(in)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Fatal("output does not end with a newline")
	}
	if len(out) > 1 && out[len(out)-2] == '\n' {
		t.Fatal("output ends with more than one newline")
	}
	canonical, err := egmmanifest.Canonical(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(canonical)+"\n" {
		t.Fatal("output is not canonical bytes plus one newline")
	}
}

func TestSealIgnoresFormatting(t *testing.T) {
	m, _ := newFixture(t)
	a := unsignedBytes(t, m)
	b := reindent(t, a)
	if string(a) == string(b) {
		t.Fatal("reformatted input is byte-identical; test is not exercising formatting")
	}
	_, outA, err := egmmanifest.SealBytes(a)
	if err != nil {
		t.Fatalf("seal a: %v", err)
	}
	_, outB, err := egmmanifest.SealBytes(b)
	if err != nil {
		t.Fatalf("seal b: %v", err)
	}
	if string(outA) != string(outB) {
		t.Fatal("canonical output depends on input formatting")
	}
}

func TestSealFileRoundTripAndExactByteHash(t *testing.T) {
	m, fixtureDir := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))
	out := filepath.Join(dir, "sealed.json")

	res, err := egmmanifest.SealFile(in, out)
	if err != nil {
		t.Fatalf("seal file: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if res.SHA256 != sha256Hex(data) {
		t.Fatalf("reported digest %s does not match the file bytes %s", res.SHA256, sha256Hex(data))
	}
	if res.Cases != 2 {
		t.Fatalf("cases = %d, want 2", res.Cases)
	}
	if res.OutputPath != out {
		t.Fatalf("output path = %s, want %s", res.OutputPath, out)
	}

	loaded, err := egmmanifest.Load(out)
	if err != nil {
		t.Fatalf("sealed output does not load: %v", err)
	}
	rep, err := egmmanifest.Verify(loaded, egmmanifest.Options{ArtifactRoot: fixtureDir})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !rep.OK() {
		t.Fatalf("sealed output does not verify: %+v", rep.Problems)
	}
}

func TestSealOutputPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not meaningful on Windows")
	}
	m, _ := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))
	out := filepath.Join(dir, "sealed.json")
	if _, err := egmmanifest.SealFile(in, out); err != nil {
		t.Fatalf("seal: %v", err)
	}
	info, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o400 {
		t.Fatalf("sealed file mode = %04o, want 0400", got)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("sealed output is a symlink")
	}
}

func TestSealRefusesModifiedRecord(t *testing.T) {
	m, _ := newFixture(t)
	in := unsignedBytes(t, m)
	var generic map[string]any
	if err := json.Unmarshal(in, &generic); err != nil {
		t.Fatal(err)
	}
	// A valid-format digest that does not match the record: the record changed after sealing.
	generic["cases"].([]any)[0].(map[string]any)["sealed_sha256"] = strings.Repeat("a", 64)
	bad, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = egmmanifest.SealBytes(bad)
	if err == nil {
		t.Fatal("a mismatched sealed_sha256 was accepted")
	}
	if !strings.Contains(err.Error(), "reseal") {
		t.Fatalf("error does not explain the refusal: %v", err)
	}
}

func TestSealRejectsInvalidInput(t *testing.T) {
	base, _ := newFixture(t)
	mutations := map[string]func(map[string]any){
		"unsupported schema": func(g map[string]any) { g["schema_version"] = 2 },
		"missing corpus":     func(g map[string]any) { delete(g, "corpus") },
		"no cases":           func(g map[string]any) { g["cases"] = []any{} },
		"missing leak_checks": func(g map[string]any) {
			for _, c := range g["cases"].([]any) {
				delete(c.(map[string]any), "leak_checks")
			}
		},
		"partial leak_checks": func(g map[string]any) {
			for _, c := range g["cases"].([]any) {
				c.(map[string]any)["leak_checks"] = map[string]any{"artifact_contains_answer": false}
			}
		},
		"duplicate id": func(g map[string]any) {
			cases := g["cases"].([]any)
			cases[1].(map[string]any)["id"] = cases[0].(map[string]any)["id"]
		},
		"missing required field": func(g map[string]any) {
			delete(g["cases"].([]any)[0].(map[string]any), "artifact_sha256")
		},
		"bad stratum": func(g map[string]any) {
			g["cases"].([]any)[0].(map[string]any)["stratum"] = "unknown"
		},
		"unsafe path": func(g map[string]any) {
			g["cases"].([]any)[0].(map[string]any)["artifact_path"] = "../escape.md"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			in := unsignedBytes(t, base)
			var generic map[string]any
			if err := json.Unmarshal(in, &generic); err != nil {
				t.Fatal(err)
			}
			mutate(generic)
			bad, err := json.Marshal(generic)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := egmmanifest.SealBytes(bad); err == nil {
				t.Fatal("invalid input was sealed")
			}
		})
	}
}

func TestSealRejectsTrailingData(t *testing.T) {
	m, _ := newFixture(t)
	in := append(unsignedBytes(t, m), []byte("]")...)
	if _, _, err := egmmanifest.SealBytes(in); err == nil {
		t.Fatal("trailing data was accepted")
	}
}

func TestSealFileRejectsExistingOutput(t *testing.T) {
	m, _ := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))
	out := writeBytes(t, dir, "sealed.json", []byte("keep me"))

	if _, err := egmmanifest.SealFile(in, out); err == nil {
		t.Fatal("existing output was overwritten")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me" {
		t.Fatalf("existing output changed to %q", got)
	}
}

func TestSealFileRejectsAliasedPaths(t *testing.T) {
	m, _ := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))

	if _, err := egmmanifest.SealFile(in, in); err == nil {
		t.Fatal("identical input and output paths were accepted")
	}

	alias := filepath.Join(dir, "alias.json")
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	if err := os.Symlink(in, alias); err != nil {
		t.Skip("symlink unsupported:", err)
	}
	if _, err := egmmanifest.SealFile(in, alias); err == nil {
		t.Fatal("a symlink alias of the input was accepted as output")
	}
}

func TestSealFileRejectsSymlinkOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	m, _ := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))

	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink unsupported:", err)
	}
	if _, err := egmmanifest.SealFile(in, link); err == nil {
		t.Fatal("a dangling symlink was accepted as output")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the symlink target was created")
	}
}

func TestSealFileRejectsUnreadableInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read an unreadable file")
	}
	m, _ := newFixture(t)
	dir := t.TempDir()
	in := writeBytes(t, dir, "unsigned.json", unsignedBytes(t, m))
	if err := os.Chmod(in, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(in, 0o600) })
	if _, err := egmmanifest.SealFile(in, filepath.Join(dir, "sealed.json")); err == nil {
		t.Fatal("an unreadable input was sealed")
	}
}

func TestSealFileLeavesNoArtifactsOnFailure(t *testing.T) {
	m, _ := newFixture(t)
	dir := t.TempDir()
	out := writeBytes(t, dir, "sealed.json", []byte("keep me"))

	valid := writeBytes(t, dir, "valid.json", unsignedBytes(t, m))
	if _, err := egmmanifest.SealFile(valid, out); err == nil {
		t.Fatal("expected failure on existing output")
	}
	invalid := writeBytes(t, dir, "invalid.json", []byte("{"))
	if _, err := egmmanifest.SealFile(invalid, filepath.Join(dir, "other.json")); err == nil {
		t.Fatal("expected failure on invalid input")
	}
	if _, err := os.Lstat(filepath.Join(dir, "other.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a failed seal left an output file")
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".egmmanifest-seal-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("failed seals left temporary files: %v", leftovers)
	}
}

// TestSealErrorsCarryNoContent checks that no failure path echoes answer-bearing content.
func TestSealErrorsCarryNoContent(t *testing.T) {
	m, _ := newFixture(t)
	secrets := []string{syntheticAnswer, syntheticLeak, m.Cases[0].DeterminabilityRationale}
	secrets = append(secrets, m.Cases[0].Distractors...)

	base := unsignedBytes(t, m)
	var generic map[string]any
	if err := json.Unmarshal(base, &generic); err != nil {
		t.Fatal(err)
	}
	cases := generic["cases"].([]any)
	cases[0].(map[string]any)["expected_finding"] = map[string]any{"location": "", "severity": "HIGH"}
	cases[1].(map[string]any)["id"] = cases[0].(map[string]any)["id"]
	bad, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = egmmanifest.SealBytes(bad)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	msg := err.Error()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		if strings.Contains(msg, s) {
			t.Fatalf("error leaks answer content %q: %s", s, msg)
		}
	}
}
