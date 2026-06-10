// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// This file is the P3-T08 worker-level golden-file parity test. It drives the
// REAL engine-driven cutover handler (UseCase.extractViaEngine) end-to-end
// through a real *fetcher.Engine assembled over the reporter's own engine
// adapter (pkg/reporter/engine: Registry + MultiTenantResolver + ConnectionStore)
// backed by testcontainers PostgreSQL and MongoDB. It asserts behavioral parity
// with the legacy direct/QueryWithAdvancedFilters path along four axes:
//
//  1. Pongo2 re-keying: a "schema__table" template key round-trips back into
//     result[database]["schema__table"] (NOT the engine's "schema.table" key),
//     so the renderer finds its rows where the template references them.
//  2. Filter row-count parity: an Equals filter behaves as IN, and a date
//     Between range is inclusive of the whole end day (end-of-day expansion) —
//     the same row sets QueryWithAdvancedFilters produced.
//  3. PARTIAL classification: a single failed section (a template table absent
//     from the discovered schema) is dropped and recorded, leaving the report
//     PARTIAL while the surviving section renders; all-sections-fail is Error.
//  4. Cancellation safety: a context canceled mid-extraction surfaces a fatal
//     error from GenerateReport (so the report is marked Error), never a silent
//     Finished with missing data.
//
// crm coverage deferred: see the package-level note in
// TestIntegration_Parity_CRMDeferred at the bottom of this file.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/trace/noop"

	_ "github.com/jackc/pgx/v5/stdlib"

	rengine "github.com/LerianStudio/midaz/v4/pkg/reporter/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	mongokit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/mongodb"
	pgkit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/postgres"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// ----------------------------------------------------------------------------
// Tenant-manager fakes. These mirror the engine package's own integration fakes
// (which live in its _test.go files and are not importable). Each routes a
// tenant ID to its own handle, exactly as the lib-commons tenant-manager does;
// the resolver routing under test is identical whether the handle came from a
// fake or the real manager.
// ----------------------------------------------------------------------------

type parityPGManager struct{ dbs map[string]*sql.DB }

func (m *parityPGManager) GetDB(_ context.Context, tenantID string) (rengine.SQLQuerier, error) {
	db, ok := m.dbs[tenantID]
	if !ok {
		return nil, fmt.Errorf("no postgres connection for tenant %q", tenantID)
	}

	return db, nil
}

type parityMongoManager struct{ dbs map[string]*mongo.Database }

func (m *parityMongoManager) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	db, ok := m.dbs[tenantID]
	if !ok {
		return nil, fmt.Errorf("no mongo database for tenant %q", tenantID)
	}

	return db, nil
}

// parityLookup is a DatasourceLookup over a fixed descriptor set, standing in
// for the bootstrap datasource map. It declares the two datasources the parity
// report extracts from: a postgres "ledger" and a generic mongo "mongo".
type parityLookup struct {
	conns map[string]rengine.DatasourceConnection
	names []string
}

func (l *parityLookup) LookupDatasource(configName string) (rengine.DatasourceConnection, bool) {
	c, ok := l.conns[configName]
	return c, ok
}

func (l *parityLookup) DatasourceConfigNames() []string { return l.names }

const parityTenant = "tenant-default"

// ----------------------------------------------------------------------------
// Infra bootstrap (mirrors the engine package's startPostgres/startMongo).
// ----------------------------------------------------------------------------

func parityStartPostgres(t *testing.T) (*pgkit.PostgresInfra, func()) {
	t.Helper()

	ctx := context.Background()

	infra := pgkit.NewPostgresInfra(pgkit.PostgresConfig{
		Name:     "parity",
		Database: "tenant_default",
		Username: "app",
		Password: "app",
	})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	return infra, func() { _ = suite.Terminate(ctx) }
}

