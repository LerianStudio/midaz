// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/datasource"
	"github.com/LerianStudio/reporter/pkg/fetcher"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testTenantLister implements TenantLister for tests (not build-tag gated).
type testTenantLister struct {
	tenants []*tmclient.TenantSummary
	err     error
}

func (m *testTenantLister) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	return m.tenants, m.err
}

func TestReconciler_ReconcileMultiTenant_IteratesTenants(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	lister := &testTenantLister{
		tenants: []*tmclient.TenantSummary{
			{ID: "tenant-1", Name: "Acme Corp", Status: "active"},
			{ID: "tenant-2", Name: "Beta Inc", Status: "active"},
		},
	}
	r.tenantLister = lister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	staleMapping := &datasource.ExtractionMapping{
		JobID:     "mt-job-1",
		ReportID:  "report-mt-1",
		Status:    "pending",
		CreatedAt: time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// Called once per tenant
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil).Times(2)

	// Fetcher returns pending for each
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "mt-job-1").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "mt-job-1",
			Status: "pending",
		}, nil).Times(2)

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMultiTenant_TenantContextInjected(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	lister := &testTenantLister{
		tenants: []*tmclient.TenantSummary{
			{ID: "tenant-ctx-verify", Name: "CTX Corp", Status: "active"},
		},
	}
	r.tenantLister = lister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ time.Duration) ([]*datasource.ExtractionMapping, error) {
			tenantID := tmcore.GetTenantIDContext(ctx)
			assert.Equal(t, "tenant-ctx-verify", tenantID,
				"reconcile must inject tenant ID into context")
			return nil, nil
		})

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMultiTenant_TenantListError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, _, mockRedis, _, _ := newTestReconciler(t, ctrl)

	lister := &testTenantLister{
		err: errors.New("tenant manager unavailable"),
	}
	r.tenantLister = lister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_ReconcileMultiTenant_NoTenants(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, _, mockRedis, _, _ := newTestReconciler(t, ctrl)

	lister := &testTenantLister{
		tenants: []*tmclient.TenantSummary{},
	}
	r.tenantLister = lister
	r.multiTenantEnabled = true
	r.serviceName = "reporter"

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, "1", gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}
