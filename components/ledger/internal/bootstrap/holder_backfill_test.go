// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	libLog "github.com/LerianStudio/lib-observability/log"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTenantLister returns a fixed tenant list (or an error) for the MT loop.
type fakeTenantLister struct {
	tenants []*tmclient.TenantSummary
	err     error
}

func (f *fakeTenantLister) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.tenants, nil
}

func newTestLoggerStrict(t *testing.T) libLog.Logger {
	t.Helper()

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentDevelopment, OTelLibraryName: "ledger-test"})
	require.NoError(t, err)

	return logger
}

func TestRun_MT_InvokesRunForTenantPerTenant(t *testing.T) {
	lister := &fakeTenantLister{tenants: []*tmclient.TenantSummary{
		{ID: "tenant-a"},
		{ID: "tenant-b"},
		{ID: "tenant-c"},
	}}

	var seen []string

	r := &HolderBackfillRunner{
		logger:             newTestLoggerStrict(t),
		multiTenantEnabled: true,
		tenantServiceName:  "ledger",
		tenantClient:       lister,
		runForTenantFn: func(_ context.Context, tenantID string) error {
			seen = append(seen, tenantID)

			return nil
		},
	}

	err := r.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-b", "tenant-c"}, seen,
		"every active tenant is processed exactly once, in order")
}

func TestRun_MT_AbortsOnFirstTenantFailure(t *testing.T) {
	lister := &fakeTenantLister{tenants: []*tmclient.TenantSummary{
		{ID: "tenant-a"},
		{ID: "tenant-b"},
		{ID: "tenant-c"},
	}}

	failErr := errors.New("connection resolution failed")

	var seen []string

	r := &HolderBackfillRunner{
		logger:             newTestLoggerStrict(t),
		multiTenantEnabled: true,
		tenantServiceName:  "ledger",
		tenantClient:       lister,
		runForTenantFn: func(_ context.Context, tenantID string) error {
			seen = append(seen, tenantID)
			if tenantID == "tenant-b" {
				return failErr
			}

			return nil
		},
	}

	err := r.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, failErr)
	assert.Contains(t, err.Error(), "tenant-b", "error names the failing tenant")
	assert.Equal(t, []string{"tenant-a", "tenant-b"}, seen,
		"the loop aborts on tenant-b: tenant-c is never processed")
}

func TestRun_MT_SkipsNilTenantEntries(t *testing.T) {
	lister := &fakeTenantLister{tenants: []*tmclient.TenantSummary{
		{ID: "tenant-a"},
		nil,
		{ID: "tenant-c"},
	}}

	var seen []string

	r := &HolderBackfillRunner{
		logger:             newTestLoggerStrict(t),
		multiTenantEnabled: true,
		tenantServiceName:  "ledger",
		tenantClient:       lister,
		runForTenantFn: func(_ context.Context, tenantID string) error {
			seen = append(seen, tenantID)

			return nil
		},
	}

	err := r.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-c"}, seen,
		"the nil tenant entry is skipped, not dereferenced")
}

func TestRun_MT_TenantListFailureAborts(t *testing.T) {
	listErr := errors.New("tenant manager unreachable")
	lister := &fakeTenantLister{err: listErr}

	called := false

	r := &HolderBackfillRunner{
		logger:             newTestLoggerStrict(t),
		multiTenantEnabled: true,
		tenantServiceName:  "ledger",
		tenantClient:       lister,
		runForTenantFn: func(_ context.Context, _ string) error {
			called = true

			return nil
		},
	}

	err := r.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, listErr)
	assert.False(t, called, "no tenant is processed when listing fails")
}
