// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

func newMockWorkerLogger(t *testing.T) libLog.Logger {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Infof(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	return logger
}

type fakeRebalanceManager struct {
	paused    bool
	pausedErr error

	loads      []internalsharding.ShardLoad
	loadsErr   error
	loadsCalls int

	hotAccounts []internalsharding.HotAccount
	hotErr      error

	isolationCounts map[int]int64
	isolationErr    error

	permitByAlias map[string]bool
	permitErr     error

	migrateResult *internalsharding.MigrationResult
	migrateErr    error
	migrations    []internalsharding.MigrationResult
	migrateFn     func(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, targetShard int, knownBalanceKeys []string) (*internalsharding.MigrationResult, error)

	markErr error
	marked  []internalsharding.HotAccount
}

func (f *fakeRebalanceManager) IsRebalancerPaused(_ context.Context) (bool, error) {
	return f.paused, f.pausedErr
}

func (f *fakeRebalanceManager) GetShardLoads(_ context.Context, _ int, _ time.Duration) ([]internalsharding.ShardLoad, error) {
	f.loadsCalls++
	if f.loadsErr != nil {
		return nil, f.loadsErr
	}

	return f.loads, nil
}

func (f *fakeRebalanceManager) TopHotAccounts(_ context.Context, _ int, _ time.Duration, _ int) ([]internalsharding.HotAccount, error) {
	if f.hotErr != nil {
		return nil, f.hotErr
	}

	return f.hotAccounts, nil
}

func (f *fakeRebalanceManager) GetShardIsolationCounts(_ context.Context, _ int) (map[int]int64, error) {
	if f.isolationErr != nil {
		return nil, f.isolationErr
	}

	return f.isolationCounts, nil
}

func (f *fakeRebalanceManager) TryAcquireRebalancePermits(_ context.Context, _, _ int, account internalsharding.HotAccount) (bool, error) {
	if f.permitErr != nil {
		return false, f.permitErr
	}

	if f.permitByAlias == nil {
		return true, nil
	}

	return f.permitByAlias[account.Alias], nil
}

func (f *fakeRebalanceManager) MigrateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, targetShard int, _ []string) (*internalsharding.MigrationResult, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx, organizationID, ledgerID, alias, targetShard, nil)
	}

	if f.migrateErr != nil {
		return nil, f.migrateErr
	}

	result := f.migrateResult
	if result == nil {
		result = &internalsharding.MigrationResult{Alias: alias, SourceShard: 0, TargetShard: targetShard, MigratedKeys: 1}
	}

	f.migrations = append(f.migrations, internalsharding.MigrationResult{
		Alias:        alias,
		SourceShard:  result.SourceShard,
		TargetShard:  targetShard,
		MigratedKeys: result.MigratedKeys,
	})

	return &internalsharding.MigrationResult{
		Alias:        alias,
		SourceShard:  result.SourceShard,
		TargetShard:  targetShard,
		MigratedKeys: result.MigratedKeys,
	}, nil
}

func (f *fakeRebalanceManager) MarkAccountIsolated(_ context.Context, account internalsharding.HotAccount, _ int) error {
	if f.markErr != nil {
		return f.markErr
	}

	f.marked = append(f.marked, account)

	return nil
}

func TestSelectTargetShard_BalancesProjectedMaxLoad(t *testing.T) {
	t.Parallel()

	loadByShard := map[int]int64{
		0: 200,
		1: 120,
		2: 80,
		3: 40,
	}

	target, ok := selectTargetShard(loadByShard, 0, 60, false, map[int]int64{})

	assert.True(t, ok)
	assert.Equal(t, 3, target)
}

func TestSelectTargetShard_IsolationAvoidsReservedShards(t *testing.T) {
	t.Parallel()

	loadByShard := map[int]int64{
		0: 300,
		1: 20,
		2: 10,
		3: 5,
	}

	isolationCounts := map[int]int64{
		1: 1,
		2: 1,
	}

	target, ok := selectTargetShard(loadByShard, 0, 180, true, isolationCounts)

	assert.True(t, ok)
	assert.Equal(t, 3, target)
}

func TestSelectTargetShard_NonIsolationFallsBackWhenAllReserved(t *testing.T) {
	t.Parallel()

	loadByShard := map[int]int64{
		0: 250,
		1: 10,
		2: 20,
	}

	isolationCounts := map[int]int64{
		1: 1,
		2: 1,
	}

	target, ok := selectTargetShard(loadByShard, 0, 25, false, isolationCounts)

	assert.True(t, ok)
	assert.Equal(t, 1, target)
}

