// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
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

	// BrokerRepo provides an abstraction on top of the message broker producer.
	BrokerRepo redpanda.ProducerRepository

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

	// BalanceOperationsTopic is the topic for async balance-transaction-operation messages.
	BalanceOperationsTopic string

	// BalanceCreateTopic is the topic for balance create events.
	BalanceCreateTopic string

	// EventsTopic is the topic for transaction events.
	EventsTopic string
	// DecisionEventsTopic is the topic for decision lifecycle events.
	// When empty, decision events fall back to EventsTopic.
	DecisionEventsTopic string
	// EventsEnabled controls transaction event publication.
	EventsEnabled bool

	// AuditTopic is the topic for transaction audit logs.
	AuditTopic string
	// AuditLogEnabled controls transaction audit publication.
	AuditLogEnabled bool

	// TransactionAsync controls whether transactions are written asynchronously via broker
	// or synchronously via direct DB writes. Resolved once at startup from
	// TRANSACTION_ASYNC to avoid per-request os.Getenv overhead.
	TransactionAsync bool

	// Version is emitted in transaction events and resolved once at startup.
	Version string

	// BatchSideEffectsTimeout is the maximum time to wait for post-commit side
	// effects (Redis cleanup, event publishing) before the consumer worker moves
	// on. Zero means use the default (2s).
	BatchSideEffectsTimeout time.Duration

	// DecisionLifecycleSyncForTests forces synchronous decision lifecycle event
	// dispatch in tests to enable deterministic event-order assertions.
	// Production code should keep this false.
	DecisionLifecycleSyncForTests bool

	// IdempotencyReplayTimeout is the maximum time to poll Redis for an in-flight
	// idempotency value before giving up. Zero means use the default (75ms).
	IdempotencyReplayTimeout time.Duration
}

// CheckHealth returns nil for unified mode (in-process calls don't need health checks).
func (uc *UseCase) CheckHealth(ctx context.Context) error {
	return nil
}
