// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"time"

	onbMongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	txMongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	onbRedis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/onboarding"
	txRedis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
)

// Compile-time interface verification.

// UseCase is a struct that aggregates all repositories for both onboarding and transaction
// domains, providing simplified access in use case implementations.
type UseCase struct {
	// --- Onboarding domain repos ---

	// OrganizationRepo provides an abstraction on top of the organization data source.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	LedgerRepo ledger.Repository

	// SegmentRepo provides an abstraction on top of the segment data source.
	SegmentRepo segment.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	PortfolioRepo portfolio.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	AccountRepo account.Repository

	// AssetRepo provides an abstraction on top of the asset data source.
	AssetRepo asset.Repository

	// AccountTypeRepo provides an abstraction on top of the account type data source.
	AccountTypeRepo accounttype.Repository

	// --- Transaction domain repos ---

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

	// --- MongoDB (separate per domain) ---

	// OnboardingMetadataRepo provides an abstraction on top of the onboarding metadata data source.
	OnboardingMetadataRepo onbMongo.Repository

	// TransactionMetadataRepo provides an abstraction on top of the transaction metadata data source.
	TransactionMetadataRepo txMongo.Repository

	// --- Redis (separate per domain) ---

	// OnboardingRedisRepo provides an abstraction on top of the onboarding redis consumer.
	OnboardingRedisRepo onbRedis.RedisRepository

	// TransactionRedisRepo provides an abstraction on top of the transaction redis consumer.
	TransactionRedisRepo txRedis.RedisRepository

	// --- RabbitMQ ---

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// --- Settings ---

	// SettingsCacheTTL is the TTL for cached ledger settings.
	// If zero, defaults to DefaultSettingsCacheTTL (5 minutes).
	SettingsCacheTTL time.Duration
}
