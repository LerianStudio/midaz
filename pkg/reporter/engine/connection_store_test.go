// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDatasourceLookup stubs the bootstrap datasource lookup seam.
type fakeDatasourceLookup struct {
	conns map[string]DatasourceConnection
	names []string
}

func (f *fakeDatasourceLookup) LookupDatasource(configName string) (DatasourceConnection, bool) {
	c, ok := f.conns[configName]
	return c, ok
}

func (f *fakeDatasourceLookup) DatasourceConfigNames() []string { return f.names }

func newFakeLookup() *fakeDatasourceLookup {
	return &fakeDatasourceLookup{
		conns: map[string]DatasourceConnection{
			"ledger": {
				ConfigName:   "ledger",
				Type:         DatasourceTypePostgres,
				Host:         "pg-host",
				Port:         "5432",
				DatabaseName: "ledger_db",
				Username:     "app",
				SSLMode:      "require",
				Schemas:      []string{"public", "reporting"},
			},
			"crm": {
				ConfigName:   "crm",
				Type:         DatasourceTypeMongo,
				Host:         "mongo-host",
				Port:         "27017",
				DatabaseName: "crm_db",
			},
		},
		names: []string{"ledger", "crm"},
	}
}

func TestConnectionStore_FindConnection_StampsTenant(t *testing.T) {
	t.Parallel()

	s := NewConnectionStore(newFakeLookup())

	desc, ok, err := s.FindConnection(context.Background(), fetcher.TenantContext{TenantID: "tenant-x"}, "ledger")
	require.NoError(t, err)
	require.True(t, ok)

	// The tenant is stamped into HostAttributes — the load-bearing link the
	// connector factory reads back at Build time.
	assert.Equal(t, "tenant-x", tenantIDFromDescriptor(desc))
	assert.Equal(t, "ledger", desc.ConfigName)
	assert.Equal(t, DatasourceTypePostgres, desc.Type)
	assert.Equal(t, "pg-host", desc.Host)
	assert.Equal(t, 5432, desc.Port)
	assert.Equal(t, "ledger_db", desc.DatabaseName)
	assert.Equal(t, "app", desc.Username)
	assert.Equal(t, "require", desc.SSLMode)
	// Configured schemas ride along as a HostAttributes override.
	assert.Equal(t, []string{"public", "reporting"}, schemaOverrideFromDescriptor(desc))
}

func TestConnectionStore_FindConnection_UnknownReturnsNotFound(t *testing.T) {
	t.Parallel()

	s := NewConnectionStore(newFakeLookup())

	_, ok, err := s.FindConnection(context.Background(), fetcher.TenantContext{TenantID: "t"}, "missing")
	require.NoError(t, err, "unknown config name must not be an error")
	assert.False(t, ok)
}

func TestConnectionStore_FindByID_DelegatesToFindConnection(t *testing.T) {
	t.Parallel()

	s := NewConnectionStore(newFakeLookup())

	desc, ok, err := s.FindByID(context.Background(), fetcher.TenantContext{TenantID: "t"}, "crm")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, DatasourceTypeMongo, desc.Type)
	assert.Equal(t, 27017, desc.Port)
}

func TestConnectionStore_PortParsing(t *testing.T) {
	t.Parallel()

	t.Run("empty port maps to zero", func(t *testing.T) {
		t.Parallel()

		port, err := parsePort("")
		require.NoError(t, err)
		assert.Equal(t, 0, port)
	})

	t.Run("non-numeric port fails closed", func(t *testing.T) {
		t.Parallel()

		_, err := parsePort("not-a-port")
		var engineErr *fetcher.EngineError
		require.ErrorAs(t, err, &engineErr)
		assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
	})
}

func TestMapDatasourceType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DatasourceTypePostgres, mapDatasourceType("postgresql"))
	assert.Equal(t, DatasourceTypeMongo, mapDatasourceType("mongodb"))
	// Unknown types pass through unchanged so the engine can reject them by key.
	assert.Equal(t, "redshift", mapDatasourceType("redshift"))
}

func TestConnectionStore_List_SortedAndStamped(t *testing.T) {
	t.Parallel()

	s := NewConnectionStore(newFakeLookup())

	descs, err := s.List(context.Background(), fetcher.TenantContext{TenantID: "tenant-y"})
	require.NoError(t, err)
	require.Len(t, descs, 2)

	// Deterministic config-name-sorted order: crm before ledger.
	assert.Equal(t, "crm", descs[0].ConfigName)
	assert.Equal(t, "ledger", descs[1].ConfigName)

	for _, d := range descs {
		assert.Equal(t, "tenant-y", tenantIDFromDescriptor(d))
	}
}

func TestConnectionStore_WriteMethodsUnsupported(t *testing.T) {
	t.Parallel()

	s := NewConnectionStore(newFakeLookup())
	ctx := context.Background()
	tenant := fetcher.TenantContext{TenantID: "t"}

	assertUnsupported := func(t *testing.T, err error) {
		t.Helper()

		var engineErr *fetcher.EngineError
		require.ErrorAs(t, err, &engineErr)
		assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
	}

	assertUnsupported(t, s.Create(ctx, tenant, fetcher.ConnectionDescriptor{}, nil))
	assertUnsupported(t, s.Update(ctx, tenant, fetcher.ConnectionDescriptor{}, nil))
	assertUnsupported(t, s.Delete(ctx, tenant, "ledger"))
	assertUnsupported(t, s.UpdateByID(ctx, tenant, "id", fetcher.ConnectionDescriptor{}, nil))
	assertUnsupported(t, s.DeleteByID(ctx, tenant, "id"))

	_, err := s.ListPaged(ctx, tenant, fetcher.ConnectionListParams{})
	assertUnsupported(t, err)
}
