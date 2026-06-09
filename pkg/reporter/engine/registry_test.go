// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// fakeResolver is a TenantResolver stub recording the tenant/config it was asked
// to resolve, used to assert tenant isolation at the unit level.
type fakeResolver struct {
	multiTenant bool

	pgErr    error
	mongoErr error
	pgDB     sqlQuerier
	mongoDB  *mongo.Database

	gotPGTenant    string
	gotPGConfig    string
	gotMongoTenant string
	gotMongoConfig string
	resolvePGCalls int
	resolveMBCalls int
}

func (r *fakeResolver) IsMultiTenant() bool { return r.multiTenant }

func (r *fakeResolver) ResolvePostgres(_ context.Context, tenantID, configName string) (postgresHandle, error) {
	r.resolvePGCalls++
	r.gotPGTenant = tenantID
	r.gotPGConfig = configName

	if r.pgErr != nil {
		return postgresHandle{}, r.pgErr
	}

	return postgresHandle{db: r.pgDB, schemas: []string{"public"}}, nil
}

func (r *fakeResolver) ResolveMongo(_ context.Context, tenantID, configName string) (mongoHandle, error) {
	r.resolveMBCalls++
	r.gotMongoTenant = tenantID
	r.gotMongoConfig = configName

	if r.mongoErr != nil {
		return mongoHandle{}, r.mongoErr
	}

	return mongoHandle{db: r.mongoDB}, nil
}

func TestRegistry_ResolvesKnownTypes(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(&fakeResolver{}, nil)

	pg, ok := reg.Connector(DatasourceTypePostgres)
	require.True(t, ok)
	assert.NotNil(t, pg)

	mg, ok := reg.Connector(DatasourceTypeMongo)
	require.True(t, ok)
	assert.NotNil(t, mg)
}

func TestRegistry_UnknownTypeReportsFalse(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(&fakeResolver{}, nil)

	_, ok := reg.Connector("cassandra")
	assert.False(t, ok)
}

func TestPostgresFactory_BuildRequiresConfigName(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(&fakeResolver{}, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	_, err := factory.Build(context.Background(), fetcher.ConnectionDescriptor{})

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestPostgresFactory_BuildPassesTenantToResolver(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{multiTenant: true, pgDB: &fakeSQLQuerier{}}
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	desc := WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "ledger",
		Type:       DatasourceTypePostgres,
	}, "tenant-a")

	_, err := factory.Build(context.Background(), desc)
	require.NoError(t, err)

	assert.Equal(t, "tenant-a", resolver.gotPGTenant)
	assert.Equal(t, "ledger", resolver.gotPGConfig)
}

func TestMongoFactory_BuildPassesTenantToResolver(t *testing.T) {
	t.Parallel()

	// A nil mongo DB makes Build fail after resolution; we only assert the
	// resolver received the right tenant/config before that.
	resolver := &fakeResolver{multiTenant: true}
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypeMongo)

	desc := WithTenantID(fetcher.ConnectionDescriptor{
		ConfigName: "crm",
		Type:       DatasourceTypeMongo,
	}, "tenant-b")

	_, _ = factory.Build(context.Background(), desc)

	assert.Equal(t, "tenant-b", resolver.gotMongoTenant)
	assert.Equal(t, "crm", resolver.gotMongoConfig)
}

func TestPostgresFactory_PropagatesResolverError(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{pgErr: errors.New("boom")}
	reg := NewRegistry(resolver, nil)
	factory, _ := reg.Connector(DatasourceTypePostgres)

	_, err := factory.Build(context.Background(), fetcher.ConnectionDescriptor{ConfigName: "ledger"})
	require.Error(t, err)
}

// fakeSQLQuerier is a do-nothing sqlQuerier used where Build must succeed but no
// query is issued.
type fakeSQLQuerier struct{ pingErr error }

func (f *fakeSQLQuerier) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeSQLQuerier) PingContext(context.Context) error { return f.pingErr }
