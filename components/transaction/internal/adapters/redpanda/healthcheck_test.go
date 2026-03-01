// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHealthChecker struct {
	err error
}

func (s stubHealthChecker) Ping(context.Context) error {
	return s.err
}

func TestCheckBrokerHealth(t *testing.T) {
	t.Parallel()

	t.Run("nil checker", func(t *testing.T) {
		t.Parallel()

		err := CheckBrokerHealth(context.Background(), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBrokerUnhealthy)
	})

	t.Run("ping error", func(t *testing.T) {
		t.Parallel()

		err := CheckBrokerHealth(context.Background(), stubHealthChecker{err: errors.New("boom")}) //nolint:err113
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBrokerUnhealthy)
	})

	t.Run("healthy", func(t *testing.T) {
		t.Parallel()

		err := CheckBrokerHealth(context.Background(), stubHealthChecker{})
		assert.NoError(t, err)
	})

	t.Run("preserves root cause", func(t *testing.T) {
		t.Parallel()

		rootCause := errors.New("dial tcp timeout") //nolint:err113

		err := CheckBrokerHealth(context.Background(), stubHealthChecker{err: rootCause})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrBrokerUnhealthy)
		require.ErrorIs(t, err, rootCause)
	})

	t.Run("propagates context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := CheckBrokerHealth(ctx, stubHealthChecker{err: ctx.Err()})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrBrokerUnhealthy)
		require.ErrorIs(t, err, context.Canceled)
	})
}
