package jev

import "errors"

var (
	errPassWithFindings        = errors.New("jev: result status PASS must not carry findings")
	errFindingsWithoutFindings = errors.New("jev: result status FINDINGS must carry at least one finding")
)

// invalidStatusError names an unrecognized status.
type invalidStatusError struct{ status string }

func (e invalidStatusError) Error() string {
	if e.status == "" {
		return "jev: result has empty status"
	}
	return "jev: result has invalid status " + e.status
}

func errInvalidStatus(status string) error { return invalidStatusError{status: status} }

// invalidSeverityError names an unrecognized finding severity.
type invalidSeverityError struct{ severity string }

func (e invalidSeverityError) Error() string {
	if e.severity == "" {
		return "jev: finding has empty severity"
	}
	return "jev: finding has invalid severity " + e.severity
}

func errInvalidSeverity(severity string) error { return invalidSeverityError{severity: severity} }
