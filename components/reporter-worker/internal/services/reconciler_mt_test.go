// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// mockTenantLister is a test double for the TenantLister interface.
type mockTenantLister struct {
	tenants []*tmclient.TenantSummary
	err     error
}

func (m *mockTenantLister) GetActiveTenantsByService(
	_ context.Context,
	_ string,
) ([]*tmclient.TenantSummary, error) {
	return m.tenants, m.err
}

func TestReconciler_MultiTenant_IteratesAllTenants(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	tenantLister := &mockTenantLister{
		tenants: []*tmclient.TenantSummary{
			{ID: "tenant-1", Name: "Acme Corp", Status: "active"},
			{ID: "tenant-2", Name: "Beta Inc", Status: "active"},
		},
	}
	r.tenantLister = tenantLister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	staleMapping := &datasource.ExtractionMapping{
		JobID:     "job-stale-1",
		ReportID:  "report-stale-1",
		Status:    "pending",
		CreatedAt: time.Now().Add(-20 * time.Minute),
	}

	// Should be called once per tenant
	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil).Times(1)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil).Times(1)

	// For each tenant context, find stale mappings
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil).Times(2)

	// For each stale mapping found, poll Fetcher
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "job-stale-1").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "job-stale-1",
			Status: "pending",
		}, nil).Times(2)

	r.reconcile(context.Background())
}

func TestReconciler_MultiTenant_TenantContextInjected(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	tenantLister := &mockTenantLister{
		tenants: []*tmclient.TenantSummary{
			{ID: "tenant-ctx-check", Name: "CTX Corp", Status: "active"},
		},
	}
	r.tenantLister = tenantLister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// Verify that the context passed to FindStalePending contains the tenant ID
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ time.Duration) ([]*datasource.ExtractionMapping, error) {
			tenantID := tmcore.GetTenantIDContext(ctx)
			assert.Equal(t, "tenant-ctx-check", tenantID,
				"reconcile must inject tenant ID into context")
			return nil, nil
		})

	r.reconcile(context.Background())
}

func TestReconciler_MultiTenant_TenantListError_LogsAndContinues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	r, _, mockRedis, _, _ := newTestReconciler(t, ctrl)

	tenantLister := &mockTenantLister{
		err: errors.New("tenant manager unavailable"),
	}
	r.tenantLister = tenantLister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// Should not panic — error is logged and reconcile exits gracefully
	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_SingleTenant_NoTenantIteration(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// multiTenantEnabled = false (default)
	// tenantLister = nil (default)

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// Single-tenant: FindStalePending is called once directly
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

	r.reconcile(context.Background())
}
