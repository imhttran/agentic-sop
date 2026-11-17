package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// cargoAuditPlatformTargets maps each supported execution platform to the release
// asset it is provisioned from. Windows is deliberately absent: it has no
// committed pin and `run_command` is refused there, so the dependency gate is not
// supported on it. Adding a platform means adding an asset to the manifest, a pin
// to the config, and a row here.
var cargoAuditPlatformTargets = map[string]string{
	"linux/amd64":  "x86_64-unknown-linux-gnu",
	"linux/arm64":  "aarch64-unknown-linux-gnu",
	"darwin/amd64": "x86_64-apple-darwin",
	"darwin/arm64": "aarch64-apple-darwin",
}

type cargoAuditManifest struct {
	Version string `json:"version"`
	Assets  map[string]struct {
		SHA256 string `json:"sha256"`
	} `json:"assets"`
}

func loadCargoAuditManifest(t *testing.T) cargoAuditManifest {
	t.Helper()
	path := filepath.Join("..", "..", "scripts", "security", "cargo-audit-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m cargoAuditManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(m.Assets) == 0 {
		t.Fatalf("%s declares no assets; the test must not pass vacuously", path)
	}
	return m
}

// TestCargoAuditRepoPinsCoverEverySupportedPlatform keeps the three records in
// step: the platform matrix, the committed binary pins, and the archive pins the
// fetch script verifies before extracting. A platform that is pinned but has no
// archive pin cannot be provisioned; an archive pin with no binary pin would run
// unverified.
func TestCargoAuditRepoPinsCoverEverySupportedPlatform(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", ".agent-sdlc", "config.yaml"))
	if err != nil {
		t.Fatalf("load repo config: %v", err)
	}
	var adapter *config.SecurityAdapterConfig
	for i := range cfg.Security.Adapters {
		if cfg.Security.Adapters[i].ID == AdapterIDCargoAudit {
			adapter = &cfg.Security.Adapters[i]
			break
		}
	}
	if adapter == nil {
		t.Fatalf("repo config has no %s adapter", AdapterIDCargoAudit)
	}

	manifest := loadCargoAuditManifest(t)
	if adapter.Version != manifest.Version {
		t.Errorf("config version %q != manifest version %q", adapter.Version, manifest.Version)
	}
	if len(adapter.BinarySHA256ByPlatform) != len(cargoAuditPlatformTargets) {
		t.Errorf("config pins %d platform(s), want %d (one per supported platform)",
			len(adapter.BinarySHA256ByPlatform), len(cargoAuditPlatformTargets))
	}

	for platform, target := range cargoAuditPlatformTargets {
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			t.Fatalf("bad platform key %q", platform)
		}
		pin := adapter.PlatformBinarySHA256(goos, goarch)
		if pin == "" {
			t.Errorf("%s has no committed binary pin", platform)
		} else if !isLowerHexSHA256(pin) {
			t.Errorf("%s pin %q is not 64 lowercase hex characters", platform, pin)
		}
		asset, ok := manifest.Assets[target]
		if !ok {
			t.Errorf("manifest has no archive pin for target %q (%s)", target, platform)
			continue
		}
		if !isLowerHexSHA256(asset.SHA256) {
			t.Errorf("manifest archive pin for %q is not 64 lowercase hex characters: %q", target, asset.SHA256)
		}
	}

	// An unsupported platform must stay unpinned rather than silently borrowing
	// another platform's digest; the gate is not supported there.
	for _, platform := range []string{"windows/amd64", "windows/arm64"} {
		goos, goarch, _ := strings.Cut(platform, "/")
		if got := adapter.PlatformBinarySHA256(goos, goarch); got != "" {
			t.Errorf("%s resolves to pin %q, want none: the platform is unsupported", platform, got)
		}
	}
}

// TestCargoAuditRepoPinsAreDistinct guards against a copy-paste that pins one
// platform's digest for another: two different platforms cannot ship the same
// bytes, so identical pins across platforms indicate a mistake.
func TestCargoAuditRepoPinsAreDistinct(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", ".agent-sdlc", "config.yaml"))
	if err != nil {
		t.Fatalf("load repo config: %v", err)
	}
	seen := map[string]string{}
	for _, adapter := range cfg.Security.Adapters {
		if adapter.ID != AdapterIDCargoAudit {
			continue
		}
		for platform, pin := range adapter.BinarySHA256ByPlatform {
			if other, ok := seen[pin]; ok {
				t.Errorf("%s and %s share binary pin %s", other, platform, pin)
			}
			seen[pin] = platform
		}
	}
	if len(seen) == 0 {
		t.Fatal("no cargo-audit platform pins found")
	}
}

// isLowerHexSHA256 reports whether v is exactly 64 lowercase hex characters.
func isLowerHexSHA256(v string) bool {
	return len(v) == 64 && strings.Trim(v, "0123456789abcdef") == ""
}
