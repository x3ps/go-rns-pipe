package rnspipe

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func newTestReconnector(base time.Duration) *reconnector {
	return &reconnector{
		baseDelay:          base,
		exponentialBackoff: true,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestBackoffFixedDelay(t *testing.T) {
	r := &reconnector{
		baseDelay:          5 * time.Second,
		exponentialBackoff: false,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if got := r.backoff(attempt); got != 5*time.Second {
			t.Errorf("backoff(%d) = %v, want 5s", attempt, got)
		}
	}
}

func TestBackoffFirstAttemptZero(t *testing.T) {
	r := newTestReconnector(5 * time.Second)
	if got := r.backoff(0); got != 0 {
		t.Errorf("backoff(0) = %v, want 0", got)
	}
}

func TestBackoffNeverNegative(t *testing.T) {
	r := newTestReconnector(5 * time.Second)
	for attempt := 0; attempt <= 63; attempt++ {
		if got := r.backoff(attempt); got < 0 {
			t.Errorf("backoff(%d) = %v, want >= 0", attempt, got)
		}
	}
}

func TestBackoffLargeAttemptsCapped(t *testing.T) {
	r := newTestReconnector(5 * time.Second)
	const maxDelay = 60 * time.Second
	limit := time.Duration(float64(maxDelay) * 1.25)
	for _, attempt := range []int{50, 100, 500, 1000, 1074, 1075, 2000} {
		got := r.backoff(attempt)
		if got < 0 {
			t.Errorf("backoff(%d) = %v, want >= 0", attempt, got)
		}
		if got > limit {
			t.Errorf("backoff(%d) = %v, want <= %v", attempt, got, limit)
		}
	}
}

func TestBackoffProgression(t *testing.T) {
	const base = 5 * time.Second
	const maxDelay = 60 * time.Second
	r := newTestReconnector(base)
	for attempt := 1; attempt <= 5; attempt++ {
		cappedF := float64(base) * float64(int64(1)<<(attempt-1))
		if cappedF > float64(maxDelay) {
			cappedF = float64(maxDelay)
		}
		lo := time.Duration(cappedF * 0.75)
		hi := time.Duration(cappedF * 1.25)
		for trial := 0; trial < 50; trial++ {
			got := r.backoff(attempt)
			if got < lo || got > hi {
				t.Errorf("backoff(%d) = %v, want in [%v, %v]", attempt, got, lo, hi)
				break
			}
		}
	}
}
