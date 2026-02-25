// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/fence"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the operation data source.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// OperationRouteRepo provides an abstraction on top of the operation route data source.
	OperationRouteRepo operationroute.Repository

	// TransactionRouteRepo provides an abstraction on top of the transaction route data source.
	TransactionRouteRepo transactionroute.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository

	// ShardRouter maps account aliases to Redis Cluster shard IDs (Phase 2A).
	// When nil, sharding is disabled and legacy {transactions} hash tags are used.
	ShardRouter *shard.Router

	// ShardManager provides Phase 2B dynamic shard routing/migration controls.
	// When nil, routing falls back to static hash-based ShardRouter behavior.
	ShardManager *internalsharding.Manager

	// Authorizer provides optional external balance authorization over gRPC.
	// When nil or disabled, Redis Lua remains the active authorization path.
	Authorizer Authorizer

	// LagChecker validates whether Redpanda consumer lag is zero for a shard
	// partition before falling back to PostgreSQL on Redis / authorizer misses.
	LagChecker fence.ConsumerLagChecker

	// ConsumerLagFenceEnabled toggles stale-balance fencing.
	ConsumerLagFenceEnabled bool

	// BalanceOperationsTopic is the Redpanda topic used for async balance
	// operations, required to compute per-partition lag.
	BalanceOperationsTopic string

	// StaleBalanceRecoverer replays lagged Redpanda records to reconstruct
	// fresh in-memory balances before falling back to PostgreSQL / authorizer load.
	StaleBalanceRecoverer StaleBalanceRecoverer

	// BalanceCacheTTL controls Redis TTL for balances cached by query flows
	// (authorizer snapshots and stale-balance recovery). Zero means no expiration.
	BalanceCacheTTL time.Duration
}

func (uc *UseCase) balanceCacheTTL() time.Duration {
	if uc == nil || uc.BalanceCacheTTL <= 0 {
		return 0
	}

	return uc.BalanceCacheTTL
}
