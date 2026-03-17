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

func TestBackoffLargeAttemptsCapped(t *testing.T) {
	const maxDelay = 60 * time.Second
	limit := time.Duration(float64(maxDelay) * 1.25)
	for _, attempt := range []int{50, 100, 500, 1000, 1074, 1075, 2000} {
		got := backoff(5*time.Second, attempt)
		if got < 0 {
			t.Errorf("backoff(5s, %d) = %v, want >= 0", attempt, got)
		}
		if got > limit {
			t.Errorf("backoff(5s, %d) = %v, want <= %v", attempt, got, limit)
		}
	}
}

func TestBackoffProgression(t *testing.T) {
	const base = 5 * time.Second
	const maxDelay = 60 * time.Second
	for attempt := 1; attempt <= 5; attempt++ {
		cappedF := float64(base) * float64(int64(1)<<(attempt-1))
		if cappedF > float64(maxDelay) {
			cappedF = float64(maxDelay)
		}
		lo := time.Duration(cappedF * 0.75)
		hi := time.Duration(cappedF * 1.25)
		for trial := 0; trial < 50; trial++ {
			got := backoff(base, attempt)
			if got < lo || got > hi {
				t.Errorf("backoff(5s, %d) = %v, want in [%v, %v]", attempt, got, lo, hi)
				break
			}
		}
	}
}
