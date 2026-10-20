package projectdetect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// installFakeTools writes one fake executable per tool. Each fake records its
// working directory and arguments, then succeeds. The test never runs a real
// toolchain, so it is deterministic and independent of the host's languages.
func installFakeTools(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"cargo", "go", "gofmt", "npm", "pnpm", "yarn", "bun"} {
		script := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$(basename \"$0\")\" \"$PWD\" \"$*\" >> \"$DETECT_LOG\"\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// runGenerated executes every proposed validation command from the repository
// root, exactly as SOP does, and returns the recorded tool invocations.
func runGenerated(t *testing.T, root, logPath string, validation Validation) []invocation {
	t.Helper()
	bin := filepath.Join(root, "bin")
	commands := append(append(append([]string{}, validation.Build...), validation.Test...), validation.Lint...)
	if len(commands) == 0 {
		t.Fatal("no validation commands were generated")
	}
	for _, command := range commands {
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"DETECT_LOG="+logPath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %q failed: %v\n%s", command, err, out)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tool log: %v", err)
	}
	var out []invocation
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("malformed tool log line %q", line)
		}
		out = append(out, invocation{tool: parts[0], cwd: parts[1], args: parts[2]})
	}
	return out
}

type invocation struct {
	tool string
	cwd  string
	args string
}

func TestGeneratedNestedCommandsRunFromManifestDirectory(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		wantDir   string
		wantTools []string
	}{
		{
			name: "rust without makefile",
			files: map[string]string{
				"crates/app/Cargo.toml": "[package]\nname = \"app\"\n",
			},
			wantDir:   "crates/app",
			wantTools: []string{"cargo", "cargo", "cargo", "cargo"},
		},
		{
			name: "go without makefile",
			files: map[string]string{
				"services/api/go.mod": "module example.com/api\n\ngo 1.27\n",
			},
			wantDir:   "services/api",
			wantTools: []string{"go", "go", "go", "gofmt"},
		},
		{
			name: "next without makefile",
			files: map[string]string{
				"web/package.json":   `{"scripts":{"build":"next build","e2e":"playwright test","typecheck":"tsc --noEmit"},"dependencies":{"next":"^15.0.0"}}`,
				"web/next.config.ts": "export default {}\n",
			},
			wantDir:   "web",
			wantTools: []string{"npm", "npm", "npm"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tc.files {
				write(t, root, rel, content)
			}
			installFakeTools(t, root)
			d := Detect(root)
			logPath := filepath.Join(root, "tools.log")
			got := runGenerated(t, root, logPath, d.Validation)
			if len(got) != len(tc.wantTools) {
				t.Fatalf("tool invocations = %d, want %d: %+v", len(got), len(tc.wantTools), got)
			}
			wantDir := filepath.Join(root, filepath.FromSlash(tc.wantDir))
			for i, inv := range got {
				if inv.tool != tc.wantTools[i] {
					t.Errorf("invocation %d tool = %q, want %q", i, inv.tool, tc.wantTools[i])
				}
				if inv.cwd != wantDir {
					t.Errorf("invocation %d cwd = %q, want %q (args %q)", i, inv.cwd, wantDir, inv.args)
				}
			}
		})
	}
}

func TestGeneratedRootCommandsRemainUnchangedAndRunAtRoot(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/root\n\ngo 1.27\n")
	installFakeTools(t, root)
	d := Detect(root)
	want := Validation{
		Build: []string{"go build ./..."},
		Test:  []string{"go test ./..."},
		Lint:  []string{"go vet ./...", "gofmt -l ."},
	}
	if d.Validation.Build[0] != want.Build[0] || d.Validation.Test[0] != want.Test[0] ||
		d.Validation.Lint[0] != want.Lint[0] || d.Validation.Lint[1] != want.Lint[1] {
		t.Fatalf("root validation = %+v, want the established commands %+v", d.Validation, want)
	}
	got := runGenerated(t, root, filepath.Join(root, "tools.log"), d.Validation)
	for _, inv := range got {
		if inv.cwd != root {
			t.Errorf("root command ran in %q, want %q", inv.cwd, root)
		}
	}
}

func TestGeneratedMixedRootAndNestedCommandsUseBothDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[package]\nname = \"root\"\n")
	write(t, root, "web/package.json", `{"scripts":{"build":"next build"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, root, "web/next.config.ts", "export default {}\n")
	installFakeTools(t, root)
	d := Detect(root)
	got := runGenerated(t, root, filepath.Join(root, "tools.log"), d.Validation)

	rustDir := root
	webDir := filepath.Join(root, "web")
	sawRust, sawWeb := false, false
	for _, inv := range got {
		switch inv.tool {
		case "cargo":
			sawRust = true
			if inv.cwd != rustDir {
				t.Errorf("cargo ran in %q, want %q", inv.cwd, rustDir)
			}
		case "npm":
			sawWeb = true
			if inv.cwd != webDir {
				t.Errorf("npm ran in %q, want %q", inv.cwd, webDir)
			}
		}
	}
	if !sawRust || !sawWeb {
		t.Errorf("mixed run did not exercise both directories: rust=%t web=%t", sawRust, sawWeb)
	}
}
