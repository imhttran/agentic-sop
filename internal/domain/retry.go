package domain

import (
	"math"
	"time"
)

type RetryPolicy struct {
	MaxAttempts   int
	BackoffFactor float64
	MaxDelay      time.Duration
}

func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:   3,
		BackoffFactor: 2.0,
		MaxDelay:      5 * time.Minute,
	}
}

func (p *RetryPolicy) IsRetryable(task *Task) bool {
	return task.Attempt < p.MaxAttempts
}

func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	baseDelay := time.Second
	delay := time.Duration(float64(baseDelay) * math.Pow(p.BackoffFactor, float64(attempt-1)))

	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	return delay
}
