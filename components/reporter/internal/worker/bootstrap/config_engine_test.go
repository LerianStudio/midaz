// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	reporterEngine "github.com/LerianStudio/midaz/v4/pkg/reporter/engine"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	clog "github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libMongo "go.mongodb.org/mongo-driver/v2/mongo"
)

// noopMongoManager is a non-nil engine.MongoManager that is never exercised. It
// exists so a postgres-only resolver test can satisfy NewMultiTenantResolver's
// fail-closed nil-manager guard without standing up a real mongo manager.
type noopMongoManager struct{}

func (noopMongoManager) GetDatabaseForTenant(context.Context, string) (*libMongo.Database, error) {
	return nil, errors.New("noopMongoManager must never be called")
}

// stubConnector is a database/sql/driver.Connector that never dials. It exists
// so a test can build a real *sql.DB handle (for dbresolver.New) without opening
// a network connection — no query is ever issued against it.
type stubConnector struct{}

func (stubConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("stub connector must never dial")
}

func (stubConnector) Driver() driver.Driver { return stubDriver{} }

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("stub driver must never open")
}

func engineTestConfig() *Config {
	return &Config{
		OtelLibraryName:              "reporter-worker-test",
		EngineMaxDatasources:         5,
		EngineMaxTablesPerDatasource: 20,
		EngineMaxFieldsPerTable:      100,
		EngineMaxConcurrency:         2,
		EngineTimeoutSec:             120,
		EngineMaxResultBytes:         1024,
	}
}

func TestInitWorkerEngine_SingleTenant_SucceedsWithWiredPorts(t *testing.T) {
	t.Parallel()

	cfg := engineTestConfig()
	logger := clog.NewNop()
	ds := pkg.NewSafeDataSources(map[string]pkg.DataSource{})

	engine, err := initWorkerEngine(cfg, logger, nil, ds, pkg.NewCircuitBreakerManager(logger), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, engine)

	// Limits are sourced from config, not left at raw DefaultLimits.
	limits := engine.Limits()
	assert.Equal(t, 5, limits.MaxDatasources)
	assert.Equal(t, 20, limits.MaxTablesPerDatasource)
	assert.Equal(t, 100, limits.MaxFieldsPerTable)
	assert.Equal(t, 2, limits.MaxConcurrency)
	assert.Equal(t, 120*time.Second, limits.Timeout)
	assert.Equal(t, int64(1024), limits.MaxResultBytes)
}

func TestInitWorkerEngine_FailsFastOnNilRegistry(t *testing.T) {
	t.Parallel()

	// engine.New IS the fail-fast: a nil ConnectorRegistry must yield a typed
	// CategoryValidation *EngineError at construction, not a deferred panic. This
	// is the contract initWorkerEngine relies on to abort bootstrap.
	_, err := fetcherEngine.New(
		fetcherEngine.WithConnectorRegistry(nil),
		fetcherEngine.WithLimits(fetcherEngine.DefaultLimits()),
	)
	require.Error(t, err)

	var engineErr *fetcherEngine.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcherEngine.CategoryValidation, engineErr.Category)
}

func TestInitWorkerEngine_NoEncryptedPersistence(t *testing.T) {
	t.Parallel()

	// initWorkerEngine must never enable encrypted persistence: doing so would
	// require a CredentialProtector, and engine.New would fail validation when
	// none is supplied. A successful construction with no protector proves
	// neither WithEncryptedPersistence(true) nor WithCredentialProtector is wired.
	cfg := engineTestConfig()
	logger := clog.NewNop()
	ds := pkg.NewSafeDataSources(map[string]pkg.DataSource{})

	engine, err := initWorkerEngine(cfg, logger, nil, ds, pkg.NewCircuitBreakerManager(logger), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, engine)
}

// fakeTenantPGManager stands in for *tmpostgres.Manager at the tenantPGManager
// seam. It records the exact tenant ID it was asked to resolve and returns a
// per-tenant dbresolver.DB (or an error), so a test can prove the adapter
// forwards the tenant verbatim and never substitutes a different connection.
type fakeTenantPGManager struct {
	dbByTenant map[string]dbresolver.DB
	err        error
	gotTenant  string
	callCount  int
}

func (m *fakeTenantPGManager) GetDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	m.gotTenant = tenantID
	m.callCount++

	if m.err != nil {
		return nil, m.err
	}

	return m.dbByTenant[tenantID], nil
}

// newFakeResolverDB builds a real dbresolver.DB over a never-dialed *sql.DB so
// the test has a distinct, comparable connection handle per tenant without
// opening a network connection. No query is ever issued against it.
func newFakeResolverDB(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB := sql.OpenDB(stubConnector{})
	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}

func TestTenantPostgresAdapter_ForwardsExactTenantAndReturnsItsDB(t *testing.T) {
	t.Parallel()

	// The adapter must forward the precise tenant ID to the lib-commons manager
	// and hand back exactly that tenant's connection — never a shared or
	// cross-tenant pool. Two tenants, two distinct DBs, each resolves only its own.
	dbA := newFakeResolverDB(t)
	dbB := newFakeResolverDB(t)

	mgr := &fakeTenantPGManager{dbByTenant: map[string]dbresolver.DB{
		"tenant-a": dbA,
		"tenant-b": dbB,
	}}
	adapter := newTenantPostgresAdapter(mgr)

	gotA, err := adapter.GetDB(context.Background(), "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", mgr.gotTenant, "adapter must forward the exact tenant ID")
	assert.Same(t, dbA, gotA, "tenant-a must resolve its own DB, never another tenant's")

	gotB, err := adapter.GetDB(context.Background(), "tenant-b")
	require.NoError(t, err)
	assert.Equal(t, "tenant-b", mgr.gotTenant)
	assert.Same(t, dbB, gotB, "tenant-b must resolve its own DB")

	assert.NotSame(t, gotA, gotB, "the two tenants must not share a connection")
}

func TestTenantPostgresAdapter_PropagatesManagerErrorWithoutSharedPoolFallback(t *testing.T) {
	t.Parallel()

	// On a manager resolution error the adapter must fail closed for that request:
	// propagate the error and NEVER substitute a shared single-tenant pool or any
	// other tenant's connection.
	sentinel := errors.New("tenant credentials unavailable")
	mgr := &fakeTenantPGManager{err: sentinel}
	adapter := newTenantPostgresAdapter(mgr)

	db, err := adapter.GetDB(context.Background(), "tenant-x")
	assert.Nil(t, db, "adapter must not return any connection on error")
	require.ErrorIs(t, err, sentinel, "the manager error must propagate, not be swallowed")
	assert.Equal(t, "tenant-x", mgr.gotTenant)
}

func TestTenantPostgresAdapter_ViaMultiTenantResolver_PropagatesError(t *testing.T) {
	t.Parallel()

	// End-to-end through the engine resolver: a manager error surfaces as a typed
	// CategoryUnavailable *EngineError and the request is refused — it does not
	// fall through to any single-tenant pool.
	mgr := &fakeTenantPGManager{err: errors.New("pool exhausted")}
	resolver := reporterEngine.NewMultiTenantResolver(newTenantPostgresAdapter(mgr), noopMongoManager{}, nil)

	_, err := resolver.ResolvePostgres(context.Background(), "tenant-x", "ledger")
	var engineErr *fetcherEngine.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcherEngine.CategoryUnavailable, engineErr.Category)
	assert.Equal(t, "tenant-x", mgr.gotTenant, "the resolver must forward the validated tenant ID")
}
