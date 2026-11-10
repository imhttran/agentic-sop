package sensitivepath

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// An override that walks "link/.." must name the directory the kernel opens
// (the link target's parent), not the one lexical cleaning would produce.
func TestEnvCredentialPathsResolveSymlinkBeforeDotDot(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "private", "account", "child")
	config := filepath.Join(base, "private", "account", "config")
	store := filepath.Join(base, "store")
	for _, d := range []string{child, config, store} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(config, "kube"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(child, filepath.Join(store, "link")); err != nil {
		t.Fatal(err)
	}
	viaLink := filepath.Join(store, "link") + "/../config"
	env := map[string]string{"GH_CONFIG_DIR": viaLink, "KUBECONFIG": viaLink + "/kube"}
	getenv := func(k string) string { return env[k] }

	wantDir, err := filepath.EvalSymlinks(config)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := EnvCredentialDirs(getenv)
	if err != nil || !reflect.DeepEqual(dirs, []string{wantDir}) {
		t.Fatalf("dirs = %v, %v; want [%s]", dirs, err, wantDir)
	}
	files, err := EnvCredentialFiles(getenv)
	if wantFile := filepath.Join(wantDir, "kube"); err != nil || !reflect.DeepEqual(files, []string{wantFile}) {
		t.Fatalf("files = %v, %v; want [%s]", files, err, wantFile)
	}
}

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

// TestEnvCredentialPaths resolves the credential locations named by the
// supported environment variables, including a KUBECONFIG list, and rejects a
// relative value.
func TestEnvCredentialPaths(t *testing.T) {
	env := map[string]string{
		"GNUPGHOME":                   "/env/gnupg",
		"CLOUDSDK_CONFIG":             "/env/gcloud",
		"GH_CONFIG_DIR":               "/env/gh",
		"AZURE_CONFIG_DIR":            "/env/azure",
		"XDG_CONFIG_HOME":             "/env/xdg",
		"DOCKER_CONFIG":               "/env/docker",
		"CARGO_HOME":                  "/env/cargo",
		"NPM_CONFIG_USERCONFIG":       "/env/npmrc",
		"NETRC":                       "/env/netrc",
		"AWS_SHARED_CREDENTIALS_FILE": "/env/aws-creds",
		"AWS_CONFIG_FILE":             "/env/aws-config",
		"KUBECONFIG":                  "/env/kube1" + string(os.PathListSeparator) + "/env/kube2",
	}
	getenv := func(k string) string { return env[k] }
	dirs, err := EnvCredentialDirs(getenv)
	if err != nil {
		t.Fatal(err)
	}
	wantDirs := []string{"/env/gnupg", "/env/gcloud", "/env/gh", "/env/azure", "/env/xdg/gh", "/env/xdg/gcloud"}
	if !reflect.DeepEqual(dirs, wantDirs) {
		t.Fatalf("dirs = %v, want %v", dirs, wantDirs)
	}
	files, err := EnvCredentialFiles(getenv)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{"/env/aws-config", "/env/aws-creds", "/env/cargo/credentials", "/env/cargo/credentials.toml", "/env/docker/config.json", "/env/kube1", "/env/kube2", "/env/netrc", "/env/npmrc", "/env/xdg/git/credentials"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("files = %v, want %v", files, wantFiles)
	}

	bad := func(string) string { return "relative/path" }
	if _, err := EnvCredentialDirs(bad); err == nil {
		t.Fatal("relative GNUPGHOME was accepted")
	}
	if _, err := EnvCredentialFiles(bad); err == nil {
		t.Fatal("relative KUBECONFIG was accepted")
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
