// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// fakeFeesDBResolver returns a per-tenant *mongo.Database handle, modelling the
// tenant-manager Mongo manager's GetDatabaseForTenant without a live connection.
type fakeFeesDBResolver struct {
	dbs map[string]*mongo.Database
	err error
}

func (f *fakeFeesDBResolver) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.dbs[tenantID], nil
}

// newDisconnectedFeeDatabase builds a *mongo.Database handle without dialling.
// mongo.Connect is lazy: no network call happens until a query runs, so a named
// database handle is pure in-memory metadata — enough to prove two tenants
// resolve to DISTINCT databases through the seam.
func newDisconnectedFeeDatabase(t *testing.T, dbName string) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	return client.Database(dbName)
}

// TestResolveFeesTenantContext_TwoTenantsResolveDifferentDatabases is the F1
// regression guard: two different tenants MUST resolve to different fee Mongo
// databases on the GENERIC MB key, and the request ctx MUST stay untouched.
func TestResolveFeesTenantContext_TwoTenantsResolveDifferentDatabases(t *testing.T) {
	dbA := newDisconnectedFeeDatabase(t, "fees_tenant_a")
	dbB := newDisconnectedFeeDatabase(t, "fees_tenant_b")

	handler := &TransactionHandler{
		MultiTenantEnabled: true,
		FeesMongoManager: &fakeFeesDBResolver{dbs: map[string]*mongo.Database{
			"tenant-a": dbA,
			"tenant-b": dbB,
		}},
	}

	reqCtxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	derivedA, err := handler.resolveFeesTenantContext(reqCtxA)
	require.NoError(t, err)

	reqCtxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")
	derivedB, err := handler.resolveFeesTenantContext(reqCtxB)
	require.NoError(t, err)

	gotA := tmcore.GetMBContext(derivedA)
	gotB := tmcore.GetMBContext(derivedB)

	require.NotNil(t, gotA)
	require.NotNil(t, gotB)
	assert.Same(t, dbA, gotA, "tenant-a must resolve to its own fee DB on the generic key")
	assert.Same(t, dbB, gotB, "tenant-b must resolve to its own fee DB on the generic key")
	assert.NotSame(t, gotA, gotB, "two tenants must NOT share a fee database")

	// Request ctx untouched: the generic MB key must NOT leak onto the caller's ctx.
	assert.Nil(t, tmcore.GetMBContext(reqCtxA), "request ctx must not carry the resolved fee DB")
	assert.Nil(t, tmcore.GetMBContext(reqCtxB), "request ctx must not carry the resolved fee DB")
}

// TestResolveFeesTenantContext_SingleTenantNoOp proves the seam is a no-op when
// multi-tenant is disabled: the static fee connection is correct there, so the
// generic key must NOT be set and the same ctx is returned.
func TestResolveFeesTenantContext_SingleTenantNoOp(t *testing.T) {
	handler := &TransactionHandler{
		MultiTenantEnabled: false,
		FeesMongoManager: &fakeFeesDBResolver{dbs: map[string]*mongo.Database{
			"tenant-a": newDisconnectedFeeDatabase(t, "fees_tenant_a"),
		}},
	}

	reqCtx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	derived, err := handler.resolveFeesTenantContext(reqCtx)
	require.NoError(t, err)
	assert.Equal(t, reqCtx, derived, "single-tenant mode must return the ctx unchanged")
	assert.Nil(t, tmcore.GetMBContext(derived), "single-tenant mode must not set the generic MB key")
}

// TestResolveFeesTenantContext_MissingTenantFailsCleanly proves that when MT is
// enabled but no tenant ID is on the ctx, the seam fails with a typed error
// instead of falling through to the shared single-tenant fee DB.
func TestResolveFeesTenantContext_MissingTenantFailsCleanly(t *testing.T) {
	handler := &TransactionHandler{
		MultiTenantEnabled: true,
		FeesMongoManager:   &fakeFeesDBResolver{dbs: map[string]*mongo.Database{}},
	}

	_, err := handler.resolveFeesTenantContext(context.Background())
	require.Error(t, err, "missing tenant must fail, never fall through to the shared DB")
}

// TestResolveFeesTenantContext_ResolutionErrorMapped proves a resolver failure
// is surfaced (mapped), not swallowed into the shared DB.
func TestResolveFeesTenantContext_ResolutionErrorMapped(t *testing.T) {
	handler := &TransactionHandler{
		MultiTenantEnabled: true,
		FeesMongoManager:   &fakeFeesDBResolver{err: errors.New("tenant manager unreachable")},
	}

	reqCtx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err := handler.resolveFeesTenantContext(reqCtx)
	require.Error(t, err)
}
