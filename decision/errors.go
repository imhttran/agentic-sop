package decision

import "errors"

// ErrorKind classifies provider failures without leaking vendor details.
type ErrorKind string

const (
	// KindInvalidRequest indicates the caller supplied a malformed request.
	KindInvalidRequest ErrorKind = "invalid_request"
	// KindUnavailable indicates the provider/backend could not be reached.
	KindUnavailable ErrorKind = "unavailable"
	// KindMalformedResponse indicates the backend answered with something the
	// adapter could not normalize.
	KindMalformedResponse ErrorKind = "malformed_response"
	// KindProviderFailure indicates any other backend failure.
	KindProviderFailure ErrorKind = "provider_failure"
)

// Sentinel errors for use with errors.Is. A ProviderError reports membership of
// the matching sentinel via its Is method.
var (
	ErrInvalidRequest    = errors.New("invalid decision request")
	ErrUnavailable       = errors.New("provider unavailable")
	ErrMalformedResponse = errors.New("malformed provider response")
	ErrProviderFailure   = errors.New("provider failure")
)

// ProviderError is the normalized error returned by providers. It exposes a
// provider name and an ErrorKind while optionally wrapping the underlying
// cause, which callers can retrieve with errors.Unwrap.
type ProviderError struct {
	Provider string
	Kind     ErrorKind
	Err      error
}

// Error implements error.
func (e *ProviderError) Error() string {
	if e.Err == nil {
		return e.Provider + ": provider error (" + string(e.Kind) + ")"
	}
	return e.Provider + ": " + string(e.Kind) + ": " + e.Err.Error()
}

// Unwrap returns the wrapped cause, if any.
func (e *ProviderError) Unwrap() error { return e.Err }

// Is reports whether the error matches one of the package sentinels.
func (e *ProviderError) Is(target error) bool {
	switch target {
	case ErrInvalidRequest:
		return e.Kind == KindInvalidRequest
	case ErrUnavailable:
		return e.Kind == KindUnavailable
	case ErrMalformedResponse:
		return e.Kind == KindMalformedResponse
	case ErrProviderFailure:
		return e.Kind == KindProviderFailure
	default:
		return false
	}
}
