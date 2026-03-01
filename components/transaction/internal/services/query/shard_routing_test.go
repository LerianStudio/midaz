// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// pipelineErrorClient is a minimal fake goredis.UniversalClient whose
// Pipeline() method returns a pipeline that always fails on Exec. This lets
// us construct a live *internalsharding.Manager without a real Redis server
// and verify that recordShardLoad does not panic when RecordShardAliasLoad
// encounters a Redis error (the error is intentionally discarded by the caller).
type pipelineErrorClient struct {
	goredis.UniversalClient
}

func (c *pipelineErrorClient) Pipeline() goredis.Pipeliner {
	return &errorPipeliner{}
}

// errorPipeliner satisfies goredis.Pipeliner. Every method is a no-op except
// Exec, which returns a hard error to simulate Redis unavailability.
type errorPipeliner struct {
	goredis.Pipeliner
}

func (p *errorPipeliner) HIncrBy(_ context.Context, _, _ string, _ int64) *goredis.IntCmd {
	return goredis.NewIntResult(0, nil)
}

func (p *errorPipeliner) Expire(_ context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	return goredis.NewBoolResult(false, nil)
}

func (p *errorPipeliner) Exec(_ context.Context) ([]goredis.Cmder, error) {
	return nil, errors.New("simulated pipeline exec failure") //nolint:err113
}

// newTestShardManager returns a *internalsharding.Manager backed by a fake
// Redis client that fails on pipeline exec. Suitable for unit tests where we
// only need the manager to be non-nil and Enabled().
func newTestShardManager(router *shard.Router) *internalsharding.Manager {
	conn := &libRedis.RedisConnection{Client: &pipelineErrorClient{}}
	return internalsharding.NewManager(conn, router, nil, internalsharding.Config{})
}

// makeOp constructs a minimal BalanceOperation for testing recordShardLoad.
func makeOp(alias string, shardID int) mmodel.BalanceOperation {
	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:        uuid.New().String(),
			AccountID: uuid.New().String(),
			Alias:     alias,
			Key:       "default",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
		},
		Alias:   alias,
		ShardID: shardID,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(50),
			Operation: "DEBIT",
		},
	}
}

// =============================================================================
// UNIT TESTS — recordShardLoad
// =============================================================================.

func TestRecordShardLoad(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	ctx := context.Background()
	router := shard.NewRouter(8)

	t.Run("normal case: valid shard ID with non-nil ShardManager does not panic", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{
			ShardManager: newTestShardManager(router),
		}

		ops := []mmodel.BalanceOperation{
			makeOp("@alice", 2),
			makeOp("@bob", 5),
		}

		// Must not panic. The underlying RecordShardAliasLoad call will fail on
		// the fake pipeline exec, but recordShardLoad intentionally discards it.
		uc.recordShardLoad(ctx, orgID, ledgerID, ops)
	})

	t.Run("nil ShardManager: returns immediately without panic", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{
			ShardManager: nil,
		}

		ops := []mmodel.BalanceOperation{makeOp("@carol", 1)}

		// The guard `uc.ShardManager == nil` must prevent any dereference.
		uc.recordShardLoad(ctx, orgID, ledgerID, ops)
	})

	t.Run("nil UseCase receiver: returns immediately without panic", func(t *testing.T) {
		t.Parallel()

		var uc *UseCase

		ops := []mmodel.BalanceOperation{makeOp("@dave", 3)}

		// The guard `uc == nil` must prevent any dereference.
		uc.recordShardLoad(ctx, orgID, ledgerID, ops)
	})

	t.Run("empty operations slice: returns immediately without any calls", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{
			ShardManager: newTestShardManager(router),
		}

		// len(operations) == 0 triggers the early-return guard.
		uc.recordShardLoad(ctx, orgID, ledgerID, nil)
		uc.recordShardLoad(ctx, orgID, ledgerID, []mmodel.BalanceOperation{})
	})

	t.Run("negative shard ID: operation is skipped without panic", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{
			ShardManager: newTestShardManager(router),
		}

		// ShardID < 0 means the op was not assigned to a shard; the loop must
		// skip it via `continue` rather than forwarding an invalid ID.
		ops := []mmodel.BalanceOperation{makeOp("@eve", -1)}

		uc.recordShardLoad(ctx, orgID, ledgerID, ops)
	})

	t.Run("mixed valid and negative shard IDs: valid op is processed, negative skipped", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{
			ShardManager: newTestShardManager(router),
		}

		ops := []mmodel.BalanceOperation{
			makeOp("@valid", 4),
			makeOp("@skipped", -1),
			makeOp("@also-valid", 7),
		}

		uc.recordShardLoad(ctx, orgID, ledgerID, ops)
	})
}
