package redact

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is an assessment corpus for the remaining unknown-format residual.
// It records what the merged A+B/C1 detector does and does not redact. The seven
// approved provider formats are now enforced by C1; the context-free opaque
// value remains the assessed residual.

func TestUnknownFormatAssessmentCorpus(t *testing.T) {
	policy := Policy{Enabled: true}
	cases := []struct {
		name         string
		input        string
		wantRedacted bool
		category     string
	}{
		// C1 known formats.
		{"glpat", "value: glpat-abcdefghijklmnopqrst", true, "c1-known-format"},
		{"npm", "value: npm_abcdefghijklmnopqrstuvwxyz0123456789", true, "c1-known-format"},
		{"pypi", "value: pypi-abcdefghijklmnop", true, "c1-known-format"},
		{"shpat", "value: shpat_abcdefghijklmnopqrstuvwxyz012345", true, "c1-known-format"},
		{"ghs", "value: ghs_abcdefghijklmnopqrst", true, "c1-known-format"},
		{"ghu", "value: ghu_abcdefghijklmnopqrst", true, "c1-known-format"},
		{"google-api-key", "value: AIzaabcdefghijklmnopqrstuvwxyz012345678", true, "c1-known-format"},
		{"sendgrid", "value: SG.abcdefghijklmnop.qrstuvwxyz012345", true, "c1-known-format"},
		{"stripe-live", "value: sk_live_abcdefghijklmnop", true, "c1-known-format"},

		// A+B shape coverage.
		{"github-pat-shape", "value: ghp_abcdefghijklmnopqrst", true, "shape"},
		{"github-fine-grained", "value: github_pat_abcdefghijklmnopqrst", true, "shape"},
		{"slack", "value: xoxb-abcdefghijkl", true, "shape"},
		{"aws-access-key-id", "value: AKIAABCDEFGHIJKLMNOP", true, "shape"},
		{"jwt", "value: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop", true, "shape"},
		{"pem", "value: -----BEGIN PRIVATE KEY-----", true, "shape"},
		{"url-userinfo", "value: postgres://user:secretpassword@db.example/app", true, "shape"},
		{"bearer", "value: Authorization: Bearer abcdefghijklmnop", true, "shape"},

		// C1 contextual keys.
		{"opaque-secret", "opaque_secret=boundary-residual-value-99", true, "c1-contextual"},
		{"api-key", "api_key=Zx9K2mQ7pL4vN8rT1wY6bC3dF5gH0jS2", true, "c1-contextual"},
		{"mixed-key", "db_passWord=Zx9K2mQ7pL4vN8rT1wY6bC3dF5gH0jS2", true, "c1-contextual"},

		// Demonstrated missing conventions (expected unredacted today).
		{"google-oauth", "value: ya29.a0AfH6SMBexampleexampleexampleexample", true, "c1-seven-format"},
		{"aws-session-key", "value: ASIAABCDEFGHIJKLMNOP", true, "c1-seven-format"},
		{"stripe-test", "value: sk_test_abcdefghijklmnop", true, "c1-seven-format"},
		{"stripe-webhook", "value: whsec_abcdefghijklmnopqrst", true, "c1-seven-format"},
		{"docker-pat", "value: dckr_pat_abcdefghijklmnopqrst", true, "c1-seven-format"},
		{"age-secret", "value: AGE-SECRET-KEY-1ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true, "c1-seven-format"},
		{"redis-empty-user", "value: redis://:secretpassword@cache.example:6379/0", true, "c1-seven-format"},
		{"context-free-base64", "value: Zx9K2mQ7pL4vN8rT1wY6bC3dF5gH0jS2", false, "context-free"},

		// Ordinary content controls.
		{"git-sha", "value: 0123456789abcdef0123456789abcdef01234567", false, "ordinary"},
		{"uuid", "value: 123e4567-e89b-12d3-a456-426614174000", false, "ordinary"},
		{"sha256", "value: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", false, "ordinary"},
		{"base64-hash", "value: SGVsbG8gV29ybGQhIFRoaXMgaXMgYW4gb3JkaW5hcnkgYmFzZTY0IHN0cmluZw==", false, "ordinary"},
		{"source-path", "value: internal/toolharness/isolation.go", false, "ordinary"},
		{"identifier", "value: maxIdentityScanEntries", false, "ordinary"},
		{"url", "value: https://example.com/docs/getting-started", false, "ordinary"},
	}

	redacted, missing := 0, 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ModelInputWithPolicy(tc.input, nil, policy)
			isRedacted := got != tc.input
			if isRedacted != tc.wantRedacted {
				t.Fatalf("redacted=%t, want %t; category=%s; output=%q", isRedacted, tc.wantRedacted, tc.category, got)
			}
			if isRedacted {
				redacted++
			}
			if tc.category == "missing-format" || tc.category == "context-free" {
				missing++
			}
		})
	}
	t.Logf("corpus: %d cases, %d redacted, %d known missing/residual", len(cases), redacted, missing)
}

