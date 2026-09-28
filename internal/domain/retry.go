package domain

// RetryPolicy bounds how many times a task may be retried. Only the attempt bound
// is used; the backoff/delay machinery was removed as unused (a requeue does not
// wait between attempts).
type RetryPolicy struct {
	MaxAttempts int
}

func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{MaxAttempts: 3}
}
