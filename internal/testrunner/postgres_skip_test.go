package testrunner

import "testing"

func TestParseGoTestExecuted(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"one passing test", `{"Action":"pass","Package":"pkg","Test":"TestA"}
{"Action":"pass","Package":"pkg","Test":""}`, true},
		{"multi-package pass", `{"Action":"pass","Package":"p1","Test":"TestA"}
{"Action":"pass","Package":"p1","Test":""}
{"Action":"pass","Package":"p2","Test":"TestB"}
{"Action":"pass","Package":"p2","Test":""}`, true},
		{"all skipped", `{"Action":"skip","Package":"pkg","Test":"TestA"}
{"Action":"skip","Package":"pkg","Test":"TestB"}
{"Action":"pass","Package":"pkg","Test":""}`, false},
		{"package-only ok", `{"Action":"output","Package":"pkg","Test":""}
{"Action":"pass","Package":"pkg","Test":""}`, false},
		{"one failed", `{"Action":"pass","Package":"pkg","Test":"TestA"}
{"Action":"fail","Package":"pkg","Test":"TestB"}
{"Action":"pass","Package":"pkg","Test":""}`, false},
		{"mixed pass and skip", `{"Action":"pass","Package":"pkg","Test":"TestA"}
{"Action":"skip","Package":"pkg","Test":"TestB"}
{"Action":"pass","Package":"pkg","Test":""}`, true},
		{"truncated (no completion)", `{"Action":"pass","Package":"pkg","Test":"TestA"}`, false},
		{"malformed non-json", "ok  pkg 0.1s", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := parseGoTestExecuted(tc.stdout)
			if got != tc.want {
				t.Fatalf("parseGoTestExecuted = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPostgresCheckAllSkippedIsNotPass(t *testing.T) {
	// A PostgresIntegration check that exits 0 but skipped every test must not be
	// classified Pass. Run the real runner on a command that emits skip-only JSON.
	c := Check{Category: PostgresIntegration, Command: `printf '{"Action":"skip","Package":"pkg","Test":"TestA"}\n{"Action":"pass","Package":"pkg","Test":""}\n'`}
	res := (&Runner{Dir: "."}).Run(t.Context(), c)
	if res.ExitCode == 0 && res.Status == Pass {
		t.Fatalf("all-skipped postgres check classified PASS (exit 0); status=%s", res.Status)
	}
}

// TestPostgresCompletionSurvivesCaptureBound proves the bounded capture keeps a
// trailing go test -json package-completion event even when a passing check emits
// far more than the capture bound before it.
func TestPostgresCompletionSurvivesCaptureBound(t *testing.T) {
	command := `printf '{"Action":"pass","Package":"pkg","Test":"TestA"}\n'; printf '%02000d' 0; printf '\n{"Action":"pass","Package":"pkg","Test":""}\n'`
	res := (&Runner{Dir: t.TempDir(), MaxOutputBytes: 512}).Run(t.Context(), Check{Category: PostgresIntegration, Command: command})
	if res.Status != Pass {
		t.Fatalf("status = %s, want PASS (stderr=%q stdout_tail=%q)", res.Status, res.Stderr, tail(res.Stdout, 200))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
