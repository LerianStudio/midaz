// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func TestIsRetryableBatchBalanceUpdateError(t *testing.T) {
	t.Parallel()

	t.Run("deadlock_detected_pgx_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: pgErrCodeDeadlockDetected}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("serialization_failure_pgx_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: pgErrCodeSerializationError}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("lock_not_available_pgx_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: pgErrCodeLockNotAvailable}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("deadlock_detected_pq_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pq.Error{Code: pq.ErrorCode(pgErrCodeDeadlockDetected)}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("serialization_failure_pq_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pq.Error{Code: pq.ErrorCode(pgErrCodeSerializationError)}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("lock_not_available_pq_is_retryable", func(t *testing.T) {
		t.Parallel()

		err := &pq.Error{Code: pq.ErrorCode(pgErrCodeLockNotAvailable)}
		assert.True(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("non_retryable_pgx_error", func(t *testing.T) {
		t.Parallel()

		// 23505 = unique_violation -- not retryable
		err := &pgconn.PgError{Code: "23505"}
		assert.False(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("non_retryable_pq_error", func(t *testing.T) {
		t.Parallel()

		// 23505 = unique_violation -- not retryable
		err := &pq.Error{Code: "23505"}
		assert.False(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("generic_error_is_not_retryable", func(t *testing.T) {
		t.Parallel()

		err := errors.New("some random error") //nolint:err113
		assert.False(t, isRetryableBatchBalanceUpdateError(err))
	})

	t.Run("wrapped_pgx_deadlock_is_retryable", func(t *testing.T) {
		t.Parallel()

		pgErr := &pgconn.PgError{Code: pgErrCodeDeadlockDetected}
		wrapped := fmt.Errorf("batch balance update chunk 0-5: %w", pgErr)
		assert.True(t, isRetryableBatchBalanceUpdateError(wrapped))
	})

	t.Run("wrapped_pq_deadlock_is_retryable", func(t *testing.T) {
		t.Parallel()

		pqErr := &pq.Error{Code: pq.ErrorCode(pgErrCodeDeadlockDetected)}
		wrapped := fmt.Errorf("batch balance update chunk 0-5: %w", pqErr)
		assert.True(t, isRetryableBatchBalanceUpdateError(wrapped))
	})

	t.Run("nil_error_is_not_retryable", func(t *testing.T) {
		t.Parallel()

		assert.False(t, isRetryableBatchBalanceUpdateError(nil))
	})
}

func TestRetryableErrorConstants(t *testing.T) {
	t.Parallel()

	// Verify that the error code constants match their expected PostgreSQL SQLSTATE codes.
	// This guards against accidental changes to the constants.
	t.Run("error_code_constants_are_correct", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "40P01", pgErrCodeDeadlockDetected, "deadlock_detected should be 40P01")
		assert.Equal(t, "40001", pgErrCodeSerializationError, "serialization_failure should be 40001")
		assert.Equal(t, "55P03", pgErrCodeLockNotAvailable, "lock_not_available should be 55P03")
	})

	t.Run("max_retries_constant_is_positive", func(t *testing.T) {
		t.Parallel()

		assert.Positive(t, balanceUpdateMaxRetries, "balanceUpdateMaxRetries must be > 0")
		assert.Equal(t, 4, balanceUpdateMaxRetries, "balanceUpdateMaxRetries should be 4")
	})
}
