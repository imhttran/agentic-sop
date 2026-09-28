package domain

import "testing"

func TestVerifyFirst(t *testing.T) {
	cases := []struct {
		mode ExecutionMode
		want bool
	}{
		{"", false},
		{ExecutionImplement, false},
		{ExecutionVerifyFirst, true},
		{"verify_first", false}, // only the canonical value; parsing normalizes
	}
	for _, tc := range cases {
		if got := tc.mode.VerifyFirst(); got != tc.want {
			t.Errorf("%q.VerifyFirst() = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestKnownExecutionMode(t *testing.T) {
	for _, ok := range []ExecutionMode{"", ExecutionImplement, ExecutionVerifyFirst} {
		if !KnownExecutionMode(ok) {
			t.Errorf("KnownExecutionMode(%q) = false, want true", ok)
		}
	}
	if KnownExecutionMode("verify") {
		t.Error("KnownExecutionMode(\"verify\") = true, want false")
	}
}

func TestParseExecutionMode(t *testing.T) {
	cases := []struct {
		in    string
		want  ExecutionMode
		known bool
	}{
		{"verify-first", ExecutionVerifyFirst, true},
		{"Verify First", ExecutionVerifyFirst, true},
		{"verify_first", ExecutionVerifyFirst, true},
		{"  VERIFY-FIRST  ", ExecutionVerifyFirst, true},
		{"implement", ExecutionImplement, true},
		{"", "", false},
		{"run the tests", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseExecutionMode(tc.in)
		if ok != tc.known || got != tc.want {
			t.Errorf("ParseExecutionMode(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.known)
		}
	}
}
