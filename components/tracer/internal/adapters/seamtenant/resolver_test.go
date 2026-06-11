// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/DATA-DOG/go-sqlmock"
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

func TestResolver_NoOpWhenSingleTenant(t *testing.T) {
	// mtEnabled=false ⇒ no-op even with a pool wired; the key is ignored.
	called := false
	pool := func(context.Context, string) (dbresolver.DB, error) {
		called = true
		return nil, nil
	}

	r := NewResolverWithPool(pool, false)
	require.False(t, r.Active())

	ctx := context.Background()

	out, err := r.Resolve(ctx, testTenantID)
	require.NoError(t, err)
	require.Equal(t, ctx, out)
	require.False(t, called, "pool must not be resolved in single-tenant mode")
	require.Empty(t, tmcore.GetTenantIDContext(out))
	require.Nil(t, tmcore.GetPGContext(out))
}

func TestResolver_NoOpWhenNilManager(t *testing.T) {
	// A nil pool (nil manager) is single-tenant mode regardless of mtEnabled.
	r := NewResolver(nil, true)
	require.False(t, r.Active())

	ctx := context.Background()

	out, err := r.Resolve(ctx, "")
	require.NoError(t, err)
	require.Equal(t, ctx, out)
}

func TestResolver_PresentKeyResolvesAndBindsPool(t *testing.T) {
	stub := newStubDB(t)

	var gotTenant string

	pool := func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		gotTenant = tenantID
		return stub, nil
	}

	r := NewResolverWithPool(pool, true)
	require.True(t, r.Active())

	out, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)

	// Tenant id and the resolved pool are bound to ctx via the manager keys.
	require.Equal(t, testTenantID, gotTenant)
	require.Equal(t, testTenantID, tmcore.GetTenantIDContext(out))
	require.Equal(t, stub, tmcore.GetPGContext(out))
}

func TestResolver_MissingKeyUnderMTFailsClean(t *testing.T) {
	called := false
	pool := func(context.Context, string) (dbresolver.DB, error) {
		called = true
		return newStubDB(t), nil
	}

	r := NewResolverWithPool(pool, true)

	out, err := r.Resolve(context.Background(), "")
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	require.False(t, called, "missing key must never resolve a pool")
	// No tenant/pool bound — never a default/wrong pool.
	require.Empty(t, tmcore.GetTenantIDContext(out))
	require.Nil(t, tmcore.GetPGContext(out))
}

func TestResolver_InvalidKeyUnderMTFailsClean(t *testing.T) {
	pool := func(context.Context, string) (dbresolver.DB, error) {
		return newStubDB(t), nil
	}

	r := NewResolverWithPool(pool, true)

	out, err := r.Resolve(context.Background(), "bad tenant id!")
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	require.Nil(t, tmcore.GetPGContext(out))
}

func TestResolver_PoolErrorPropagates(t *testing.T) {
	sentinel := errors.New("pool down")
	pool := func(context.Context, string) (dbresolver.DB, error) {
		return nil, sentinel
	}

	r := NewResolverWithPool(pool, true)

	out, err := r.Resolve(context.Background(), testTenantID)
	require.ErrorIs(t, err, sentinel)
	// On a resolution failure no pool is bound (technical error, not a fallback).
	require.Nil(t, tmcore.GetPGContext(out))
}
