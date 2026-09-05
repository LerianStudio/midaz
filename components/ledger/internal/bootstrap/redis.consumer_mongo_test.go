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
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
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
