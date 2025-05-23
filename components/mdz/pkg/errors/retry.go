package errors

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	Multiplier     float64
	OnRetry        func(attempt int, delay time.Duration, err error)
	RetryCondition func(error) bool
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		OnRetry:      nil,
		RetryCondition: func(err error) bool {
			return IsRetryable(err)
		},
	}
}

// RetryableFunc represents a function that can be retried
type RetryableFunc func(ctx context.Context) error

// Retry executes a function with retry logic
func Retry(ctx context.Context, fn RetryableFunc, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Execute the function
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !config.RetryCondition(err) {
			return err
		}

		// Check if this is the last attempt
		if attempt == config.MaxAttempts {
			break
		}

		// Get retry delay from error if available
		if retryDelay := GetRetryDelay(err); retryDelay > 0 {
			delay = retryDelay
		}

		// Call retry callback if provided
		if config.OnRetry != nil {
			config.OnRetry(attempt, delay, err)
		}

		// Wait before retrying
		select {
		case <-time.After(delay):
			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		case <-ctx.Done():
			return Wrap(ctx.Err(), ErrorTypeTimeout, "retry cancelled")
		}
	}

	// Wrap the final error with retry information
	return Wrap(lastErr, ErrorTypeInternal, fmt.Sprintf("failed after %d attempts", config.MaxAttempts))
}

// RetryWithRollback executes a function with retry logic and rollback on failure
func RetryWithRollback(ctx context.Context, fn RetryableFunc, rollback func(context.Context) error, config *RetryConfig) error {
	err := Retry(ctx, fn, config)
	if err != nil && rollback != nil {
		// Attempt rollback
		if rbErr := rollback(ctx); rbErr != nil {
			// Combine both errors
			return Wrap(err, ErrorTypeInternal, fmt.Sprintf("operation failed and rollback failed: %v", rbErr))
		}
	}
	return err
}

// Transaction represents a transactional operation with rollback capability
type Transaction struct {
	steps     []TransactionStep
	completed []int
}

// TransactionStep represents a single step in a transaction
type TransactionStep struct {
	Name     string
	Execute  func(context.Context) error
	Rollback func(context.Context) error
}

// NewTransaction creates a new transaction
func NewTransaction() *Transaction {
	return &Transaction{
		steps:     make([]TransactionStep, 0),
		completed: make([]int, 0),
	}
}

// AddStep adds a step to the transaction
func (t *Transaction) AddStep(step TransactionStep) *Transaction {
	t.steps = append(t.steps, step)
	return t
}

// Execute runs all transaction steps with automatic rollback on failure
func (t *Transaction) Execute(ctx context.Context) error {
	for i, step := range t.steps {
		if err := step.Execute(ctx); err != nil {
			// Rollback completed steps in reverse order
			rbErr := t.rollback(ctx)
			if rbErr != nil {
				return Wrap(err, ErrorTypeInternal,
					fmt.Sprintf("step '%s' failed and rollback failed: %v", step.Name, rbErr))
			}
			return Wrap(err, ErrorTypeInternal, fmt.Sprintf("step '%s' failed", step.Name))
		}
		t.completed = append(t.completed, i)
	}
	return nil
}

// rollback reverses completed steps
func (t *Transaction) rollback(ctx context.Context) error {
	var errs []error

	// Rollback in reverse order
	for i := len(t.completed) - 1; i >= 0; i-- {
		stepIdx := t.completed[i]
		step := t.steps[stepIdx]

		if step.Rollback != nil {
			if err := step.Rollback(ctx); err != nil {
				errs = append(errs, fmt.Errorf("rollback '%s' failed: %w", step.Name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %v", errs)
	}

	return nil
}
