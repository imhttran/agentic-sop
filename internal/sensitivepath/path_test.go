package sensitivepath

import "testing"

// TestToolReadDenied is the approved narrow profile: well-known credential
// files/directories and key material are denied, the documented template files
// and ordinary source (including a generic secrets source directory) are not.
func TestToolReadDenied(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".env", true},
		{".env.local", true},
		{"config/.env", true},
		{".env.example", false},
		{".env.sample", false},
		{"docs/.env.example", false},
		{".npmrc", true},
		{".netrc", true},
		{".git-credentials", true},
		{"credentials.json", true},
		{"internal/credentials_helper.go", true},
		{"id_rsa", true},
		{"id_ed25519.pub", true},
		{".aws/credentials", true},
		{".ssh/id_rsa", true},
		{".kube/config", true},
		{".gnupg/key", true},
		{".gnupg/private-keys-v1.d/key", true},
		{"deploy/service.key", true},
		{"certs/ca.pem", true},
		{"internal/secrets/manager.go", false},
		{"secrets.yaml", false},
		{"notes.txt", false},
		{"internal/a.go", false},
		{"", false},
		{".", false},
	}
	for _, tc := range cases {
		if got := IsToolReadDenied(tc.path); got != tc.want {
			t.Errorf("IsToolReadDenied(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestReviewSensitivePreservesExistingPolicy is the compatibility guard for the
// review path: extracting the classifier must not change what review excludes.
func TestReviewSensitivePreservesExistingPolicy(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".env", true},
		{".env.example", true},
		{"config/.env", true},
		{"internal/a.go", false},
		{"internal/secrets/manager.go", true},
		{"secrets.yaml", true},
		{"secret.txt", true},
		{"notes.txt", false},
		{"deploy/service.key", true},
		{"certs/ca.pem", true},
		{".ssh/id_rsa", true},
	}
	for _, tc := range cases {
		if got := IsReviewSensitive(tc.path); got != tc.want {
			t.Errorf("IsReviewSensitive(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
