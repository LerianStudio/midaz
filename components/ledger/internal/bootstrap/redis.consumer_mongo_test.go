// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type recoveryMongoStub struct {
	database *mongo.Database
	err      error
	calls    int
	tenant   string
	ctx      context.Context
}

func (resolver *recoveryMongoStub) GetDatabaseForTenant(ctx context.Context, tenant string) (*mongo.Database, error) {
	resolver.calls++
	resolver.ctx, resolver.tenant = ctx, tenant
	return resolver.database, resolver.err
}

type recoveryMongoFinalizerStub struct {
	ctx   context.Context
	calls int
	err   error
}

func (finalizer *recoveryMongoFinalizerStub) Finalize(ctx context.Context, _ *command.BalanceEngineRecoveryEnvelope) error {
	finalizer.calls++
	finalizer.ctx = ctx
	return finalizer.err
}

func TestRecoveryMongoPreservesTenantAndTimeout(t *testing.T) {
	resolver := &recoveryMongoStub{database: &mongo.Database{}}
	delegate := &recoveryMongoFinalizerStub{}
	finalizer := &tenantRecoveryFinalizer{delegate: delegate, mongoResolver: resolver, multiTenantEnabled: true}
	ctx, cancel := context.WithTimeout(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), time.Minute)
	defer cancel()
	envelope := &command.BalanceEngineRecoveryEnvelope{TenantID: "tenant-a"}
	require.NoError(t, finalizer.Finalize(ctx, envelope))
	require.Equal(t, 1, resolver.calls)
	require.Equal(t, "tenant-a", resolver.tenant)
	require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(delegate.ctx))
	require.Same(t, resolver.database, tmcore.GetMBContext(delegate.ctx))
	require.Same(t, resolver.database, tmcore.GetMBContext(delegate.ctx, constant.ModuleTransaction))
	require.Equal(t, 1, delegate.calls)
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	resolverDeadline, ok := resolver.ctx.Deadline()
	require.True(t, ok)
	delegateDeadline, ok := delegate.ctx.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, resolverDeadline)
	require.Equal(t, deadline, delegateDeadline)
	cancel()
	require.ErrorIs(t, delegate.ctx.Err(), context.Canceled)
}

func TestRecoveryMongoFailureStopsFinalization(t *testing.T) {
	lookupErr := errors.New("tenant database unavailable")
	for _, scenario := range []string{"missing tenant", "foreign tenant", "missing resolver", "nil database", "lookup error", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			resolver := &recoveryMongoStub{database: &mongo.Database{}}
			delegate := &recoveryMongoFinalizerStub{}
			finalizer := &tenantRecoveryFinalizer{delegate: delegate, mongoResolver: resolver, multiTenantEnabled: true}
			ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")
			envelope := &command.BalanceEngineRecoveryEnvelope{TenantID: "tenant-a"}
			wantCalls := 0
			switch scenario {
			case "missing tenant":
				ctx = t.Context()
			case "foreign tenant":
				envelope.TenantID = "tenant-b"
			case "missing resolver":
				finalizer.mongoResolver = nil
			case "nil database":
				resolver.database = nil
				wantCalls = 1
			case "lookup error":
				resolver.err = lookupErr
				wantCalls = 1
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := finalizer.Finalize(ctx, envelope)
			require.Error(t, err)
			if scenario == "lookup error" {
				require.ErrorIs(t, err, lookupErr)
			}
			require.Equal(t, wantCalls, resolver.calls)
			require.Zero(t, delegate.calls)
		})
	}
}

func TestRecoveryMongoSingleTenantUsesExistingMetadataConnection(t *testing.T) {
	resolver := &recoveryMongoStub{err: errors.New("must not resolve")}
	delegateErr := errors.New("metadata persistence failed")
	delegate := &recoveryMongoFinalizerStub{err: delegateErr}
	finalizer := &tenantRecoveryFinalizer{delegate: delegate, mongoResolver: resolver}
	ctx := t.Context()
	require.ErrorIs(t, finalizer.Finalize(ctx, &command.BalanceEngineRecoveryEnvelope{}), delegateErr)
	require.Zero(t, resolver.calls)
	require.Equal(t, 1, delegate.calls)
	require.Equal(t, ctx, delegate.ctx)
}

func TestRecoveryMongoDoesNotChangeLegacyTenantReadiness(t *testing.T) {
	consumer := &RedisQueueConsumer{
		multiTenantEnabled: true,
		pgManager:          &tmpostgres.Manager{},
		tenantCache:        &tenantcache.TenantCache{},
	}
	consumer.WithBalanceEngineFinalizer(&tenantRecoveryFinalizer{multiTenantEnabled: true})
	require.True(t, consumer.isMultiTenantReady(), "missing recovery Mongo must not select single-tenant dispatch")
}

