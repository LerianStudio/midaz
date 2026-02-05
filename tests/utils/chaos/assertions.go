//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertRecoveryWithin asserts that a check function succeeds within the given timeout.
// This is useful for verifying that a service recovers after chaos injection.
func AssertRecoveryWithin(t *testing.T, check func() error, timeout time.Duration, msgAndArgs ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error

	for {
		select {
		case <-ctx.Done():
			msg := fmt.Sprintf("service did not recover within %v", timeout)
			if lastErr != nil {
				msg = fmt.Sprintf("%s: last error: %v", msg, lastErr)
			}

			require.Fail(t, msg, msgAndArgs...)

			return
		case <-ticker.C:
			if err := check(); err == nil {
				return // Success
			} else {
				lastErr = err
			}
		}
	}
}

// AssertNoDataLoss asserts that data is preserved after chaos.
// It compares the result of a query function before and after chaos.
func AssertNoDataLoss[T comparable](t *testing.T, before, after T, msgAndArgs ...any) {
	t.Helper()
	assert.Equal(t, before, after, msgAndArgs...)
}

// AssertDataIntegrity runs a custom integrity check function and fails if it returns an error.
func AssertDataIntegrity(t *testing.T, check func() error, msgAndArgs ...any) {
	t.Helper()

	err := check()
	require.NoError(t, err, msgAndArgs...)
}

// AssertGracefulDegradation asserts that operations fail gracefully during chaos.
// It expects the operation to return an error (not panic) and optionally validates the error.
func AssertGracefulDegradation(t *testing.T, operation func() error, validateError func(error) bool, msgAndArgs ...any) {
	t.Helper()

	err := operation()
	require.Error(t, err, msgAndArgs...)

	if validateError != nil {
		assert.True(t, validateError(err), "error should match expected type: %v", err)
	}
}
