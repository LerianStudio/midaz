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

// fakePGManager / fakeMongoManager stub the lib-commons tenant managers,
// recording which tenant they were asked to resolve.
type fakePGManager struct {
	db        sqlQuerier
	err       error
	gotTenant string
}

func (m *fakePGManager) GetDB(_ context.Context, tenantID string) (sqlQuerier, error) {
	m.gotTenant = tenantID
	if m.err != nil {
		return nil, m.err
	}

	return m.db, nil
}

type fakeMongoManager struct {
	db        *mongo.Database
	err       error
	gotTenant string
}

func (m *fakeMongoManager) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	m.gotTenant = tenantID
	if m.err != nil {
		return nil, m.err
	}

	return m.db, nil
}

// fakeSingleTenantDatasources stubs the env-configured datasource map.
type fakeSingleTenantDatasources struct {
	pgDB    *sql.DB
	schemas []string
	pgErr   error

	mongoDB  *mongo.Database
	mongoErr error
}

func (f *fakeSingleTenantDatasources) ResolvePostgres(context.Context, string) (*sql.DB, []string, error) {
	if f.pgErr != nil {
		return nil, nil, f.pgErr
	}

	return f.pgDB, f.schemas, nil
}

func (f *fakeSingleTenantDatasources) ResolveMongo(context.Context, string) (*mongo.Database, error) {
	if f.mongoErr != nil {
		return nil, f.mongoErr
	}

	return f.mongoDB, nil
}

func TestMultiTenantResolver_RequiresTenantID(t *testing.T) {
	t.Parallel()

	r := NewMultiTenantResolver(&fakePGManager{}, &fakeMongoManager{}, nil)

	_, err := r.ResolvePostgres(context.Background(), "", "ledger")
	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestMultiTenantResolver_RejectsMalformedTenantID(t *testing.T) {
	t.Parallel()

	r := NewMultiTenantResolver(&fakePGManager{}, &fakeMongoManager{}, nil)

	_, err := r.ResolvePostgres(context.Background(), "bad tenant id!", "ledger")
	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestMultiTenantResolver_ForwardsTenantToPGManager(t *testing.T) {
	t.Parallel()

	pg := &fakePGManager{db: &fakeSQLQuerier{}}
	r := NewMultiTenantResolver(pg, &fakeMongoManager{}, func(string) []string { return []string{"public", "audit"} })

	handle, err := r.ResolvePostgres(context.Background(), "tenant-x", "ledger")
	require.NoError(t, err)

	assert.Equal(t, "tenant-x", pg.gotTenant)
	assert.Equal(t, []string{"public", "audit"}, handle.schemas)
	assert.True(t, r.IsMultiTenant())
}

func TestMultiTenantResolver_WrapsManagerErrorAsUnavailable(t *testing.T) {
	t.Parallel()

	pg := &fakePGManager{err: errors.New("pool exhausted")}
	r := NewMultiTenantResolver(pg, &fakeMongoManager{}, nil)

	_, err := r.ResolvePostgres(context.Background(), "tenant-x", "ledger")
	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryUnavailable, engineErr.Category)
}

func TestSingleTenantResolver_IgnoresTenantID(t *testing.T) {
	t.Parallel()

	ds := &fakeSingleTenantDatasources{pgDB: &sql.DB{}, schemas: []string{"public"}}
	r := NewSingleTenantResolver(ds)

	handle, err := r.ResolvePostgres(context.Background(), "irrelevant-tenant", "ledger")
	require.NoError(t, err)
	assert.Equal(t, []string{"public"}, handle.schemas)
	assert.False(t, r.IsMultiTenant())
}

func TestDescriptor_TenantRoundTrip(t *testing.T) {
	t.Parallel()

	desc := WithTenantID(fetcher.ConnectionDescriptor{ConfigName: "ledger"}, "tenant-z")
	assert.Equal(t, "tenant-z", tenantIDFromDescriptor(desc))
}

func TestDescriptor_NoTenantReturnsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, tenantIDFromDescriptor(fetcher.ConnectionDescriptor{ConfigName: "ledger"}))
}

func TestDescriptor_SchemaOverride(t *testing.T) {
	t.Parallel()

	desc := fetcher.ConnectionDescriptor{
		ConfigName: "ledger",
		HostAttributes: map[string]any{
			hostAttrSchemas: []string{"public", "reporting"},
		},
	}

	assert.Equal(t, []string{"public", "reporting"}, schemaOverrideFromDescriptor(desc))
}
