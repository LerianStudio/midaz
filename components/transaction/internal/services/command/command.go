// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// Compile-time interface verification.
// UseCase implements mbootstrap.BalancePort for unified ledger mode,
// allowing the onboarding module to call balance operations directly (in-process)
// without network overhead.
var _ mbootstrap.BalancePort = (*UseCase)(nil)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the asset rate data source.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// OperationRouteRepo provides an abstraction on top of the operation route data source.
	OperationRouteRepo operationroute.Repository

	// TransactionRouteRepo provides an abstraction on top of the transaction route data source.
	TransactionRouteRepo transactionroute.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository

	// ShardRouter maps account aliases to Redis Cluster shard IDs (Phase 2A).
	// When nil, sharding is disabled and legacy {transactions} hash tags are used.
	ShardRouter *shard.Router

	// ShardManager provides Phase 2B dynamic shard routing/migration controls.
	// When nil, routing falls back to static hash-based ShardRouter behavior.
	ShardManager *internalsharding.Manager

	// Authorizer owns async BTO publishing when enabled.
	Authorizer AuthorizerPublisher

	// RabbitMQBalanceOperationExchange is the exchange name for async balance-transaction-operation messages.
	// Injected from config to avoid direct os.Getenv calls in the service layer.
	RabbitMQBalanceOperationExchange string

	// RabbitMQBalanceOperationKey is the routing key for async balance-transaction-operation messages.
	// Injected from config to avoid direct os.Getenv calls in the service layer.
	RabbitMQBalanceOperationKey string

	// TransactionAsync controls whether transactions are written asynchronously via RabbitMQ
	// or synchronously via direct DB writes. Resolved once at startup from
	// RABBITMQ_TRANSACTION_ASYNC to avoid per-request os.Getenv overhead.
	TransactionAsync bool

	// ShardedBTOQueuesEnabled controls whether balance-transaction-operation messages
	// are routed to per-shard queues. Resolved once at startup from
	// RABBITMQ_TRANSACTION_BALANCE_OPERATION_SHARDED to avoid per-request os.Getenv overhead.
	ShardedBTOQueuesEnabled bool
}

// CheckHealth returns nil for unified mode (in-process calls don't need health checks).
func (uc *UseCase) CheckHealth(ctx context.Context) error {
	return nil
}