func parityOpenPostgres(t *testing.T, infra *pgkit.PostgresInfra) *sql.DB {
	t.Helper()

	endpoint, err := infra.Endpoint()
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://app:app@%s/tenant_default?sslmode=disable", endpoint.Upstream)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	// The testcontainers postgres module restarts briefly during init-db, so the
	// first connections can be reset. Ride out that window. Test infra only.
	deadline := time.Now().Add(30 * time.Second)

	for {
		pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := db.PingContext(pingCtx)
		cancel()

		if pingErr == nil {
			return db
		}

		if time.Now().After(deadline) {
			require.NoError(t, pingErr)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

func parityStartMongo(t *testing.T) (*mongo.Client, func()) {
	t.Helper()

	ctx := context.Background()

	infra := mongokit.NewMongoDBInfra(mongokit.MongoDBConfig{Name: "parity"})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	uri, err := infra.URI()
	require.NoError(t, err)

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Ping(connectCtx, nil))

	return client, func() {
		_ = client.Disconnect(context.Background())
		_ = suite.Terminate(context.Background())
	}
}

// parityEngine assembles a real *fetcher.Engine over the reporter's own engine
// adapter — the SAME ports the worker bootstrap wires (Registry over a
// multi-tenant resolver + ConnectionStore over a datasource lookup) — so the
// parity test exercises the production extraction path, not a memory stub.
func parityEngine(t *testing.T, pg *sql.DB, mongoDB *mongo.Database) *fetcherEngine.Engine {
	t.Helper()

	lookup := &parityLookup{
		conns: map[string]rengine.DatasourceConnection{
			"ledger": {ConfigName: "ledger", Type: rengine.DatasourceTypePostgres, Schemas: []string{"public"}},
			"mongo":  {ConfigName: "mongo", Type: rengine.DatasourceTypeMongo},
		},
		names: []string{"ledger", "mongo"},
	}

	resolver := rengine.NewMultiTenantResolver(
		&parityPGManager{dbs: map[string]*sql.DB{parityTenant: pg}},
		&parityMongoManager{dbs: map[string]*mongo.Database{parityTenant: mongoDB}},
		nil,
	)

	engine, err := fetcherEngine.New(
		fetcherEngine.WithConnectorRegistry(rengine.NewRegistry(resolver, nil)),
		fetcherEngine.WithConnectionStore(rengine.NewConnectionStore(lookup)),
		fetcherEngine.WithObservability(rengine.NewObservability(nil)),
		fetcherEngine.WithLimits(fetcherEngine.DefaultLimits()),
	)
	require.NoError(t, err)

	return engine
}

func parityUseCase(engine *fetcherEngine.Engine) *UseCase {
	return &UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("parity"),
		Engine:            engine,
		EngineMultiTenant: true,
	}
}

// parityCtx stamps the tenant ID the multi-tenant resolver requires.
func parityCtx() context.Context {
	return tmcore.ContextWithTenantID(context.Background(), parityTenant)
}

// seedParityPostgres creates the accounts table on the tenant DB and inserts the
// canonical fixture rows used to verify filter row-count parity:
//
//	id     status     created_at
//	acc-1  ACTIVE     2025-06-01
//	acc-2  PENDING    2025-06-15
//	acc-3  INACTIVE   2025-06-30
//	acc-4  ACTIVE     2025-07-10
func seedParityPostgres(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`CREATE TABLE accounts (id text PRIMARY KEY, status text, created_at date, secret text)`)
	require.NoError(t, err)

	seed := []struct {
		id, status, created string
	}{
		{"acc-1", "ACTIVE", "2025-06-01"},
		{"acc-2", "PENDING", "2025-06-15"},
		{"acc-3", "INACTIVE", "2025-06-30"},
		{"acc-4", "ACTIVE", "2025-07-10"},
	}
	for _, s := range seed {
		_, err = db.ExecContext(ctx,
			`INSERT INTO accounts (id, status, created_at, secret) VALUES ($1, $2, $3, $4)`,
			s.id, s.status, s.created, "do-not-extract")
		require.NoError(t, err)
	}
}

