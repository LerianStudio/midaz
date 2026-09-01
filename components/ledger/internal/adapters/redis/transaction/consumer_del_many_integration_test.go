//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_DelMany_RemovesEveryKeyInOneCommand proves the property the
// block/unblock cache invalidation depends on: the whole key set disappears
// together, and no key outside the set is touched.
func TestIntegration_DelMany_RemovesEveryKeyInOneCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	targets := []string{
		"balance:{transactions}:del-many:a",
		"balance:{transactions}:del-many:b",
		"balance:{transactions}:del-many:c",
	}
	bystander := "balance:{transactions}:del-many:keep"

	for _, key := range append(append([]string{}, targets...), bystander) {
		require.NoError(t, infra.repo.Set(ctx, key, "cached", time.Minute))
	}

	require.NoError(t, infra.repo.DelMany(ctx, targets))

	for _, key := range targets {
		got, err := infra.repo.Get(ctx, key)
		require.NoError(t, err)
		assert.Empty(t, got, "key %q must be gone after DelMany", key)
	}

	survivor, err := infra.repo.Get(ctx, bystander)
	require.NoError(t, err)
	assert.Equal(t, "cached", survivor, "a key outside the set must never be evicted")
}

// TestIntegration_DelMany_IsIdempotentAndTolerantOfMissingKeys locks the retry
// contract. The command layer re-runs the eviction on every converging retry,
// so a set that is already gone — or a set with only some keys present — must
// still report success.
func TestIntegration_DelMany_IsIdempotentAndTolerantOfMissingKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	present := "balance:{transactions}:del-many-idem:present"
	absent := "balance:{transactions}:del-many-idem:absent"

	require.NoError(t, infra.repo.Set(ctx, present, "cached", time.Minute))

	require.NoError(t, infra.repo.DelMany(ctx, []string{present, absent}),
		"a partially-present set must not error")
	require.NoError(t, infra.repo.DelMany(ctx, []string{present, absent}),
		"repeating the eviction must stay a success")

	// An empty set is the shape the caller passes for an account with no
	// balances; it must be a silent no-op rather than a round-trip or an error.
	require.NoError(t, infra.repo.DelMany(ctx, nil))
	require.NoError(t, infra.repo.DelMany(ctx, []string{}))
}
