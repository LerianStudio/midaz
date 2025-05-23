package errors

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config == nil {
		t.Fatal("DefaultRetryConfig should not return nil")
	}

	if config.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts 3, got %d", config.MaxAttempts)
	}

	if config.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay 1s, got %v", config.InitialDelay)
	}

	if config.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay 30s, got %v", config.MaxDelay)
	}

	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %f", config.Multiplier)
	}

	if config.OnRetry != nil {
		t.Error("Expected OnRetry to be nil by default")
	}

	if config.RetryCondition == nil {
		t.Error("Expected RetryCondition to be set")
	}
}

func TestRetry_Success(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		return nil // Success on first try
	}

	config := DefaultRetryConfig()
	err := Retry(ctx, fn, config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected function to be called once, called %d times", callCount)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return New(ErrorTypeNetwork, "temporary error").WithRetry(1 * time.Second)
		}
		return nil // Success on third try
	}

	config := DefaultRetryConfig()
	config.InitialDelay = 1 * time.Millisecond // Speed up test

	err := Retry(ctx, fn, config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected function to be called 3 times, called %d times", callCount)
	}
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		return New(ErrorTypeNetwork, "persistent error").WithRetry(1 * time.Second)
	}

	config := DefaultRetryConfig()
	config.InitialDelay = 1 * time.Millisecond // Speed up test

	err := Retry(ctx, fn, config)

	if err == nil {
		t.Error("Expected error after max attempts exceeded")
	}

	if callCount != config.MaxAttempts {
		t.Errorf("Expected function to be called %d times, called %d times", config.MaxAttempts, callCount)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		return New(ErrorTypeValidation, "non-retryable error") // Without WithRetry()
	}

	config := DefaultRetryConfig()
	err := Retry(ctx, fn, config)

	if err == nil {
		t.Error("Expected error for non-retryable error")
	}

	if callCount != 1 {
		t.Errorf("Expected function to be called once for non-retryable error, called %d times", callCount)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		if callCount == 1 {
			cancel() // Cancel context on first call
		}
		return New(ErrorTypeNetwork, "error").WithRetry(1 * time.Second)
	}

	config := DefaultRetryConfig()
	config.InitialDelay = 1 * time.Millisecond

	err := Retry(ctx, fn, config)

	if err == nil {
		t.Error("Expected error when context is cancelled")
	}

	// Should stop retrying when context is cancelled
	if callCount > 2 {
		t.Errorf("Expected minimal calls after context cancellation, got %d", callCount)
	}
}

func TestRetry_NilConfig(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	fn := func(ctx context.Context) error {
		callCount++
		return nil
	}

	err := Retry(ctx, fn, nil) // nil config should use defaults

	if err != nil {
		t.Errorf("Expected no error with nil config, got %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected function to be called once, called %d times", callCount)
	}
}

func TestRetry_OnRetryCallback(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	retryCallbacks := 0

	fn := func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return New(ErrorTypeNetwork, "temporary error").WithRetry(1 * time.Second)
		}
		return nil
	}

	config := DefaultRetryConfig()
	config.InitialDelay = 1 * time.Millisecond
	config.OnRetry = func(attempt int, delay time.Duration, err error) {
		retryCallbacks++
	}

	err := Retry(ctx, fn, config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if retryCallbacks != 2 {
		t.Errorf("Expected 2 retry callbacks, got %d", retryCallbacks)
	}
}

func TestRetryWithRollback_Success(t *testing.T) {
	ctx := context.Background()

	txn := NewTransaction()

	step1Called := false
	step2Called := false

	step1 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step1Called = true
			return nil
		},
		Rollback: func(ctx context.Context) error {
			step1Called = false
			return nil
		},
	}

	step2 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step2Called = true
			return nil
		},
		Rollback: func(ctx context.Context) error {
			step2Called = false
			return nil
		},
	}

	txn.AddStep(step1)
	txn.AddStep(step2)

	config := DefaultRetryConfig()
	fn := func(ctx context.Context) error {
		return txn.Execute(ctx)
	}

	rollback := func(ctx context.Context) error {
		// Simple rollback function for testing
		return nil
	}

	err := RetryWithRollback(ctx, fn, rollback, config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !step1Called {
		t.Error("Step 1 should have been called")
	}

	if !step2Called {
		t.Error("Step 2 should have been called")
	}
}

func TestNewTransaction(t *testing.T) {
	txn := NewTransaction()

	if txn == nil {
		t.Fatal("NewTransaction should not return nil")
	}

	if len(txn.steps) != 0 {
		t.Error("New transaction should have no steps")
	}
}

func TestTransaction_AddStep(t *testing.T) {
	txn := NewTransaction()

	step := TransactionStep{
		Execute:  func(ctx context.Context) error { return nil },
		Rollback: func(ctx context.Context) error { return nil },
	}

	txn.AddStep(step)

	if len(txn.steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(txn.steps))
	}
}

func TestTransaction_Execute_Success(t *testing.T) {
	ctx := context.Background()
	txn := NewTransaction()

	step1Executed := false
	step2Executed := false

	step1 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step1Executed = true
			return nil
		},
		Rollback: func(ctx context.Context) error {
			return nil
		},
	}

	step2 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step2Executed = true
			return nil
		},
		Rollback: func(ctx context.Context) error {
			return nil
		},
	}

	txn.AddStep(step1)
	txn.AddStep(step2)

	err := txn.Execute(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !step1Executed {
		t.Error("Step 1 should have been executed")
	}

	if !step2Executed {
		t.Error("Step 2 should have been executed")
	}
}

func TestTransaction_Execute_FailureWithRollback(t *testing.T) {
	ctx := context.Background()
	txn := NewTransaction()

	step1Executed := false
	step1RolledBack := false
	step2Executed := false

	step1 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step1Executed = true
			return nil
		},
		Rollback: func(ctx context.Context) error {
			step1RolledBack = true
			return nil
		},
	}

	step2 := TransactionStep{
		Execute: func(ctx context.Context) error {
			step2Executed = true
			return fmt.Errorf("step 2 failed")
		},
		Rollback: func(ctx context.Context) error {
			return nil
		},
	}

	txn.AddStep(step1)
	txn.AddStep(step2)

	err := txn.Execute(ctx)

	if err == nil {
		t.Error("Expected error from failed step")
	}

	if !step1Executed {
		t.Error("Step 1 should have been executed")
	}

	if !step2Executed {
		t.Error("Step 2 should have been attempted")
	}

	if !step1RolledBack {
		t.Error("Step 1 should have been rolled back")
	}
}