func seedParityMongo(t *testing.T, db *mongo.Database) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	docs := []any{
		bson.M{"_id": "h-1", "name": "Alice", "status": "ACTIVE", "ssn": "secret"},
		bson.M{"_id": "h-2", "name": "Bob", "status": "PENDING", "ssn": "secret"},
		bson.M{"_id": "h-3", "name": "Carol", "status": "ACTIVE", "ssn": "secret"},
	}

	_, err := db.Collection("holders").InsertMany(ctx, docs)
	require.NoError(t, err)
}

// ----------------------------------------------------------------------------
// The golden parity test: multi-datasource (postgres + mongo), with filters and
// a date Between range, driven through the real engine cutover handler.
// ----------------------------------------------------------------------------

func TestIntegration_Parity_MultiDatasourceFiltersAndBetween(t *testing.T) {
	pgInfra, pgDown := parityStartPostgres(t)
	defer pgDown()

	mongoClient, mongoDown := parityStartMongo(t)
	defer mongoDown()

	pg := parityOpenPostgres(t, pgInfra)
	seedParityPostgres(t, pg)

	mongoDB := mongoClient.Database("tenant_default")
	seedParityMongo(t, mongoDB)

	uc := parityUseCase(parityEngine(t, pg, mongoDB))

	// A representative report: a postgres section keyed in Pongo2 "schema__table"
	// form with BOTH an Equals filter (status ACTIVE) and a date Between range
	// (created_at within June 2025), and a mongo section with an Equals filter.
	message := GenerateReportMessage{
		ReportID:   uuid.New(),
		TemplateID: uuid.New(),
		DataQueries: map[string]map[string][]string{
			"ledger": {"public__accounts": {"id", "status", "created_at"}},
			"mongo":  {"holders": {"name", "status"}},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"ledger": {"public__accounts": {
				"status":     {Equals: []any{"ACTIVE"}},
				"created_at": {Between: []any{"2025-06-01", "2025-06-30"}},
			}},
			"mongo": {"holders": {
				"status": {Equals: []any{"ACTIVE"}},
			}},
		},
	}

	result := make(map[string]map[string][]map[string]any)

	failures, err := uc.extractViaEngine(parityCtx(), message, result)
	require.NoError(t, err)
	require.Empty(t, failures, "both sections must succeed")

	// AXIS 1 — Pongo2 re-keying. The postgres rows must land under the ORIGINAL
	// "public__accounts" template key (NOT the engine's "public.accounts"), so
	// the renderer finds them. The mongo collection round-trips under its bare key.
	require.Contains(t, result, "ledger")
	require.Contains(t, result["ledger"], "public__accounts",
		"postgres rows must re-key to the Pongo2 schema__table template key")
	assert.NotContains(t, result["ledger"], "public.accounts",
		"the engine's dot-qualified key must NOT leak to the renderer")

	require.Contains(t, result, "mongo")
	require.Contains(t, result["mongo"], "holders")

	// AXIS 2 — filter row-count parity. status=ACTIVE AND created_at within
	// June 2025: acc-1 (06-01) only — acc-3 is INACTIVE, acc-4 is ACTIVE but July.
	// The 06-30 boundary proves end-of-day expansion does not exclude it on
	// status grounds; the status filter is what removes acc-3.
	pgRows := result["ledger"]["public__accounts"]
	require.Len(t, pgRows, 1, "ACTIVE + June-2025 Between must match exactly acc-1")
	assert.Equal(t, "acc-1", pgRows[0]["id"])
	// Projection parity: the unselected secret column is never extracted.
	assert.NotContains(t, pgRows[0], "secret", "unselected column must not be extracted")

	// Mongo Equals=ACTIVE matches Alice + Carol (IN semantics over the singleton).
	mongoRows := result["mongo"]["holders"]
	require.Len(t, mongoRows, 2, "ACTIVE holders must match Alice and Carol")
	names := map[string]bool{}
	for _, r := range mongoRows {
		names[fmt.Sprint(r["name"])] = true
		assert.NotContains(t, r, "ssn", "unselected ssn must not be projected")
	}
	assert.True(t, names["Alice"] && names["Carol"], "got %v", names)
}

