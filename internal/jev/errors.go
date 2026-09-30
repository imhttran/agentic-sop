package jev

import (
	"errors"
	"fmt"
)

var (
	errPassWithFindings        = errors.New("jev: result status PASS must not carry findings")
	errFindingsWithoutFindings = errors.New("jev: result status FINDINGS must carry at least one finding")

	errEvidencePassWithItems        = errors.New("jev: evidence status PASS must not carry items")
	errEvidenceFindingsWithoutItems = errors.New("jev: evidence status FINDINGS must carry at least one item")
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

// invalidVersionError names an unsupported evidence schema version.
type invalidVersionError struct{ version int }

func (e invalidVersionError) Error() string {
	return fmt.Sprintf("jev: evidence has unsupported version %d", e.version)
}

func errInvalidVersion(version int) error { return invalidVersionError{version: version} }

// invalidPurposeError names an unrecognized or empty purpose.
type invalidPurposeError struct{ purpose string }

func (e invalidPurposeError) Error() string {
	if e.purpose == "" {
		return "jev: evidence has empty purpose"
	}
	return "jev: evidence has invalid purpose " + e.purpose
}

func errInvalidPurpose(purpose string) error { return invalidPurposeError{purpose: purpose} }

// invalidEvidenceSeverityError names an unrecognized or empty evidence severity.
type invalidEvidenceSeverityError struct{ severity string }

func (e invalidEvidenceSeverityError) Error() string {
	if e.severity == "" {
		return "jev: evidence has empty severity"
	}
	return "jev: evidence has invalid severity " + e.severity
}

func errInvalidEvidenceSeverity(severity string) error {
	return invalidEvidenceSeverityError{severity: severity}
}

// invalidEvidenceStatusError names an unrecognized or empty evidence status.
type invalidEvidenceStatusError struct{ status string }

func (e invalidEvidenceStatusError) Error() string {
	if e.status == "" {
		return "jev: evidence has empty status"
	}
	return "jev: evidence has invalid status " + e.status
}

func errInvalidEvidenceStatus(status string) error {
	return invalidEvidenceStatusError{status: status}
}

// invalidCategoryError names an unrecognized evidence finding category.
type invalidCategoryError struct{ category string }

func (e invalidCategoryError) Error() string {
	return "jev: evidence has invalid category " + e.category
}

func errInvalidCategory(category string) error { return invalidCategoryError{category: category} }

// invalidConfidenceError names an out-of-range confidence.
type invalidConfidenceError struct{ confidence float64 }

func (e invalidConfidenceError) Error() string {
	return fmt.Sprintf("jev: evidence confidence %v is out of range (want 0.0 <= c <= 1.0)", e.confidence)
}

func errInvalidConfidence(confidence float64) error {
	return invalidConfidenceError{confidence: confidence}
}
