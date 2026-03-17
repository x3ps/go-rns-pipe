package rnspipe

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// reconnector manages reconnection attempts with exponential backoff and jitter.
// See: PipeInterface.py#L145-L156 — reconnect_pipe loop with respawn_delay
type reconnector struct {
	baseDelay   time.Duration
	maxAttempts int // 0 = infinite
	logger      *slog.Logger
	onStatus    func(online bool)
}

// run executes fn in a loop until it succeeds (returns nil) or ctx is cancelled.
// It applies exponential backoff with jitter between attempts, capped at 60s.
func (r *reconnector) run(ctx context.Context, fn func() error) error {
	attempt := 0
	for {
		if r.maxAttempts > 0 && attempt >= r.maxAttempts {
			r.logger.Error("max reconnect attempts reached", "attempts", attempt)
			return context.Canceled
		}

		delay := r.backoff(attempt)
		r.logger.Info("attempting reconnect", "attempt", attempt+1, "delay", delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		if err := fn(); err != nil {
			r.logger.Warn("reconnect failed", "attempt", attempt+1, "error", err)
			attempt++
			continue
		}

		r.logger.Info("reconnected successfully", "attempt", attempt+1)
		if r.onStatus != nil {
			r.onStatus(true)
		}
		return nil
	}
}

// backoff computes the delay for a given attempt using exponential backoff with
// jitter. The delay is capped at 60 seconds.
func (r *reconnector) backoff(attempt int) time.Duration {
	maxDelay := 60 * time.Second
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(r.baseDelay) * exp)
	if delay > maxDelay {
		delay = maxDelay
	}
	// Add jitter: ±25%
	jitter := time.Duration(float64(delay) * (0.75 + rand.Float64()*0.5))
	return jitter
}