func TestUnknownFormatInventoryClosesExactValue(t *testing.T) {
	const opaque = "boundary-residual-value-99"
	if got := ModelInputWithPolicy("value: "+opaque, nil, Policy{Enabled: true}); !strings.Contains(got, opaque) {
		t.Fatalf("the context-free residual is no longer observed; update the assessment: %q", got)
	}
	if got := ModelInputWithPolicy("value: "+opaque, nil, Policy{}); !strings.Contains(got, opaque) {
		t.Fatalf("C1 disabled unexpectedly redacted the opaque value: %q", got)
	}

	path := filepath.Join(t.TempDir(), "inventory.txt")
	if err := os.WriteFile(path, []byte(opaque+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadInventory(path, InventoryLimits{})
	if err != nil {
		t.Fatal(err)
	}
	got := ModelInputWithPolicy("value: "+opaque, nil, Policy{Enabled: true, Values: values})
	if strings.Contains(got, opaque) {
		t.Fatalf("the external inventory did not redact the exact value: %q", got)
	}
}

func TestUnknownFormatC1OnlyFormat(t *testing.T) {
	const glpat = "glpat-abcdefghijklmnopqrst"
	if got := ModelInputWithPolicy("value: "+glpat, nil, Policy{}); !strings.Contains(got, glpat) {
		t.Fatalf("C1-disabled unexpectedly redacted a C1-only format: %q", got)
	}
	if got := ModelInputWithPolicy("value: "+glpat, nil, Policy{Enabled: true}); strings.Contains(got, glpat) {
		t.Fatalf("C1-enabled did not redact a known format: %q", got)
	}
}

func entropyBitsPerChar(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int)
	for _, r := range s {
		counts[r]++
	}
	total := float64(len([]rune(s)))
	var entropy float64
	for _, count := range counts {
		p := float64(count) / total
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func TestEntropyFalsePositiveAssessment(t *testing.T) {
	type sample struct {
		name   string
		value  string
		secret bool
	}
	samples := []sample{
		{"high-entropy-token", "Zx9K2mQ7pL4vN8rT1wY6bC3dF5gH0jS2", true},
		{"low-entropy-opaque", "hunter2hunter2hunter2", true},
		{"git-sha", "0123456789abcdef0123456789abcdef01234567", false},
		{"uuid", "123e4567-e89b-12d3-a456-426614174000", false},
		{"sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", false},
		{"base64-hash", "SGVsbG8gV29ybGQhIFRoaXMgaXMgYW4gb3JkaW5hcnkgYmFzZTY0IHN0cmluZw==", false},
		{"source-path", "internal/toolharness/isolation.go", false},
		{"identifier", "maxIdentityScanEntries", false},
	}
	const minLen = 20
	for _, threshold := range []float64{3.5, 4.0} {
		falsePositives, truePositives, falseNegatives := 0, 0, 0
		for _, s := range samples {
			flagged := len(s.value) >= minLen && entropyBitsPerChar(s.value) > threshold
			if flagged {
				t.Logf("threshold %.1f flagged %s (secret=%t, entropy=%.2f)", threshold, s.name, s.secret, entropyBitsPerChar(s.value))
			}
			switch {
			case flagged && s.secret:
				truePositives++
			case flagged && !s.secret:
				falsePositives++
			case !flagged && s.secret:
				falseNegatives++
			}
		}
		t.Logf("entropy threshold %.1f: true positives=%d false positives=%d false negatives=%d", threshold, truePositives, falsePositives, falseNegatives)
		if falsePositives == 0 {
			t.Fatalf("threshold %.1f flagged no ordinary identifier; the corpus is not exercising false positives", threshold)
		}
		if falseNegatives == 0 {
			t.Fatalf("threshold %.1f caught every secret; adjust the corpus to include a low-entropy opaque value", threshold)
		}
	}
}
