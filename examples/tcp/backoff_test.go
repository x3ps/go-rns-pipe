package main

import (
	"testing"
	"time"
)

func TestBackoffFirstAttemptZero(t *testing.T) {
	if got := backoff(5*time.Second, 0); got != 0 {
		t.Errorf("backoff(5s, 0) = %v, want 0", got)
	}
}

func TestBackoffNeverNegative(t *testing.T) {
	for attempt := 0; attempt <= 63; attempt++ {
		if got := backoff(5*time.Second, attempt); got < 0 {
			t.Errorf("backoff(5s, %d) = %v, want >= 0", attempt, got)
		}
	}
}

// TestBackoffFixedDelay verifies that all non-zero attempts return exactly base,
// matching TCPInterface.py RECONNECT_WAIT = 5 (fixed delay, no exponential growth).
func TestBackoffFixedDelay(t *testing.T) {
	const base = 5 * time.Second
	for attempt := 1; attempt <= 5; attempt++ {
		if got := backoff(base, attempt); got != base {
			t.Errorf("backoff(%v, %d) = %v, want %v", base, attempt, got, base)
		}
	}
}
