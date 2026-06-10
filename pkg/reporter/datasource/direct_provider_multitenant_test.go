// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	pg "github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock the multi-tenant DirectProvider dispatch — the third-rail
// seam where a regression would silently read the global env pool instead of
// the per-tenant pool, fail open on a tenant-resolution error, or break the
// CRM-vs-org routing asymmetry. The inner tenantSchemaSource isolation is proven
// in tenant_schema_source_test.go; here we prove the PROVIDER wiring routes
// through that seam, never to the SafeDataSources repositories, and fails closed.

// recordingSchemaSource is a schemaSource that records the exact arguments it
// was called with and returns configurable schema/error. It lets a provider
// test observe routing (which dataSourceID/orgID reached the MT source) without
// a live database.
type recordingSchemaSource struct {
	pgSchema    []pg.TableSchema
	pgErr       error
	mongoSchema []mongodb.CollectionSchema
	mongoErr    error

	pgCalls    int
	pgConfig   string
	pgSchemas  []string
	mongoCalls int
	mongoDSID  string
	mongoOrgID string
}

func (r *recordingSchemaSource) PostgresSchema(_ context.Context, configName string, schemas []string) ([]pg.TableSchema, error) {
	r.pgCalls++
	r.pgConfig = configName
	r.pgSchemas = schemas

	if r.pgErr != nil {
		return nil, r.pgErr
	}

	return r.pgSchema, nil
}

func (r *recordingSchemaSource) MongoSchema(_ context.Context, dataSourceID, organizationID string) ([]mongodb.CollectionSchema, error) {
	r.mongoCalls++
	r.mongoDSID = dataSourceID
	r.mongoOrgID = organizationID

	if r.mongoErr != nil {
		return nil, r.mongoErr
	}

	return r.mongoSchema, nil
}

var _ schemaSource = (*recordingSchemaSource)(nil)

// newMTProvider builds a DirectProvider in multi-tenant mode with the given
// recording schema source injected directly onto the unexported seam. The
// SafeDataSources entries carry NO live repositories (PostgresRepository /
// MongoDBRepository are nil) so any accidental env-pool read would error — but
// the MT dispatch must route to the recorder before that ever happens.
func newMTProvider(t *testing.T, dsMap map[string]pkg.DataSource, src schemaSource) *DirectProvider {
	t.Helper()

	sds := newTestSafeDataSources(t, dsMap)

	return &DirectProvider{
		safeDatasources: sds,
		tenantSchema:    src,
	}
}

// --- Routing: MT ValidateSchema goes through the tenant source, not env pool --

func TestDirectProvider_MT_ValidatePostgres_RoutesThroughTenantSource(t *testing.T) {
	src := &recordingSchemaSource{
		pgSchema: []pg.TableSchema{
			{
				SchemaName: "public",
				TableName:  "users",
				Columns: []pg.ColumnInformation{
					{Name: "id", DataType: "uuid"},
					{Name: "name", DataType: "varchar"},
				},
			},
		},
	}

	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType: pkg.PostgreSQLType,
			// Intentionally NOT initialized and with a nil repository: in MT mode
			// the provider must never consult these, dispatching to the tenant
			// source instead.
			Initialized:        false,
			Status:             libConstants.DataSourceStatusUnavailable,
			PostgresRepository: nil,
			Schemas:            []string{"public", "reporting"},
		},
	}

	provider := newMTProvider(t, dsMap, src)

	result, err := provider.ValidateSchema(context.Background(), "pg_main", map[string][]string{"users": {"id", "name"}})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid, "fields exist in the tenant-resolved schema")
	assert.Equal(t, 1, src.pgCalls, "MT validation must resolve schema through the tenant source")
	assert.Equal(t, "pg_main", src.pgConfig)
	assert.Equal(t, []string{"public", "reporting"}, src.pgSchemas,
		"the datasource's configured schemas must be passed through unchanged")
}

// --- Fail-closed: a tenant-resolution error is a HARD error, not a D7 warning -

func TestDirectProvider_MT_ValidatePostgres_ResolutionErrorFailsClosed(t *testing.T) {
	resolveErr := errors.New("tenant manager unavailable")
	src := &recordingSchemaSource{pgErr: resolveErr}

	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType: pkg.PostgreSQLType,
			// Mark the datasource unavailable: in single-tenant mode this would
			// trip the D7 green-warning path. In MT mode it must NOT — an
			// unresolvable tenant pool is a real failure, not a known-degraded
			// datasource.
			Status:  libConstants.DataSourceStatusUnavailable,
			Schemas: []string{"public"},
		},
	}

	provider := newMTProvider(t, dsMap, src)

	result, err := provider.ValidateSchema(context.Background(), "pg_main", map[string][]string{"users": {"id"}})

	require.Error(t, err, "a tenant-resolution failure must surface as a hard error")
	assert.Nil(t, result, "no Valid:true result may leak on a resolution failure")
	assert.ErrorIs(t, err, resolveErr, "the underlying tenant-resolution error must propagate")
}

