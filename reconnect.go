package rnspipe

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// reconnector manages reconnection attempts with configurable backoff.
// See: PipeInterface.py#L145-L156 — reconnect_pipe loop with respawn_delay
type reconnector struct {
	baseDelay          time.Duration
	maxAttempts        int // 0 = infinite
	exponentialBackoff bool
	logger             *slog.Logger
}

// run executes fn in a loop until fn returns nil (context cancelled) or ctx is
// cancelled externally. It applies exponential backoff with jitter between
// attempts, capped at 60s. The first attempt has no delay.
func (r *reconnector) run(ctx context.Context, fn func() error) error {
	attempt := 0
	for {
		if r.maxAttempts > 0 && attempt > r.maxAttempts {
			r.logger.Error("max reconnect attempts reached", "attempts", attempt)
			return ErrMaxReconnectAttemptsReached
		}

		delay := r.backoff(attempt)
		if attempt > 0 {
			r.logger.Info("attempting reconnect", "attempt", attempt, "delay", delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		if err := fn(); err != nil {
			if errors.Is(err, ErrPipeClosed) {
				return err // terminal: don't retry
			}
			r.logger.Warn("reconnect failed", "attempt", attempt+1, "error", err)
			attempt++
			continue
		}

		// fn returned nil — context was cancelled inside fn.
		return ctx.Err()
	}
}

// backoff computes the delay for a given attempt. The first attempt (0) has no
// delay. When exponentialBackoff is false (default), returns a fixed baseDelay
// matching PipeInterface.py respawn_delay behavior. When exponentialBackoff is
// true, uses exponential backoff with ±25% jitter capped at 60 seconds.
func (r *reconnector) backoff(attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}
	if !r.exponentialBackoff {
		return r.baseDelay // Fixed delay: matches PipeInterface.py respawn_delay
	}
	const maxDelay = 60 * time.Second
	exp := math.Pow(2, float64(attempt-1))
	delayF := float64(r.baseDelay) * exp
	if delayF > float64(maxDelay) {
		delayF = float64(maxDelay)
	}
	// Add jitter: ±25%
	jitter := time.Duration(delayF * (0.75 + rand.Float64()*0.5))
	return jitter
}
