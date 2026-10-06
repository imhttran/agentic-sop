// Package caller holds the immutable Total acceptance tests for Workload B.
package caller

import "testing"

// FROZEN IMMUTABLE acceptance tests for caller.Total. Never changed between
// repetitions; no test tampering.
func TestTotal(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{4, 5, 9},
		{-4, 2, -2},
	}
	for _, c := range cases {
		if got := Total(c.a, c.b); got != c.want {
			t.Errorf("Total(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
