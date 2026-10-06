// Package sum is the frozen Workload B production placeholder for the sum package.
//
// FROZEN INITIAL FIXTURE: Add is a placeholder returning 0 so the immutable
// acceptance tests fail, confirming the intended missing implementation.
package sum

// Add returns the sum of a and b.
//
// PLACEHOLDER: returns 0 so the frozen acceptance tests fail before B001.
func Add(a, b int) int {
	return 0
}
