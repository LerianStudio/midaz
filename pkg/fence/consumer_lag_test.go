// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
)

type fakeLagAdminClient struct {
	fetchOffsetsResp kadm.OffsetResponses
	fetchOffsetsErr  error
	listEndResp      kadm.ListedOffsets
	listEndErr       error

	fetchOffsetsCalls int
	listEndCalls      int
}

func (f *fakeLagAdminClient) FetchOffsets(_ context.Context, _ string) (kadm.OffsetResponses, error) {
	f.fetchOffsetsCalls++

	if f.fetchOffsetsErr != nil {
		return nil, f.fetchOffsetsErr
	}

	return f.fetchOffsetsResp, nil
}

func (f *fakeLagAdminClient) ListEndOffsets(_ context.Context, _ ...string) (kadm.ListedOffsets, error) {
	f.listEndCalls++

	if f.listEndErr != nil {
		return nil, f.listEndErr
	}

	return f.listEndResp, nil
}

func TestFranzConsumerLagCheckerPartitionLag(t *testing.T) {
	t.Parallel()

	t.Run("returns zero when partition is caught up", func(t *testing.T) {
		t.Parallel()

		admin := &fakeLagAdminClient{
			fetchOffsetsResp: kadm.OffsetResponses{
				"ledger.balance.operations": map[int32]kadm.OffsetResponse{
					3: {
						Offset: kadm.Offset{Topic: "ledger.balance.operations", Partition: 3, At: 120},
					},
				},
			},
			listEndResp: kadm.ListedOffsets{
				"ledger.balance.operations": map[int32]kadm.ListedOffset{
					3: {Topic: "ledger.balance.operations", Partition: 3, Offset: 120},
				},
			},
		}

		checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)

		lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 3)
		require.NoError(t, err)
		assert.Equal(t, int64(0), lag)
	})

	t.Run("returns positive lag when end offset is ahead", func(t *testing.T) {
		t.Parallel()

		admin := &fakeLagAdminClient{
			fetchOffsetsResp: kadm.OffsetResponses{
				"ledger.balance.operations": map[int32]kadm.OffsetResponse{
					2: {
						Offset: kadm.Offset{Topic: "ledger.balance.operations", Partition: 2, At: 95},
					},
				},
			},
			listEndResp: kadm.ListedOffsets{
				"ledger.balance.operations": map[int32]kadm.ListedOffset{
					2: {Topic: "ledger.balance.operations", Partition: 2, Offset: 100},
				},
			},
		}

		checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)

		lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 2)
		require.NoError(t, err)
		assert.Equal(t, int64(5), lag)
	})
}

func TestFranzConsumerLagCheckerFailOpenOnAdminError(t *testing.T) {
	t.Parallel()

	admin := &fakeLagAdminClient{
		fetchOffsetsErr: errors.New("redpanda unavailable"),
	}

	checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)

	lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 1)
	require.Error(t, err)
	assert.Equal(t, int64(0), lag)

	caughtUp := checker.IsPartitionCaughtUp(context.Background(), "ledger.balance.operations", 1)
	assert.True(t, caughtUp, "on checker error, fence must fail-open")
}

func TestFranzConsumerLagCheckerUsesTTLCache(t *testing.T) {
	t.Parallel()

	admin := &fakeLagAdminClient{
		fetchOffsetsResp: kadm.OffsetResponses{
			"ledger.balance.operations": map[int32]kadm.OffsetResponse{
				7: {
					Offset: kadm.Offset{Topic: "ledger.balance.operations", Partition: 7, At: 20},
				},
			},
		},
		listEndResp: kadm.ListedOffsets{
			"ledger.balance.operations": map[int32]kadm.ListedOffset{
				7: {Topic: "ledger.balance.operations", Partition: 7, Offset: 25},
			},
		},
	}

	checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)
	now := time.Unix(1730000000, 0)
	checker.now = func() time.Time { return now }

	lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 7)
	require.NoError(t, err)
	assert.Equal(t, int64(5), lag)
	assert.Equal(t, 1, admin.fetchOffsetsCalls)
	assert.Equal(t, 1, admin.listEndCalls)

	lag, err = checker.PartitionLag(context.Background(), "ledger.balance.operations", 7)
	require.NoError(t, err)
	assert.Equal(t, int64(5), lag)
	assert.Equal(t, 1, admin.fetchOffsetsCalls)
	assert.Equal(t, 1, admin.listEndCalls)

	now = now.Add(501 * time.Millisecond)
	lag, err = checker.PartitionLag(context.Background(), "ledger.balance.operations", 7)
	require.NoError(t, err)
	assert.Equal(t, int64(5), lag)
	assert.Equal(t, 2, admin.fetchOffsetsCalls)
	assert.Equal(t, 2, admin.listEndCalls)
}

func TestFranzConsumerLagChecker_IsPartitionCaughtUpLagBoundary(t *testing.T) {
	t.Parallel()

	admin := &fakeLagAdminClient{
		fetchOffsetsResp: kadm.OffsetResponses{
			"ledger.balance.operations": map[int32]kadm.OffsetResponse{
				5: {
					Offset: kadm.Offset{Topic: "ledger.balance.operations", Partition: 5, At: 99},
				},
			},
		},
		listEndResp: kadm.ListedOffsets{
			"ledger.balance.operations": map[int32]kadm.ListedOffset{
				5: {Topic: "ledger.balance.operations", Partition: 5, Offset: 100},
			},
		},
	}

	checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)

	lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 5)
	require.NoError(t, err)
	assert.Equal(t, int64(1), lag)
	assert.False(t, checker.IsPartitionCaughtUp(context.Background(), "ledger.balance.operations", 5))
}

func TestFranzConsumerLagChecker_ListEndOffsetsErrorPath(t *testing.T) {
	t.Parallel()

	admin := &fakeLagAdminClient{
		fetchOffsetsResp: kadm.OffsetResponses{
			"ledger.balance.operations": map[int32]kadm.OffsetResponse{
				2: {
					Offset: kadm.Offset{Topic: "ledger.balance.operations", Partition: 2, At: 10},
				},
			},
		},
		listEndErr: errors.New("list end offsets unavailable"),
	}

	checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, true)

	lag, err := checker.PartitionLag(context.Background(), "ledger.balance.operations", 2)
	require.Error(t, err)
	assert.Equal(t, int64(0), lag)
	assert.ErrorContains(t, err, "list end offsets")
}

func TestFranzConsumerLagChecker_FailClosedOnAdminError(t *testing.T) {
	t.Parallel()

	admin := &fakeLagAdminClient{
		fetchOffsetsErr: errors.New("redpanda unavailable"),
	}

	checker := newFranzConsumerLagChecker(admin, "midaz-balance-projector", 500*time.Millisecond, false)

	assert.False(t, checker.IsPartitionCaughtUp(context.Background(), "ledger.balance.operations", 1))
}
