// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package resilience

import (
	"context"
	"errors"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/sony/gobreaker"
)

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Name          string
	MaxRequests   uint32        // max requests in half-open state
	Interval      time.Duration // cyclic period for clearing counts
	Timeout       time.Duration // period of open state before half-open
	FailureThresh uint32        // consecutive failures before open
	FailureRatio  float64       // failure ratio threshold (0.0-1.0), 0 to disable
	MinRequests   uint32        // minimum requests before failure ratio is evaluated
}

// DefaultConfig returns default circuit breaker configuration
func DefaultConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:          name,
		MaxRequests:   3,
		Interval:      60 * time.Second,
		Timeout:       30 * time.Second,
		FailureThresh: 5,
		FailureRatio:  0.5,
		MinRequests:   10,
	}
}

// CircuitBreaker wraps sony/gobreaker with logging
type CircuitBreaker struct {
	cb     *gobreaker.CircuitBreaker
	logger libLog.Logger
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(cfg CircuitBreakerConfig, logger libLog.Logger) *CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		IsSuccessful: func(err error) bool {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return true
			}

			return err == nil
		},
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip on consecutive failures
			if counts.ConsecutiveFailures >= cfg.FailureThresh {
				return true
			}
			// Trip on failure ratio if enabled and minimum requests met
			// Guard against division by zero
			if cfg.FailureRatio > 0 && cfg.MinRequests > 0 && counts.Requests >= cfg.MinRequests {
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)

				return failureRatio >= cfg.FailureRatio
			}

			return false
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.With(
				libLog.String("operation", "circuit_breaker.state_change"),
				libLog.String("circuit_breaker.name", name),
				libLog.String("circuit_breaker.state_from", from.String()),
				libLog.String("circuit_breaker.state_to", to.String()),
			).Log(context.Background(), libLog.LevelInfo, "Circuit breaker state changed")
		},
	}

	return &CircuitBreaker{
		cb:     gobreaker.NewCircuitBreaker(settings),
		logger: logger,
	}
}

// Execute runs the given function with circuit breaker protection.
// Context cancellation is handled separately from circuit breaker failure counting
// to prevent client-side timeouts from being counted as service failures.
// Note: When context is cancelled, Execute returns immediately with ctx.Err(),
// but fn() continues to run until completion in the background. The inner goroutine
// will terminate after fn() completes (no permanent leak), but fn itself is NOT cancelled.
// Callers requiring cancellable operations should ensure fn respects ctx internally.
func (c *CircuitBreaker) Execute(ctx context.Context, fn func() (any, error)) (any, error) {
	// Check if context is already cancelled before invoking the circuit breaker
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type result struct {
		val any
		err error
	}

	// Buffered channel ensures the goroutine can always send its result without blocking,
	// even if the caller returns early due to context cancellation.
	done := make(chan result, 1)

	// Execute fn through circuit breaker in a goroutine with cancellable wrapper.
	// The wrapper ensures fn runs in its own temporary goroutine and respects context cancellation.
	go func() {
		// Create a cancellable wrapper that respects ctx.Done()
		wrappedFn := func() (any, error) {
			// Buffered channel ensures the inner goroutine can always complete its send
			// even if we return early due to context cancellation, preventing goroutine leaks.
			fnDone := make(chan result, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						// Use select with default to avoid blocking if channel is full
						// (shouldn't happen with buffer of 1, but defensive)
						select {
						case fnDone <- result{nil, fmt.Errorf("panic recovered in circuit breaker: %v", r)}:
						default:
							c.logger.With(
								libLog.String("operation", "circuit_breaker.execute"),
								libLog.String("circuit_breaker.name", c.cb.Name()),
								libLog.String("warning", "panic_result_dropped"),
								libLog.String("panic", fmt.Sprintf("%v", r)),
							).Log(ctx, libLog.LevelWarn, "Panic result dropped - fnDone channel unexpectedly full")
						}
					}
				}()

				val, err := fn()
				// Non-blocking send: buffer ensures this won't block even if context is cancelled
				select {
				case fnDone <- result{val, err}:
				default:
					// Channel full (shouldn't happen with buffer of 1), log for debugging
					c.logger.With(
						libLog.String("operation", "circuit_breaker.execute"),
						libLog.String("circuit_breaker.name", c.cb.Name()),
						libLog.String("warning", "result_dropped"),
						libLog.Bool("has_error", err != nil),
					).Log(ctx, libLog.LevelWarn, "Result dropped - fnDone channel unexpectedly full")
				}
			}()

			// Select between context cancellation and fn completion
			select {
			case <-ctx.Done():
				// Context was cancelled; return immediately without waiting for fn
				// The goroutine running fn() will complete and send to fnDone,
				// but the buffered channel ensures it won't block.
				c.logger.With(
					libLog.String("operation", "circuit_breaker.execute"),
					libLog.String("circuit_breaker.name", c.cb.Name()),
					libLog.String("warning", "operation_orphaned"),
				).Log(ctx, libLog.LevelWarn, "Operation orphaned due to context cancellation - fn() still running in background")

				return nil, ctx.Err()
			case res := <-fnDone:
				// fn completed normally
				return res.val, res.err
			}
		}

		val, err := c.cb.Execute(wrappedFn)

		// Treat context cancellation errors as non-counting failures.
		// If the wrapped function returned a context error, do not let it propagate
		// as a circuit breaker failure; instead, return it without affecting metrics.
		// Note: Context errors are already marked as "successful" in IsSuccessful settings,
		// but we still short-circuit here to skip result handling and return immediately.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			done <- result{nil, err}
			return
		}

		done <- result{val, err}
	}()

	select {
	case <-ctx.Done():
		// Outer context cancellation; return immediately.
		// The goroutine will complete eventually and send to 'done',
		// but the buffered channel ensures it won't block.
		return nil, ctx.Err()
	case res := <-done:
		return res.val, res.err
	}
}

// State returns the current state of the circuit breaker
func (c *CircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}

// IsOpen returns true if circuit breaker is open
func (c *CircuitBreaker) IsOpen() bool {
	return c.cb.State() == gobreaker.StateOpen
}
