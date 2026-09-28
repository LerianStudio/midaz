// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const testTenantID = "tenant-007"

// newStubDB builds a real dbresolver.DB over a sqlmock connection so the test
// can assert pool identity through tmcore.GetPGContext without a live database.
func newStubDB(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}

func TestResolver_InactiveWhenSingleTenant(t *testing.T) {
	t.Parallel()

	pool := func(context.Context, string) (dbresolver.DB, error) { return nil, nil }

	require.False(t, NewResolverWithPool(pool, false).Active(), "single-tenant mode ignores the tenant key")
	require.False(t, NewResolver(nil, true).Active(), "a nil manager is single-tenant mode regardless of mtEnabled")
	require.False(t, NewResolverWithPool(nil, true).Active())

	var absent *Resolver
	require.False(t, absent.Active())
	require.True(t, NewResolverWithPool(pool, true).Active())
}

func TestResolver_PresentKeyResolvesPool(t *testing.T) {
	t.Parallel()

	stub := newStubDB(t)

	var gotTenant string

	r := NewResolverWithPool(func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		gotTenant = tenantID
		return stub, nil
	}, true)

	db, err := r.resolvePool(context.Background(), testTenantID)
	require.NoError(t, err)
	require.Equal(t, testTenantID, gotTenant)
	require.Equal(t, stub, db)

	bound := bindTenant(context.Background(), testTenantID, db)
	require.Equal(t, testTenantID, tmcore.GetTenantIDContext(bound))
	require.Equal(t, stub, tmcore.GetPGContext(bound))
}

func TestResolver_MissingOrInvalidKeyNeverResolvesAPool(t *testing.T) {
	t.Parallel()

	for _, tenantID := range []string{"", "bad tenant id!"} {
		called := false
		r := NewResolverWithPool(func(context.Context, string) (dbresolver.DB, error) {
			called = true
			return newStubDB(t), nil
		}, true)

		db, err := r.resolvePool(context.Background(), tenantID)
		require.ErrorIs(t, err, constant.ErrReservationTenantRequired, tenantID)
		require.Nil(t, db)
		require.False(t, called, "an invalid key must never resolve a pool")
	}
}

func TestResolver_PoolErrorClassification(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		poolErr error
		want    error
	}{
		{"tenant not found", fmt.Errorf("failed to get tenant config: %w", tmcore.ErrTenantNotFound), constant.ErrInsufficientPrivileges},
		{"association denied", fmt.Errorf("tenant %s: %w", testTenantID, tmcore.ErrTenantServiceAccessDenied), constant.ErrInsufficientPrivileges},
		{"association suspended", fmt.Errorf("%w: %w", tmcore.ErrTenantServiceAccessDenied, &tmcore.TenantSuspendedError{TenantID: testTenantID, Status: "suspended"}), constant.ErrInsufficientPrivileges},
		{"suspended without access-denied wrap", &tmcore.TenantSuspendedError{TenantID: testTenantID, Status: "purged"}, constant.ErrInsufficientPrivileges},
		{"pool down", errors.New("pool down"), constant.ErrTenantServiceUnavailable},
		{"breaker open", tmcore.ErrCircuitBreakerOpen, constant.ErrTenantServiceUnavailable},
		{"service not configured", tmcore.ErrServiceNotConfigured, constant.ErrTenantServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := NewResolverWithPool(func(context.Context, string) (dbresolver.DB, error) {
				return nil, tc.poolErr
			}, true)

			db, err := r.resolvePool(context.Background(), testTenantID)
			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, err, tc.poolErr, "the cause stays in the chain for span attribution")
			require.NotErrorIs(t, err, constant.ErrContextPolicyUnavailable, "a pool outcome is never a policy configuration defect")
			require.Nil(t, db)
		})
	}
}
