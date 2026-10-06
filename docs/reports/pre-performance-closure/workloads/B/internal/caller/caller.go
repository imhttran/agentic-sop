// Package caller is the frozen Workload B production placeholder for the caller
// package.
//
// FROZEN INITIAL FIXTURE: Total is a placeholder returning 0 so the immutable
// acceptance tests fail, confirming the intended missing implementation. Task B001
// must make Total forward to sum.Add, changing at least this file and
// internal/sum/sum.go.
package caller

// Total returns the total of a and b by forwarding to sum.Add.
//
// PLACEHOLDER: returns 0 so the frozen acceptance tests fail before B001.
func Total(a, b int) int {
	return 0
}