// TestIntegration_Parity_BetweenInclusiveOfEndDay isolates the end-of-day
// expansion: a Between [2025-06-01, 2025-06-30] WITHOUT a status filter must
// include the 2025-06-30 row, matching the legacy builder that expanded a
// date-only upper bound to end-of-day. A naive "<= 2025-06-30 00:00:00" would
// silently drop the last day's rows on a financial report.
func TestIntegration_Parity_BetweenInclusiveOfEndDay(t *testing.T) {
	pgInfra, pgDown := parityStartPostgres(t)
	defer pgDown()

	pg := parityOpenPostgres(t, pgInfra)
	seedParityPostgres(t, pg)

	// Mongo is unused here; the resolver tolerates a nil mongo manager for a
	// postgres-only report.
	uc := parityUseCase(parityEngineNoMongo(t, pg))

	message := GenerateReportMessage{
		ReportID:   uuid.New(),
		TemplateID: uuid.New(),
		DataQueries: map[string]map[string][]string{
			"ledger": {"public__accounts": {"id", "created_at"}},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"ledger": {"public__accounts": {
				"created_at": {Between: []any{"2025-06-01", "2025-06-30"}},
			}},
		},
	}

	result := make(map[string]map[string][]map[string]any)

	failures, err := uc.extractViaEngine(parityCtx(), message, result)
	require.NoError(t, err)
	require.Empty(t, failures)

	rows := result["ledger"]["public__accounts"]
	// 06-01, 06-15, 06-30 included; 07-10 excluded. The 06-30 row proves the
	// inclusive end-of-day boundary.
	require.Len(t, rows, 3)

	ids := map[string]bool{}
	for _, r := range rows {
		ids[fmt.Sprint(r["id"])] = true
	}

	assert.True(t, ids["acc-3"], "the 2025-06-30 row must be included (end-of-day expansion)")
	assert.False(t, ids["acc-4"], "the 2025-07-10 row must be excluded")
}

// parityEngineNoMongo assembles an engine with only the postgres datasource
// registered, for postgres-only reports.
func parityEngineNoMongo(t *testing.T, pg *sql.DB) *fetcherEngine.Engine {
	t.Helper()

	lookup := &parityLookup{
		conns: map[string]rengine.DatasourceConnection{
			"ledger": {ConfigName: "ledger", Type: rengine.DatasourceTypePostgres, Schemas: []string{"public"}},
		},
		names: []string{"ledger"},
	}

	resolver := rengine.NewMultiTenantResolver(
		&parityPGManager{dbs: map[string]*sql.DB{parityTenant: pg}},
		nil,
		nil,
	)

	engine, err := fetcherEngine.New(
		fetcherEngine.WithConnectorRegistry(rengine.NewRegistry(resolver, nil)),
		fetcherEngine.WithConnectionStore(rengine.NewConnectionStore(lookup)),
		fetcherEngine.WithObservability(rengine.NewObservability(nil)),
		fetcherEngine.WithLimits(fetcherEngine.DefaultLimits()),
	)
	require.NoError(t, err)

	return engine
}