func TestDirectProvider_MT_ValidateMongo_ResolutionErrorFailsClosed(t *testing.T) {
	resolveErr := errors.New("tenant mongo manager unavailable")
	src := &recordingSchemaSource{mongoErr: resolveErr}

	dsMap := map[string]pkg.DataSource{
		crmDataSourceID: {
			DatabaseType: pkg.MongoDBType,
			Status:       libConstants.DataSourceStatusUnavailable,
		},
	}

	provider := newMTProvider(t, dsMap, src)

	result, err := provider.ValidateSchema(context.Background(), crmDataSourceID, map[string][]string{"holders": {"id"}})

	require.Error(t, err, "a tenant-resolution failure must surface as a hard error")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, resolveErr)
}

// --- CRM-vs-org asymmetry: details uses crm branch, validation skips it -------

func TestDirectProvider_MT_CRMDetails_UsesCRMBranch(t *testing.T) {
	src := &recordingSchemaSource{
		mongoSchema: []mongodb.CollectionSchema{
			{CollectionName: "holders", Fields: []mongodb.FieldInformation{{Name: "id", DataType: "string"}}},
		},
	}

	dsMap := map[string]pkg.DataSource{
		crmDataSourceID: {
			DatabaseType:        pkg.MongoDBType,
			MidazOrganizationID: "org-123",
		},
	}

	provider := newMTProvider(t, dsMap, src)

	schema, err := provider.GetDataSourceSchema(context.Background(), crmDataSourceID)

	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, 1, src.mongoCalls)
	assert.Equal(t, crmDataSourceID, src.mongoDSID,
		"the DETAILS path must pass the crm dataSourceID so the MT source takes the CRM prefix-grouped branch")
	assert.Equal(t, "org-123", src.mongoOrgID)
}

func TestDirectProvider_MT_CRMValidation_SkipsCRMBranch(t *testing.T) {
	// The validation path deliberately passes an empty dataSourceID so the MT
	// source takes the org/plain branch, matching the single-tenant validation
	// contract (raw per-org collections, not CRM prefix-grouped logical names).
	src := &recordingSchemaSource{
		mongoSchema: []mongodb.CollectionSchema{
			{CollectionName: "holders_org-123", Fields: []mongodb.FieldInformation{{Name: "id", DataType: "string"}}},
		},
	}

	dsMap := map[string]pkg.DataSource{
		crmDataSourceID: {
			DatabaseType:        pkg.MongoDBType,
			MidazOrganizationID: "org-123",
		},
	}

	provider := newMTProvider(t, dsMap, src)

	result, err := provider.ValidateSchema(context.Background(), crmDataSourceID, map[string][]string{"holders": {"id"}})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, src.mongoCalls)
	assert.Empty(t, src.mongoDSID,
		"the VALIDATION path must pass an empty dataSourceID so the MT source stays OFF the CRM prefix branch")
	assert.Equal(t, "org-123", src.mongoOrgID,
		"validation must still pass the org ID so the org-suffix transformation matches")
	assert.True(t, result.Valid, "the org-suffixed collection must resolve via the org branch")
}

// --- GetDataSourceSchema MT routing for postgres -----------------------------

func TestDirectProvider_MT_GetPostgresSchema_RoutesThroughTenantSource(t *testing.T) {
	src := &recordingSchemaSource{
		pgSchema: []pg.TableSchema{
			{SchemaName: "public", TableName: "users", Columns: []pg.ColumnInformation{{Name: "id", DataType: "uuid"}}},
		},
	}

	dsMap := map[string]pkg.DataSource{
		"pg_main": {
			DatabaseType:       pkg.PostgreSQLType,
			Initialized:        false,
			PostgresRepository: nil,
			Schemas:            []string{"public"},
		},
	}

	provider := newMTProvider(t, dsMap, src)

	schema, err := provider.GetDataSourceSchema(context.Background(), "pg_main")

	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, 1, src.pgCalls, "GetDataSourceSchema must route through the tenant source in MT mode")
	require.Len(t, schema.Tables, 1)
	assert.Equal(t, "public.users", schema.Tables[0].Name)
}

// --- A non-crm org-scoped datasource takes the org path on BOTH paths ---------

func TestDirectProvider_MT_NonCRMOrgScoped_UsesOrgPath(t *testing.T) {
	src := &recordingSchemaSource{
		mongoSchema: []mongodb.CollectionSchema{
			{CollectionName: "accounts_org-9", Fields: []mongodb.FieldInformation{{Name: "id", DataType: "string"}}},
		},
	}

	dsMap := map[string]pkg.DataSource{
		"mongo_org": {
			DatabaseType:        pkg.MongoDBType,
			MidazOrganizationID: "org-9",
		},
	}

	provider := newMTProvider(t, dsMap, src)

	// Details path: passes the real (non-crm) dataSourceID, which the MT source
	// routes to the org branch because organizationID is set and the ID is not crm.
	_, err := provider.GetDataSourceSchema(context.Background(), "mongo_org")
	require.NoError(t, err)
	assert.Equal(t, "mongo_org", src.mongoDSID)
	assert.Equal(t, "org-9", src.mongoOrgID)
}
