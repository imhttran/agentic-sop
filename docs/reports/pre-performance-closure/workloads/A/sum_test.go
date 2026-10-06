package sum

import "testing"

// FROZEN IMMUTABLE acceptance tests for Workload A. Never changed between
// repetitions; no test tampering.
func TestAdd(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{1, 2, 3},
		{-2, 5, 3},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := Add(c.a, c.b); got != c.want {
			t.Errorf("Add(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
