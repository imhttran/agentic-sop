package redact

import (
	"reflect"
	"testing"
)

const m = "[REDACTED]"

// The corpus every surface must catch. Each row is a diagnostic shape that has
// carried a credential in the wild; the last rows must survive so a redaction
// never eats readable context.
func TestShapesValues(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"env assignment", "api_key=sk-live-123", "api_key=[REDACTED]"},
		{"colon assignment", "token: abc123def", "token: [REDACTED]"},
		{"json body", `{"api_key": "sk-live-123"}`, `{"api_key": "[REDACTED]"}`},
		{"provider prose", "Incorrect API key provided: sk-proj-abc123", "Incorrect API key provided: [REDACTED]"},
		{"secret key", "SECRET_KEY=abc123", "SECRET_KEY=[REDACTED]"},
		// The key is not a credential name, so the URL rule redacts the password.
		{"url credentials", "DATABASE_URL=postgres://user:s3cret@db:5432/app", "DATABASE_URL=postgres://user:[REDACTED]@db:5432/app"},
		{"bearer keeps context", "Authorization: Bearer abcdefghijkl", "Authorization: Bearer [REDACTED]"},
		{"github token", "using ghp_abcdefghijklmnopqrstuvwxyz0123456789", "using [REDACTED]"},
		{"aws key id", "AKIAIOSFODNN7EXAMPLE", "[REDACTED]"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.invalidsignature", "[REDACTED]"},
		{"pem header", "-----BEGIN RSA PRIVATE KEY-----", "[REDACTED]"},
		{"plain diagnostic", "sop validate: exit status 1", "sop validate: exit status 1"},
		{"bare token word", "reason: token budget exhausted", "reason: token budget exhausted"},
		{"plain url", "GET https://example.com/v1/status -> 200", "GET https://example.com/v1/status -> 200"},
		{"file path", "reading internal/controller/sopclient/commands.go", "reading internal/controller/sopclient/commands.go"},
		{"basic auth word", "basic authentication failed", "basic authentication failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShapesValues(tc.in, Fixed(m)); got != tc.want {
				t.Errorf("ShapesValues(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Shapes replaces the whole match, key included; ShapesValues keeps the key.
func TestShapesReplacesWholeMatch(t *testing.T) {
	if got := Shapes("api_key=sk-live-123", Fixed(m)); got != m {
		t.Errorf("Shapes = %q, want %q", got, m)
	}
}

// A Marker can render per kind, so a surface keeps its typed markers.
func TestMarkerPerKind(t *testing.T) {
	byKind := func(k Kind) string {
		switch k {
		case KindKey, KindToken, KindAccessKey:
			return "<" + string(k) + ">"
		default:
			return m
		}
	}
	got := ShapesValues("grep AKIAIOSFODNN7EXAMPLE --key sk-live-123", byKind)
	want := "grep <access-key> --key <key>"
	if got != want {
		t.Errorf("ShapesValues with per-kind markers = %q, want %q", got, want)
	}
}

// A line-level surface loses the credentialed line and keeps the rest.
func TestLine(t *testing.T) {
	in := "sop run: exit status 1\napi_key=sk-live-123\nreason: build failed"
	want := "sop run: exit status 1\n" + m + "\nreason: build failed"
	if got := Line(in, m); got != want {
		t.Errorf("Line = %q, want %q", got, want)
	}
	if got := Line("reason: build failed", m); got != "reason: build failed" {
		t.Errorf("Line redacted a clean line: %q", got)
	}
}

// An already-written marker must not be matched again.
func TestShapesValuesIsIdempotent(t *testing.T) {
	once := ShapesValues("api_key=[REDACTED] and token: [REDACTED]", Fixed(m))
	if again := ShapesValues(once, Fixed(m)); again != once {
		t.Errorf("second pass changed %q to %q", once, again)
	}
}

func TestValues(t *testing.T) {
	secrets := []string{"short", "a-longer-secret-value", ""}
	got := Values("token short and a-longer-secret-value", secrets, Fixed(m))
	want := "token " + m + " and " + m
	if got != want {
		t.Errorf("Values = %q, want %q", got, want)
	}
}

func TestEnvSecrets(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"OPENAI_API_KEY=sk-live-123456",
		"GITHUB_TOKEN=ghp_abcdefghijklmnop",
		"SOP_SHORT=ab",
		"HOME=/home/operator",
		"OPENAI_API_KEY_DUP=sk-live-123456",
	}
	got := EnvSecrets(env)
	want := []string{"ghp_abcdefghijklmnop", "sk-live-123456"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnvSecrets = %v, want %v (longest first, deduplicated, short values skipped)", got, want)
	}
}

func TestIsSecretName(t *testing.T) {
	secret := []string{"OPENAI_API_KEY", "SOP_TOKEN", "AWS_SECRET_ACCESS_KEY", "DB_PASSWORD", "--api-key", "api.key", "SESSION_ID", "AUTHORIZATION", "SSH_AUTH_SOCK", "MYKEY"}
	// Over-redacting these is the accepted cost of never missing, say, MYKEY.
	notSecret := []string{"PATH", "HOME", "AWS_REGION", "SOP_LOG", "BRANCH_NAME"}
	for _, name := range secret {
		if !IsSecretName(name) {
			t.Errorf("IsSecretName(%q) = false, want true", name)
		}
	}
	for _, name := range notSecret {
		if IsSecretName(name) {
			t.Errorf("IsSecretName(%q) = true, want false", name)
		}
	}
}
