package perf

import "io"

// WriteJEVDuration renders a JEV duration in milliseconds using the package's
// single timing vocabulary (see humanMS), so a JEV metric reads like every other
// SOP timing. It is diagnostic rendering only: it never asserts PASS or FAIL.
func WriteJEVDuration(w io.Writer, ms int64) {
	_, _ = io.WriteString(w, humanMS(ms))
}