func TestBalanceRedisToBalancePreservesLegacyReplayProjection(t *testing.T) {
	overdraftLimit := "9876543210.123456789012345678"

	got, err := balanceRedisToBalance(mmodel.BalanceRedis{
		Alias:                 "@cash",
		ID:                    "balance-id",
		AccountID:             "account-id",
		Key:                   "settlement",
		Available:             decimal.RequireFromString("100.000000000000000001"),
		OnHold:                decimal.RequireFromString("2.300000000000000004"),
		Version:               42,
		AccountType:           "deposit",
		AllowSending:          1,
		AllowReceiving:        1,
		AssetCode:             "BRL",
		Direction:             "DEBIT",
		OverdraftUsed:         "0.000000000000000009",
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        overdraftLimit,
		BalanceScope:          "AVAILABLE",
	}, "organization-id", "ledger-id")

	require.NoError(t, err)
	require.Equal(t, "@cash", got.Alias)
	require.Equal(t, "balance-id", got.ID)
	require.Equal(t, "account-id", got.AccountID)
	require.Equal(t, "settlement", got.Key)
	require.True(t, decimal.RequireFromString("100.000000000000000001").Equal(got.Available))
	require.True(t, decimal.RequireFromString("2.300000000000000004").Equal(got.OnHold))
	require.Equal(t, int64(42), got.Version)
	require.Equal(t, "deposit", got.AccountType)
	require.True(t, got.AllowSending)
	require.True(t, got.AllowReceiving)
	require.Equal(t, "BRL", got.AssetCode)
	require.Equal(t, "organization-id", got.OrganizationID)
	require.Equal(t, "ledger-id", got.LedgerID)
	require.Equal(t, "DEBIT", got.Direction)
	require.True(t, decimal.RequireFromString("0.000000000000000009").Equal(got.OverdraftUsed))
	require.NotNil(t, got.Settings)
	require.True(t, got.Settings.AllowOverdraft)
	require.True(t, got.Settings.OverdraftLimitEnabled)
	require.NotNil(t, got.Settings.OverdraftLimit)
	require.Equal(t, overdraftLimit, *got.Settings.OverdraftLimit)
	require.Equal(t, "AVAILABLE", got.Settings.BalanceScope)
}

func TestBalanceRedisToBalanceRejectsInvalidLegacyOverdraftValues(t *testing.T) {
	for name, balance := range map[string]mmodel.BalanceRedis{
		"used":  {OverdraftUsed: "not-a-decimal"},
		"limit": {OverdraftLimit: "not-a-decimal"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := balanceRedisToBalance(balance, "organization-id", "ledger-id")

			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

func TestProcessMessageRejectsInvalidLegacySnapshotsBeforePersistence(t *testing.T) {
	valid := completeLegacyBalanceRedis("valid", "1.000000000000000001")
	invalid := completeLegacyBalanceRedis("invalid", "not-a-decimal")

	for name, message := range map[string]mmodel.TransactionRedisQueue{
		"before": {
			Balances: []mmodel.BalanceRedis{invalid},
			Validate: &mtransaction.Responses{},
		},
		"after": {
			Balances:      []mmodel.BalanceRedis{valid},
			BalancesAfter: []mmodel.BalanceRedis{invalid},
			Validate:      &mtransaction.Responses{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			consumer := &RedisQueueConsumer{Logger: libLog.NewNop()}

			require.NotPanics(t, func() {
				consumer.processMessage(t.Context(), "redis-key", "raw-payload", message)
			})
		})
	}
}

func TestBalanceRedisToBalancePreservesLegacyOverdraftDefaults(t *testing.T) {
	got, err := balanceRedisToBalance(mmodel.BalanceRedis{}, "organization-id", "ledger-id")

	require.NoError(t, err)
	require.Equal(t, constant.DefaultBalanceKey, got.Key)
	require.True(t, got.OverdraftUsed.IsZero())
	require.NotNil(t, got.Settings)
	require.False(t, got.Settings.AllowOverdraft)
	require.False(t, got.Settings.OverdraftLimitEnabled)
	require.Nil(t, got.Settings.OverdraftLimit)
	require.Empty(t, got.Settings.BalanceScope)
}

func completeLegacyBalanceRedis(id, overdraftUsed string) mmodel.BalanceRedis {
	return mmodel.BalanceRedis{
		ID:                    id,
		Available:             decimal.RequireFromString("10.000000000000000001"),
		OnHold:                decimal.RequireFromString("3.000000000000000002"),
		Direction:             "DEBIT",
		OverdraftUsed:         overdraftUsed,
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        "100.000000000000000003",
		BalanceScope:          "internal",
	}
}
