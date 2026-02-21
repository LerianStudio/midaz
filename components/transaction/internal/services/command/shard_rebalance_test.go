// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestShardManagerWithClient(t *testing.T) (*internalsharding.Manager, redis.UniversalClient) {
	t.Helper()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	conn := &libRedis.RedisConnection{Client: client, Connected: true}

	t.Cleanup(func() {
		err := client.Close()
		if err != nil {
			require.ErrorIs(t, err, redis.ErrClosed)
		}
		mini.Close()
	})

	manager := internalsharding.NewManager(conn, shard.NewRouter(8), nil, internalsharding.Config{})
	require.NotNil(t, manager)

	return manager, client
}

func newTestShardManager(t *testing.T) *internalsharding.Manager {
	t.Helper()

	manager, _ := newTestShardManagerWithClient(t)

	return manager
}

func TestMigrateAccountShardRequiresBalanceRepository(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), uuid.New(), uuid.New(), "@alice", 1)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "balance repository not configured")
}

func TestMigrateAccountShardRejectsWildcardAlias(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{
		BalanceRepo:  balance.NewMockRepository(ctrl),
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), uuid.New(), uuid.New(), "@ali*ce", 1)

	require.Error(t, err)
	assert.Nil(t, result)

	var invalidAliasErr pkg.InternalServerError
	assert.ErrorAs(t, err, &invalidAliasErr)
}

func TestMigrateAccountShardRejectsEmptyAlias(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{
		BalanceRepo:  balance.NewMockRepository(ctrl),
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), uuid.New(), uuid.New(), "", 1)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "alias is required")
}

func TestMigrateAccountShardRejectsOutOfRangeTargetShard(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{
		BalanceRepo:  balance.NewMockRepository(ctrl),
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	tests := []int{-1, 8}
	for _, targetShard := range tests {
		result, err := uc.MigrateAccountShard(context.Background(), uuid.New(), uuid.New(), "@alice", targetShard)

		require.Error(t, err)
		assert.Nil(t, result)

		var invalidParamErr pkg.ValidationError
		assert.ErrorAs(t, err, &invalidParamErr)
	}
}

func TestMigrateAccountShardReturnsRepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	repoErr := errors.New("repository unavailable")

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		ListByAliases(gomock.Any(), organizationID, ledgerID, []string{"@alice"}).
		Return(nil, repoErr)

	uc := &UseCase{
		BalanceRepo:  mockBalanceRepo,
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), organizationID, ledgerID, "@alice", 1)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, repoErr)
}

func TestMigrateAccountShardSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "@alice"
	router := shard.NewRouter(8)
	manager, client := newTestShardManagerWithClient(t)

	sourceShard := router.ResolveBalance(alias, constant.DefaultBalanceKey)
	targetShard := (sourceShard + 1) % router.ShardCount()
	sourceKey := utils.BalanceShardKey(sourceShard, organizationID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	require.NoError(t, client.Set(context.Background(), sourceKey, `{"available":"100"}`, 0).Err())

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
		Return([]*mmodel.Balance{{Alias: alias, Key: constant.DefaultBalanceKey}}, nil)

	uc := &UseCase{
		BalanceRepo:  mockBalanceRepo,
		ShardRouter:  router,
		ShardManager: manager,
	}

	result, err := uc.MigrateAccountShard(context.Background(), organizationID, ledgerID, alias, targetShard)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, alias, result.Alias)
	assert.Equal(t, sourceShard, result.SourceShard)
	assert.Equal(t, targetShard, result.TargetShard)
	assert.Equal(t, 1, result.MigratedKeys)
}

func TestSetShardRebalancePausedAndStatus(t *testing.T) {
	t.Parallel()

	manager := newTestShardManager(t)
	router := shard.NewRouter(8)
	uc := &UseCase{ShardManager: manager, ShardRouter: router}

	require.NoError(t, uc.SetShardRebalancePaused(context.Background(), true))

	status, err := uc.GetShardRebalanceStatus(context.Background())
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Paused)
	assert.Len(t, status.Loads, router.ShardCount())
}

func TestSetShardRebalancePausedRequiresManager(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	err := uc.SetShardRebalancePaused(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard manager not configured")
}

func TestGetShardRebalanceStatusRequiresManagerAndRouter(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	status, err := uc.GetShardRebalanceStatus(context.Background())
	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "shard manager not configured")
}

func TestGetShardRebalanceStatusReturnsManagerError(t *testing.T) {
	t.Parallel()

	manager, client := newTestShardManagerWithClient(t)
	router := shard.NewRouter(8)
	uc := &UseCase{ShardManager: manager, ShardRouter: router}

	require.NoError(t, client.Close())

	status, err := uc.GetShardRebalanceStatus(context.Background())
	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "closed")
}

func TestMigrateAccountShardRejectsExternalAlias(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{
		BalanceRepo:  balance.NewMockRepository(ctrl),
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), uuid.New(), uuid.New(), "@external/USD", 1)

	require.Error(t, err)
	assert.Nil(t, result)

	var invalidAliasErr pkg.InternalServerError
	assert.ErrorAs(t, err, &invalidAliasErr)
}

func TestMigrateAccountShardReturnsNotFoundWhenAliasMissing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		ListByAliases(gomock.Any(), organizationID, ledgerID, []string{"@alice"}).
		Return(nil, nil)

	uc := &UseCase{
		BalanceRepo:  mockBalanceRepo,
		ShardRouter:  shard.NewRouter(8),
		ShardManager: newTestShardManager(t),
	}

	result, err := uc.MigrateAccountShard(context.Background(), organizationID, ledgerID, "@alice", 1)

	require.Error(t, err)
	assert.Nil(t, result)

	var notFoundErr pkg.EntityNotFoundError
	assert.ErrorAs(t, err, &notFoundErr)
}
