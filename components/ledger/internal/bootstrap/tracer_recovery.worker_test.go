// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func recoveryWorkerConfig() TracerRecoveryWorkerConfig {
	return TracerRecoveryWorkerConfig{MultiTenant: true, Service: "ledger", Interval: time.Second, CycleTimeout: time.Second, TenantTimeout: time.Second, MaxTenants: 1, MaxCatalogTenants: 10}
}

func TestTracerRecoveryWorkerContinuesAfterCyclePanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	processor := NewMocktracerRecoveryProcessor(ctrl)
	cfg := recoveryWorkerConfig()
	cfg.MultiTenant = false
	cfg.Interval = time.Millisecond
	worker, err := NewTracerRecoveryWorker(processor, nil, nil, cfg, libLog.NewNop())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	first := processor.EXPECT().RunOnce(gomock.Any()).DoAndReturn(func(context.Context) (command.TracerRecoverySummary, error) {
		panic("injected processor panic")
	})
	delivered := false
	processor.EXPECT().RunOnce(gomock.Any()).After(first).DoAndReturn(func(context.Context) (command.TracerRecoverySummary, error) {
		delivered = true
		cancel()
		return command.TracerRecoverySummary{Delivered: 1}, nil
	})
	require.NoError(t, worker.run(ctx))
	require.True(t, delivered, "the next cycle must deliver retained work after a panic")
}

func recoveryTestPool(t *testing.T) dbresolver.DB {
	t.Helper()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return dbresolver.New(dbresolver.WithPrimaryDBs(db))
}

func TestTracerRecoveryWorkerDiscoversWithoutLocalCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	processor, catalog, resolver := NewMocktracerRecoveryProcessor(ctrl), NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
	worker, err := NewTracerRecoveryWorker(processor, catalog, resolver, recoveryWorkerConfig(), libLog.NewNop())
	require.NoError(t, err)
	catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{{ID: "tenant-b", Status: "active"}, {ID: "tenant-a", Status: "active"}}, nil).Times(3)
	for _, tenant := range []string{"tenant-a", "tenant-b", "tenant-a"} {
		pool := recoveryTestPool(t)
		resolve := resolver.EXPECT().GetDB(gomock.Any(), tenant).DoAndReturn(func(ctx context.Context, id string) (dbresolver.DB, error) {
			require.Equal(t, id, tmcore.GetTenantIDContext(ctx))
			return pool, nil
		})
		processor.EXPECT().RunOnce(gomock.Any()).After(resolve).DoAndReturn(func(ctx context.Context) (command.TracerRecoverySummary, error) {
			require.Equal(t, tenant, tmcore.GetTenantIDContext(ctx))
			require.Same(t, pool, tmcore.GetPGContext(ctx, constant.ModuleTransaction))
			_, bounded := ctx.Deadline()
			require.True(t, bounded)
			return command.TracerRecoverySummary{Claimed: 1, Delivered: 1}, nil
		})
		summary, err := worker.runCycle(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, summary.Delivered)
	}
}

func TestTracerRecoveryWorkerContinuesAfterTenantPoolFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	processor, catalog, resolver := NewMocktracerRecoveryProcessor(ctrl), NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
	cfg := recoveryWorkerConfig()
	cfg.MaxTenants = 2
	worker, err := NewTracerRecoveryWorker(processor, catalog, resolver, cfg, libLog.NewNop())
	require.NoError(t, err)
	catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{{ID: "tenant-a", Status: "active"}, {ID: "tenant-b", Status: "active"}}, nil)
	failure := errors.New("pool unavailable")
	first := resolver.EXPECT().GetDB(gomock.Any(), "tenant-a").Return(nil, failure)
	pool := recoveryTestPool(t)
	second := resolver.EXPECT().GetDB(gomock.Any(), "tenant-b").After(first).Return(pool, nil)
	processor.EXPECT().RunOnce(gomock.Any()).After(second).Return(command.TracerRecoverySummary{Delivered: 1}, nil)
	summary, err := worker.runCycle(t.Context())
	require.ErrorIs(t, err, failure)
	require.Equal(t, 1, summary.Delivered)
}

func TestTracerRecoveryWorkerRejectsIncompleteCatalog(t *testing.T) {
	for _, entries := range [][]*tmclient.TenantSummary{{nil}, {{ID: "tenant-a", Status: "suspended"}}, {{ID: "tenant-a", Status: "active"}, {ID: "tenant-a", Status: "active"}}} {
		ctrl := gomock.NewController(t)
		processor, catalog, resolver := NewMocktracerRecoveryProcessor(ctrl), NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
		worker, err := NewTracerRecoveryWorker(processor, catalog, resolver, recoveryWorkerConfig(), libLog.NewNop())
		require.NoError(t, err)
		catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return(entries, nil)
		_, err = worker.runCycle(t.Context())
		require.Error(t, err)
	}
}

func TestTracerRecoveryWorkerSingleTenantAndShutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	processor := NewMocktracerRecoveryProcessor(ctrl)
	cfg := recoveryWorkerConfig()
	cfg.MultiTenant = false
	worker, err := NewTracerRecoveryWorker(processor, nil, nil, cfg, libLog.NewNop())
	require.NoError(t, err)
	processor.EXPECT().RunOnce(gomock.Any()).Return(command.TracerRecoverySummary{Claimed: 2, Delivered: 2}, nil)
	summary, err := worker.runCycle(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, summary.Delivered)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, worker.run(ctx), "canceled startup starts no recovery")
}

func TestTracerRecoveryWorkerBoundsEachTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	processor, catalog, resolver := NewMocktracerRecoveryProcessor(ctrl), NewMocktracerRecoveryCatalog(ctrl), NewMocktracerRecoveryPoolResolver(ctrl)
	cfg := recoveryWorkerConfig()
	cfg.MaxTenants, cfg.TenantTimeout = 2, 5*time.Millisecond
	worker, err := NewTracerRecoveryWorker(processor, catalog, resolver, cfg, libLog.NewNop())
	require.NoError(t, err)
	catalog.EXPECT().GetActiveTenantsByService(gomock.Any(), "ledger").Return([]*tmclient.TenantSummary{{ID: "tenant-a", Status: "active"}, {ID: "tenant-b", Status: "active"}}, nil)
	pool := recoveryTestPool(t)
	resolver.EXPECT().GetDB(gomock.Any(), gomock.Any()).Return(pool, nil).Times(2)
	first := processor.EXPECT().RunOnce(gomock.Any()).DoAndReturn(func(ctx context.Context) (command.TracerRecoverySummary, error) {
		require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(ctx))
		<-ctx.Done()
		return command.TracerRecoverySummary{}, ctx.Err()
	})
	processor.EXPECT().RunOnce(gomock.Any()).After(first).DoAndReturn(func(ctx context.Context) (command.TracerRecoverySummary, error) {
		require.Equal(t, "tenant-b", tmcore.GetTenantIDContext(ctx))
		require.NoError(t, ctx.Err())
		return command.TracerRecoverySummary{Delivered: 1}, nil
	})
	summary, err := worker.runCycle(t.Context())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, summary.Delivered)
}
