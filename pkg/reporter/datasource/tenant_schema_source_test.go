// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libMongo "go.mongodb.org/mongo-driver/v2/mongo"
)

// fakeTenantPGManager records which tenant it was asked to resolve and returns
// a configurable connection/error. It NEVER substitutes another tenant — the
// test asserts the recorded tenant equals the context tenant, proving isolation.
type fakeTenantPGManager struct {
	db        dbresolver.DB
	err       error
	gotTenant string
	calls     int
}

func (m *fakeTenantPGManager) GetDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	m.calls++
	m.gotTenant = tenantID

	if m.err != nil {
		return nil, m.err
	}

	return m.db, nil
}

// fakeTenantMongoManager mirrors fakeTenantPGManager for the Mongo seam.
type fakeTenantMongoManager struct {
	db        *libMongo.Database
	err       error
	gotTenant string
	calls     int
}

func (m *fakeTenantMongoManager) GetDatabaseForTenant(_ context.Context, tenantID string) (*libMongo.Database, error) {
	m.calls++
	m.gotTenant = tenantID

	if m.err != nil {
		return nil, m.err
	}

	return m.db, nil
}

// ctxWithTenant stamps a tenant ID onto the context via the lib-commons helper,
// the same path the manager's TenantMiddleware uses on a real request.
func ctxWithTenant(tenantID string) context.Context {
	return tmcore.ContextWithTenantID(context.Background(), tenantID)
}

// --- Isolation: the resolved pool is the CONTEXT tenant's pool, only ---------

func TestTenantSchemaSource_PostgresResolvesContextTenant(t *testing.T) {
	t.Parallel()

	pg := &fakeTenantPGManager{err: errors.New("stop before query")}
	src := newTenantSchemaSource(pg, &fakeTenantMongoManager{}, log.NewNop())

	_, err := src.PostgresSchema(ctxWithTenant("tenant-a"), "ledger", []string{"public"})

	require.Error(t, err, "must fail closed when the manager errors")
	assert.Equal(t, "tenant-a", pg.gotTenant,
		"the schema source must resolve ONLY the context tenant's pool")
	assert.Equal(t, 1, pg.calls)
}

func TestTenantSchemaSource_MongoResolvesContextTenant(t *testing.T) {
	t.Parallel()

	mongo := &fakeTenantMongoManager{err: errors.New("stop before discovery")}
	src := newTenantSchemaSource(&fakeTenantPGManager{}, mongo, log.NewNop())

	_, err := src.MongoSchema(ctxWithTenant("tenant-b"), "crm", "")

	require.Error(t, err, "must fail closed when the manager errors")
	assert.Equal(t, "tenant-b", mongo.gotTenant,
		"the schema source must resolve ONLY the context tenant's database")
	assert.Equal(t, 1, mongo.calls)
}

// --- Fail-closed: missing tenant never resolves any pool --------------------

func TestTenantSchemaSource_MissingTenantFailsClosed(t *testing.T) {
	t.Parallel()

	pg := &fakeTenantPGManager{}
	mongo := &fakeTenantMongoManager{}
	src := newTenantSchemaSource(pg, mongo, log.NewNop())

	_, pgErr := src.PostgresSchema(context.Background(), "ledger", nil)
	require.Error(t, pgErr, "missing tenant must fail closed (postgres)")
	assert.Contains(t, pgErr.Error(), "tenant id is required")
	assert.Equal(t, 0, pg.calls, "no pool may be resolved without a tenant")

	_, mongoErr := src.MongoSchema(context.Background(), "crm", "")
	require.Error(t, mongoErr, "missing tenant must fail closed (mongo)")
	assert.Contains(t, mongoErr.Error(), "tenant id is required")
	assert.Equal(t, 0, mongo.calls, "no database may be resolved without a tenant")
}

// --- Fail-closed: malformed tenant is rejected before resolution ------------

func TestTenantSchemaSource_MalformedTenantFailsClosed(t *testing.T) {
	t.Parallel()

	pg := &fakeTenantPGManager{}
	src := newTenantSchemaSource(pg, &fakeTenantMongoManager{}, log.NewNop())

	// A leading hyphen violates the lib-commons tenant-id shape.
	_, err := src.PostgresSchema(ctxWithTenant("-bad-tenant"), "ledger", nil)

	require.Error(t, err, "malformed tenant must fail closed")
	assert.Contains(t, err.Error(), "tenant id is invalid")
	assert.Equal(t, 0, pg.calls, "no pool may be resolved for a malformed tenant")
}

// --- Fail-closed: a manager error never falls back to a shared/other pool ---

func TestTenantSchemaSource_ManagerErrorFailsClosed(t *testing.T) {
	t.Parallel()

	pgErr := errors.New("tenant manager unavailable")
	pg := &fakeTenantPGManager{err: pgErr}
	src := newTenantSchemaSource(pg, &fakeTenantMongoManager{}, log.NewNop())

	schema, err := src.PostgresSchema(ctxWithTenant("tenant-c"), "ledger", []string{"public"})

	require.Error(t, err, "a resolution error must fail closed, not fall back")
	assert.Nil(t, schema, "no schema may be returned on a resolution failure")
	assert.ErrorIs(t, err, pgErr, "the underlying manager error must be propagated")
}

// --- Fail-closed: a nil resolved connection is rejected at the seam ----------

func TestTenantSchemaSource_NilConnectionFailsClosed(t *testing.T) {
	t.Parallel()

	// Manager returns (nil, nil): no error, but a nil connection. The seam must
	// catch this rather than nil-deref on the first query.
	pg := &fakeTenantPGManager{db: nil, err: nil}
	src := newTenantSchemaSource(pg, &fakeTenantMongoManager{}, log.NewNop())

	_, err := src.PostgresSchema(ctxWithTenant("tenant-d"), "ledger", nil)

	require.Error(t, err, "a nil resolved connection must fail closed")
	assert.Contains(t, err.Error(), "nil connection")

	mongo := &fakeTenantMongoManager{db: nil, err: nil}
	mongoSrc := newTenantSchemaSource(&fakeTenantPGManager{}, mongo, log.NewNop())

	_, mongoErr := mongoSrc.MongoSchema(ctxWithTenant("tenant-d"), "crm", "")
	require.Error(t, mongoErr, "a nil resolved database must fail closed")
	assert.Contains(t, mongoErr.Error(), "nil database")
}
