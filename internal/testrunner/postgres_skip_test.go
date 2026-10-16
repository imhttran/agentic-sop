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