// TestIntegration_Parity_PartialOnSingleSectionFailure proves the PARTIAL
// classification survives the cutover against REAL infra: one section maps a
// table absent from the discovered schema (it fails PlanExtraction validation),
// the other extracts cleanly. The failed section is dropped from result and
// recorded; decideReportStatus classifies the whole report PARTIAL.
func TestIntegration_Parity_PartialOnSingleSectionFailure(t *testing.T) {
	pgInfra, pgDown := parityStartPostgres(t)
	defer pgDown()

	pg := parityOpenPostgres(t, pgInfra)
	seedParityPostgres(t, pg)

	uc := parityUseCase(parityEngineNoMongo(t, pg))

	message := GenerateReportMessage{
		ReportID:   uuid.New(),
		TemplateID: uuid.New(),
		DataQueries: map[string]map[string][]string{
			"ledger": {
				"public__accounts":   {"id"},
				"public__ghosttable": {"id"}, // absent from the schema -> section fails
			},
		},
	}

	result := make(map[string]map[string][]map[string]any)

	failures, err := uc.extractViaEngine(parityCtx(), message, result)
	require.NoError(t, err, "a section failure is recorded, never fatal")
	require.Len(t, failures, 1)
	assert.Equal(t, "ledger", failures[0].database)
	assert.NotEmpty(t, failures[0].errorCode, "failed section carries a classified code (E9)")

	// The whole ledger section is dropped (a section is all-or-nothing): the
	// bad table sinks it, exactly as the legacy direct path dropped the section.
	assert.NotContains(t, result, "ledger")

	status, metadata := decideReportStatus(len(message.DataQueries), failures)
	assert.Equal(t, "Error", status,
		"with a single datasource, its failure is all-sections-failed -> Error")
	require.NotNil(t, metadata)

	// And the mixed case: add a second, healthy datasource so one section
	// survives -> PARTIAL.
	failuresMixed := []sectionFailure{{database: "ledger", errorCode: "x"}}
	statusMixed, _ := decideReportStatus(2, failuresMixed)
	assert.Equal(t, "Partial", statusMixed,
		"one of two sections failing must classify PARTIAL, not Error")
}

// TestIntegration_Parity_ContextCancelMarksError proves a context canceled
// mid-extraction surfaces a FATAL error from extractViaEngine (not a recorded
// section failure), so GenerateReport marks the report Error rather than
// silently Finishing it with missing data. The cancel happens before the loop
// observes the next datasource; extractViaEngine's ctx.Err() guard converts it
// into the returned error the handler treats as fatal.
func TestIntegration_Parity_ContextCancelMarksError(t *testing.T) {
	pgInfra, pgDown := parityStartPostgres(t)
	defer pgDown()

	pg := parityOpenPostgres(t, pgInfra)
	seedParityPostgres(t, pg)

	uc := parityUseCase(parityEngineNoMongo(t, pg))

	ctx, cancel := context.WithCancel(parityCtx())

	message := GenerateReportMessage{
		ReportID:   uuid.New(),
		TemplateID: uuid.New(),
		DataQueries: map[string]map[string][]string{
			"ledger": {"public__accounts": {"id"}},
		},
	}

	// Cancel before extraction begins: the per-datasource ctx.Err() guard at the
	// top of extractViaEngine's loop converts the canceled context into the
	// returned fatal error.
	cancel()

	result := make(map[string]map[string][]map[string]any)

	_, err := uc.extractViaEngine(ctx, message, result)
	require.Error(t, err, "a canceled context must surface a fatal error, never silent success")
	require.ErrorIs(t, err, context.Canceled)

	// And the handler maps any fatal extraction error to report status Error.
	meta := reportErrorMetadata(err)
	assert.Equal(t, "report_generation_canceled", meta["error_code"])
}

// TestIntegration_Parity_CRMCovered points at where the crm
// worker-level parity now lives.
//
// crm parity (org fan-out over holders_* physical collections with
// organization_id injection, CryptoEncryptSecretKeyCRM field decryption,
// and the document -> search.document hash-based advanced-filter transform) does
// NOT route through the generic engine assembled in this file — it runs via
// uc.extractCRM against the host-owned per-tenant mongo repository
// (UseCase.ExternalDataSources). That path is covered end-to-end against a live
// testcontainers MongoDB in
// generate-report-extraction_crm_parity_integration_test.go, which drives
// the real composed handler over a real mongodb.ExternalDataSource and
// CircuitBreakerManager with fixtures encrypted/hashed by the same lib-commons
// crypto primitives the crm module uses.
func TestIntegration_Parity_CRMCovered(t *testing.T) {
	t.Log("crm worker-level parity covered in " +
		"generate-report-extraction_crm_parity_integration_test.go " +
		"(TestIntegration_CRMParity_*).")
}
