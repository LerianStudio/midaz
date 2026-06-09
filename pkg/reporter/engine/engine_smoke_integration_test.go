// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// smokeLookup is a DatasourceLookup over a fixed set of descriptors, standing in
// for the bootstrap datasource map so the smoke test exercises the real
// ConnectionStore → Registry → Connector path the engine drives.
type smokeLookup struct {
	conns map[string]DatasourceConnection
	names []string
}

func (l *smokeLookup) LookupDatasource(configName string) (DatasourceConnection, bool) {
	c, ok := l.conns[configName]
	return c, ok
}

func (l *smokeLookup) DatasourceConfigNames() []string { return l.names }

// newSmokeEngine wires the Phase 2 ports — ConnectionStore (T01), Observability
// (T02), Registry (Phase 1) over a multi-tenant resolver — into a real
// engine.New, the same assembly the worker bootstrap performs.
func newSmokeEngine(t *testing.T, dbs map[string]*sql.DB) *fetcher.Engine {
	t.Helper()

	lookup := &smokeLookup{
		conns: map[string]DatasourceConnection{
			"ledger": {ConfigName: "ledger", Type: DatasourceTypePostgres, Schemas: []string{"public"}},
		},
		names: []string{"ledger"},
	}

	resolver := NewMultiTenantResolver(&pgManagerFake{dbs: dbs}, nil, nil)

	engine, err := fetcher.New(
		fetcher.WithConnectorRegistry(NewRegistry(resolver, nil)),
		fetcher.WithConnectionStore(NewConnectionStore(lookup)),
		fetcher.WithObservability(NewObservability(nil)),
		fetcher.WithLimits(fetcher.DefaultLimits()),
	)
	require.NoError(t, err)

	return engine
}

// extract drives PlanExtraction + ExecuteExtraction (Direct mode) for one tenant
// and returns the decoded rows for the ledger.public.accounts table. The request
// is FILTERLESS — filters are still rejected until Phase 3.
func extract(t *testing.T, engine *fetcher.Engine, tenantID string) []map[string]any {
	t.Helper()

	ctx := context.Background()
	tenant, err := fetcher.NewTenantContext(tenantID)
	require.NoError(t, err)

	req := fetcher.ExtractionRequest{
		MappedFields: map[string]fetcher.FieldSelection{
			"ledger": {"public.accounts": {"id", "owner"}},
		},
	}

	plan, err := engine.PlanExtraction(ctx, tenant, req)
	require.NoError(t, err)

	result, err := engine.ExecuteExtraction(ctx, plan)
	require.NoError(t, err)
	require.NotNil(t, result.Direct, "direct mode must return an inline payload")

	return rowsFromDirect(t, result.Direct.Data)
}

// rowsFromDirect extracts the accounts rows from the engine's Direct payload. The
// engine's serialized result shape is an internal contract, so this walks the
// decoded structure for the accounts table rather than assuming a fixed top-level
// type — tolerating both a flat {table: [rows]} map and a nested
// {datasource: {table: [rows]}} map.
func rowsFromDirect(t *testing.T, data []byte) []map[string]any {
	t.Helper()

	var root any
	require.NoError(t, json.Unmarshal(data, &root))

	return findAccountRows(root)
}

func findAccountRows(node any) []map[string]any {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if strings.Contains(key, "accounts") {
				if rows := toRowSlice(child); rows != nil {
					return rows
				}
			}

			if rows := findAccountRows(child); rows != nil {
				return rows
			}
		}
	case []any:
		for _, child := range v {
			if rows := findAccountRows(child); rows != nil {
				return rows
			}
		}
	}

	return nil
}

func toRowSlice(node any) []map[string]any {
	arr, ok := node.([]any)
	if !ok {
		return nil
	}

	rows := make([]map[string]any, 0, len(arr))

	for _, item := range arr {
		row, ok := item.(map[string]any)
		if !ok {
			return nil
		}

		rows = append(rows, row)
	}

	return rows
}

func TestIntegration_EngineSmoke_NewThenExtract(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	db := openTenantDB(t, infra, "tenant_default", false)

	_, err := db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, owner text, secret text)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO accounts (id, owner, secret) VALUES ($1, $2, $3)`, "acc-1", "tenant-default", "hidden")
	require.NoError(t, err)

	engine := newSmokeEngine(t, map[string]*sql.DB{"tenant-default": db})

	rows := extract(t, engine, "tenant-default")
	require.Len(t, rows, 1)
	assert.Equal(t, "acc-1", rows[0]["id"])
	assert.Equal(t, "tenant-default", rows[0]["owner"])
	// Projection must exclude the unselected secret column.
	_, hasSecret := rows[0]["secret"]
	assert.False(t, hasSecret, "secret column must not be extracted")
}

func TestIntegration_EngineSmoke_TenantIsolation(t *testing.T) {
	infra, teardown := startPostgres(t)
	defer teardown()

	ctx := context.Background()
	dbA := openTenantDB(t, infra, "tenant_default", false)
	dbB := openTenantDB(t, infra, "tenant_b", true)

	for _, setup := range []struct {
		db    *sql.DB
		owner string
	}{{dbA, "tenant-a"}, {dbB, "tenant-b"}} {
		_, err := setup.db.ExecContext(ctx, `CREATE TABLE accounts (id text PRIMARY KEY, owner text)`)
		require.NoError(t, err)
		_, err = setup.db.ExecContext(ctx, `INSERT INTO accounts (id, owner) VALUES ($1, $2)`, "row-1", setup.owner)
		require.NoError(t, err)
	}

	engine := newSmokeEngine(t, map[string]*sql.DB{
		"tenant-a": dbA,
		"tenant-b": dbB,
	})

	// Each tenant's extraction resolves ONLY its own database — the tenant ID
	// stamped by FindConnection into the descriptor routes the connector to the
	// right pool. No cross-tenant read.
	rowsA := extract(t, engine, "tenant-a")
	require.Len(t, rowsA, 1)
	assert.Equal(t, "tenant-a", rowsA[0]["owner"])

	rowsB := extract(t, engine, "tenant-b")
	require.Len(t, rowsB, 1)
	assert.Equal(t, "tenant-b", rowsB[0]["owner"])
}