func TestSelectTargetShard_BoundaryConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		loadByShard     map[int]int64
		sourceShard     int
		accountLoad     int64
		mustIsolate     bool
		isolationCounts map[int]int64
		wantShard       int
		wantOK          bool
	}{
		{
			name:            "accountLoad zero returns false",
			loadByShard:     map[int]int64{0: 200, 1: 50},
			sourceShard:     0,
			accountLoad:     0,
			mustIsolate:     false,
			isolationCounts: map[int]int64{},
			wantShard:       0,
			wantOK:          false,
		},
		{
			name:            "single shard only source exists returns false",
			loadByShard:     map[int]int64{0: 500},
			sourceShard:     0,
			accountLoad:     100,
			mustIsolate:     false,
			isolationCounts: map[int]int64{},
			wantShard:       0,
			wantOK:          false,
		},
		{
			name:            "all shards at zero load picks lowest shard ID",
			loadByShard:     map[int]int64{0: 0, 1: 0, 2: 0},
			sourceShard:     0,
			accountLoad:     10,
			mustIsolate:     false,
			isolationCounts: map[int]int64{},
			wantShard:       1,
			wantOK:          true,
		},
		{
			name:        "mustIsolate true with all candidates having isolation counts returns false",
			loadByShard: map[int]int64{0: 300, 1: 20, 2: 10},
			sourceShard: 0,
			accountLoad: 150,
			mustIsolate: true,
			isolationCounts: map[int]int64{
				1: 1,
				2: 2,
			},
			wantShard: 0,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotShard, gotOK := selectTargetShard(tt.loadByShard, tt.sourceShard, tt.accountLoad, tt.mustIsolate, tt.isolationCounts)
			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantShard, gotShard)
		})
	}
}

func TestShardRebalanceWorkerRebalanceOncePaused(t *testing.T) {
	t.Parallel()

	manager := &fakeRebalanceManager{paused: true}
	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(4), time.Second, time.Minute, 1.5, 4, 0.7, 100)

	require.NotNil(t, worker)
	require.NoError(t, worker.rebalanceOnce(context.Background()))
	assert.Equal(t, 0, manager.loadsCalls)
}

func TestShardRebalanceWorkerRebalanceOnceMigratesEligibleAccount(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 200},
			{ShardID: 1, Load: 50},
			{ShardID: 2, Load: 40},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@external/USD", Load: 180},
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@alice", Load: 120},
		},
		permitByAlias:   map[string]bool{"@alice": true},
		isolationCounts: map[int]int64{},
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	assert.Equal(t, "@alice", manager.migrations[0].Alias)
	assert.Equal(t, 2, manager.migrations[0].TargetShard)
}

func TestShardRebalanceWorkerRebalanceOnceSkipsDeniedPermitAndMigratesNext(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 280},
			{ShardID: 1, Load: 60},
			{ShardID: 2, Load: 40},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@blocked", Load: 160},
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@next", Load: 100},
		},
		permitByAlias:   map[string]bool{"@blocked": false, "@next": true},
		isolationCounts: map[int]int64{},
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	assert.Equal(t, "@next", manager.migrations[0].Alias)
}

func TestShardRebalanceWorkerRebalanceOnceMarksIsolation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 320},
			{ShardID: 1, Load: 20},
			{ShardID: 2, Load: 10},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@whale", Load: 260},
		},
		permitByAlias:   map[string]bool{"@whale": true},
		isolationCounts: map[int]int64{},
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	require.Len(t, manager.marked, 1)
	assert.Equal(t, "@whale", manager.marked[0].Alias)
}

func TestShardRebalanceWorkerRebalanceOnceReturnsPermitError(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{{ShardID: 0, Load: 220}, {ShardID: 1, Load: 40}},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@alice", Load: 150},
		},
		isolationCounts: map[int]int64{},
		permitErr:       errors.New("redis unavailable"), //nolint:err113
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(2), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	err := worker.rebalanceOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis unavailable")
}

func TestShardRebalanceWorkerRebalanceOnceReturnsLoadError(t *testing.T) {
	t.Parallel()

	manager := &fakeRebalanceManager{loadsErr: errors.New("load error")} //nolint:err113
	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(2), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	err := worker.rebalanceOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load error")
}

