// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveAllowFlagArg locks the tri-state ARGV contract of
// scripts/update_balance_allow_flags.lua: an absent flag keeps the cached
// value, a present one writes its boolean.
func TestResolveAllowFlagArg(t *testing.T) {
	t.Parallel()

	t.Run("nil keeps the cached value", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, -1, resolveAllowFlagArg(nil))
	})

	t.Run("false writes 0", func(t *testing.T) {
		t.Parallel()

		flag := false

		assert.Equal(t, 0, resolveAllowFlagArg(&flag))
	})

	t.Run("true writes 1", func(t *testing.T) {
		t.Parallel()

		flag := true

		assert.Equal(t, 1, resolveAllowFlagArg(&flag))
	})
}

// TestUpdateBalanceCacheAllowFlags_BothNilIsNoop pins the entry guard: a call
// carrying neither flag returns before any Redis interaction. The repository
// holds a nil connection here, so reaching the client would panic — the test
// passing IS the proof that Redis is never touched.
func TestUpdateBalanceCacheAllowFlags_BothNilIsNoop(t *testing.T) {
	t.Parallel()

	rr := &RedisConsumerRepository{}

	err := rr.UpdateBalanceCacheAllowFlags(context.Background(), uuid.New(), uuid.New(), "@cash#default", nil, nil)

	require.NoError(t, err)
}