func TestShardRebalanceWorkerRebalanceOnceReturnsHotAccountError(t *testing.T) {
	t.Parallel()

	manager := &fakeRebalanceManager{
		loads:  []internalsharding.ShardLoad{{ShardID: 0, Load: 220}, {ShardID: 1, Load: 40}},
		hotErr: errors.New("hot account error"), //nolint:err113
	}
	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(2), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	err := worker.rebalanceOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hot account error")
}

func TestShardRebalanceWorkerRebalanceOnceReturnsIsolationError(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads:        []internalsharding.ShardLoad{{ShardID: 0, Load: 220}, {ShardID: 1, Load: 40}},
		hotAccounts:  []internalsharding.HotAccount{{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@alice", Load: 150}},
		isolationErr: errors.New("isolation error"), //nolint:err113
	}
	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(2), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	err := worker.rebalanceOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "isolation error")
}

func TestShardRebalanceWorkerRebalanceOnceContinuesAfterMigrationError(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 280},
			{ShardID: 1, Load: 60},
			{ShardID: 2, Load: 40},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@fail", Load: 170},
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@next", Load: 120},
		},
		permitByAlias:   map[string]bool{"@fail": true, "@next": true},
		isolationCounts: map[int]int64{},
		migrateResult:   &internalsharding.MigrationResult{Alias: "@next", SourceShard: 0, TargetShard: 2, MigratedKeys: 1},
	}

	originalMigrateErr := errors.New("migration failed") //nolint:err113
	called := 0
	manager.migrateFn = func(_ context.Context, organizationID, ledgerID uuid.UUID, alias string, targetShard int, _ []string) (*internalsharding.MigrationResult, error) {
		called++
		if called == 1 {
			return nil, originalMigrateErr
		}

		manager.migrations = append(manager.migrations, internalsharding.MigrationResult{Alias: alias, TargetShard: targetShard, MigratedKeys: 1})

		return &internalsharding.MigrationResult{Alias: alias, SourceShard: 0, TargetShard: targetShard, MigratedKeys: 1}, nil
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	assert.Equal(t, "@next", manager.migrations[0].Alias)
}

func TestShardRebalanceWorkerRebalanceOnceReturnsPauseStateError(t *testing.T) {
	t.Parallel()

	manager := &fakeRebalanceManager{pausedErr: errors.New("pause read error")} //nolint:err113
	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(2), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	err := worker.rebalanceOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pause read error")
}

func TestShardRebalanceWorkerRebalanceOnceContinuesWhenIsolationMarkFails(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 320},
			{ShardID: 1, Load: 20},
			{ShardID: 2, Load: 10},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@whale", Load: 260},
		},
		permitByAlias:   map[string]bool{"@whale": true},
		isolationCounts: map[int]int64{},
		markErr:         errors.New("mark failed"), //nolint:err113
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	assert.Empty(t, manager.marked)
}

func TestShardRebalanceWorkerRebalanceOnceContinuesWhenMigrationResultIsNil(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	manager := &fakeRebalanceManager{
		loads: []internalsharding.ShardLoad{
			{ShardID: 0, Load: 280},
			{ShardID: 1, Load: 60},
			{ShardID: 2, Load: 40},
		},
		hotAccounts: []internalsharding.HotAccount{
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@nil-result", Load: 180},
			{OrganizationID: organizationID, LedgerID: ledgerID, Alias: "@next", Load: 120},
		},
		permitByAlias:   map[string]bool{"@nil-result": true, "@next": true},
		isolationCounts: map[int]int64{},
	}

	call := 0
	manager.migrateFn = func(_ context.Context, organizationID, ledgerID uuid.UUID, alias string, targetShard int, _ []string) (*internalsharding.MigrationResult, error) {
		call++
		if call == 1 {
			return nil, nil
		}

		manager.migrations = append(manager.migrations, internalsharding.MigrationResult{Alias: alias, TargetShard: targetShard, MigratedKeys: 1})

		return &internalsharding.MigrationResult{Alias: alias, SourceShard: 0, TargetShard: targetShard, MigratedKeys: 1}, nil
	}

	worker := NewShardRebalanceWorker(newMockWorkerLogger(t), manager, shard.NewRouter(3), time.Second, time.Minute, 1.5, 4, 0.7, 200)

	require.NoError(t, worker.rebalanceOnce(context.Background()))
	require.Len(t, manager.migrations, 1)
	assert.Equal(t, "@next", manager.migrations[0].Alias)
}
